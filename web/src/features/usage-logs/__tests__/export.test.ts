/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import {
  exportUsageLogsCsv,
  usageLogCsvRow,
  type UsageLogExportOptions,
  type UsageLogExportRecord,
} from '../../../../shared/usage-log-export'

const headers = [
  'Username',
  'Token name',
  'Model name',
  'Input tokens',
  'Output tokens',
  'Cache read tokens',
  'Cache write tokens',
  'Amount (USD)',
  'Time',
]
const record: UsageLogExportRecord = {
  username: 'alice',
  token_name: 'work',
  model_name: 'public-model',
  prompt_tokens: 1234,
  completion_tokens: 56,
  quota: 1250,
  created_at: 100,
}
const rowFormatting = {
  formatAmount: (quota: number) => quota / 500_000,
  formatTime: (timestamp: number) => `time-${timestamp}`,
}

function exportOptions(
  fetchExport: UsageLogExportOptions['fetchExport'],
  signal?: AbortSignal
): UsageLogExportOptions {
  return {
    fetchExport,
    ...rowFormatting,
    headers,
    filename: 'usage-logs.csv',
    signal,
  }
}

// Exercise the real decoder with arbitrary network chunk boundaries.
function responseStream(
  text: string,
  chunkSize = 31
): ReadableStream<Uint8Array> {
  const bytes = new TextEncoder().encode(text)
  return new ReadableStream({
    start(controller) {
      for (let offset = 0; offset < bytes.length; offset += chunkSize) {
        controller.enqueue(bytes.slice(offset, offset + chunkSize))
      }
      controller.close()
    },
  })
}

function readBlob(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.addEventListener('error', () => reject(reader.error), { once: true })
    reader.addEventListener(
      'load',
      () => {
        resolve(
          new TextDecoder('utf-8', { ignoreBOM: true }).decode(
            reader.result as ArrayBuffer
          )
        )
      },
      { once: true }
    )
    reader.readAsArrayBuffer(blob)
  })
}

describe('usage log CSV rows', () => {
  test('exports nine fields and retains small numeric charges and cache counts', () => {
    expect(
      usageLogCsvRow(
        {
          ...record,
          quota: 1,
          cache_read_tokens: 800,
          cache_write_tokens: 120,
        },
        rowFormatting
      )
    ).toBe('"alice","work","public-model",1234,56,800,120,0.000002,"time-100"')
  })

  test('defaults missing or invalid counters to zero', () => {
    expect(
      usageLogCsvRow(
        {
          created_at: 100,
          prompt_tokens: -1,
          completion_tokens: Number.NaN,
          cache_read_tokens: Infinity,
        },
        rowFormatting
      )
    ).toBe('"","","",0,0,0,0,0,"time-100"')
  })

  test('escapes Chinese, quotes, commas and embedded newlines', () => {
    expect(
      usageLogCsvRow(
        {
          ...record,
          username: '王,"甲"',
          token_name: 'first\r\nsecond',
        },
        rowFormatting
      )
    ).toBe(
      '"王,""甲""","first\r\nsecond","public-model",1234,56,0,0,0.0025,"time-100"'
    )
  })

  test.each([
    '=1+1',
    '+SUM(1,2)',
    '-1+2',
    '@SUM(1)',
    '\t =1',
    '\r\n=1',
    '\u0000=1',
  ])('treats formula-like names as text: %j', (name) => {
    expect(usageLogCsvRow({ ...record, username: name }, rowFormatting)).toBe(
      `"'${name}","work","public-model",1234,56,0,0,0.0025,"time-100"`
    )
  })
})

describe('complete streamed usage log CSV download', () => {
  const createObjectURL = vi.fn((_blob: Blob) => 'blob:usage-logs-export')
  const revokeObjectURL = vi.fn()
  let downloadedFilename: string | undefined

  beforeEach(() => {
    downloadedFilename = undefined
    vi.stubGlobal(
      'URL',
      class extends URL {
        static createObjectURL = createObjectURL
        static revokeObjectURL = revokeObjectURL
      }
    )
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(
      function (this: HTMLAnchorElement) {
        downloadedFilename = this.download
      }
    )
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  test('downloads 101 rows in one request with UTF-8 BOM, headers and browser cleanup', async () => {
    const rows = Array.from({ length: 101 }, (_, index) => ({
      ...record,
      username: `用户-${index}`,
    }))
    const payload = `${rows
      .map((row) => JSON.stringify(row))
      .join('\n')}\n{"complete":true,"count":101}\n`
    const fetchExport = vi.fn().mockResolvedValue(responseStream(payload, 1))
    vi.useFakeTimers()
    expect(await exportUsageLogsCsv(exportOptions(fetchExport))).toBe(101)
    expect(fetchExport).toHaveBeenCalledOnce()
    expect(createObjectURL).toHaveBeenCalledOnce()
    expect(downloadedFilename).toBe('usage-logs.csv')
    expect(document.querySelector('a[download]')).toBeNull()
    expect(revokeObjectURL).not.toHaveBeenCalled()
    vi.runAllTimers()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:usage-logs-export')
    vi.useRealTimers()

    const blob = createObjectURL.mock.calls[0][0]
    expect(blob.type).toBe('text/csv;charset=utf-8')
    const lines = (await readBlob(blob)).split('\r\n')
    expect(lines[0]).toBe(
      `\uFEFF${headers.map((header) => `"${header}"`).join(',')}`
    )
    expect(lines).toHaveLength(103)
    expect(lines[1]).toBe(
      '"用户-0","work","public-model",1234,56,0,0,0.0025,"time-100"'
    )
    expect(lines[101]).toBe(
      '"用户-100","work","public-model",1234,56,0,0,0.0025,"time-100"'
    )
  })

  test('does not download when no non-error logs match', async () => {
    const fetchExport = vi
      .fn()
      .mockResolvedValue(responseStream('{"complete":true,"count":0}\n'))
    expect(await exportUsageLogsCsv(exportOptions(fetchExport))).toBe(0)
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  test.each([
    ['missing completion marker', `${JSON.stringify(record)}\n`],
    [
      'wrong completion count',
      `${JSON.stringify(record)}\n{"complete":true,"count":2}\n`,
    ],
    ['malformed JSON', `${JSON.stringify(record)}\n{invalid\n`],
    ['invalid record', '{"success":false,"message":"failed"}\n'],
    [
      'data after completion',
      `{"complete":true,"count":0}\n${JSON.stringify(record)}\n`,
    ],
  ])('refuses a partial download: %s', async (_, payload) => {
    const fetchExport = vi.fn().mockResolvedValue(responseStream(payload))
    await expect(
      exportUsageLogsCsv(exportOptions(fetchExport))
    ).rejects.toThrow()
    expect(createObjectURL).not.toHaveBeenCalled()
    expect(downloadedFilename).toBeUndefined()
  })

  test('does not download after a network read failure', async () => {
    let reads = 0
    const stream = new ReadableStream<Uint8Array>({
      pull(controller) {
        if (reads++ === 0) {
          controller.enqueue(
            new TextEncoder().encode(JSON.stringify(record) + '\n')
          )
        } else controller.error(new Error('Connection interrupted'))
      },
    })
    await expect(
      exportUsageLogsCsv(exportOptions(async () => stream))
    ).rejects.toThrow('Connection interrupted')
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  test('does not request after cancellation', async () => {
    const controller = new AbortController()
    controller.abort()
    const fetchExport = vi.fn()
    await expect(
      exportUsageLogsCsv(exportOptions(fetchExport, controller.signal))
    ).rejects.toMatchObject({ name: 'AbortError' })
    expect(fetchExport).not.toHaveBeenCalled()
  })

  test('cancels a pending stream read and never downloads a partial report', async () => {
    const controller = new AbortController()
    const cancel = vi.fn()
    let reading!: () => void
    const pendingRead = new Promise<void>((resolve) => {
      reading = resolve
    })
    const stream = new ReadableStream<Uint8Array>({ pull: reading, cancel })
    const exporting = exportUsageLogsCsv(
      exportOptions(async () => stream, controller.signal)
    )
    const cancelled = expect(exporting).rejects.toMatchObject({
      name: 'AbortError',
    })
    await pendingRead
    controller.abort()
    await cancelled
    expect(cancel).toHaveBeenCalledOnce()
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  test('does not download if cancelled while waiting for the response', async () => {
    const controller = new AbortController()
    let resolveResponse!: (stream: ReadableStream<Uint8Array>) => void
    const fetchExport = () =>
      new Promise<ReadableStream<Uint8Array>>((resolve) => {
        resolveResponse = resolve
      })
    const exporting = exportUsageLogsCsv(
      exportOptions(fetchExport, controller.signal)
    )
    const cancelled = expect(exporting).rejects.toMatchObject({
      name: 'AbortError',
    })
    controller.abort()
    resolveResponse(
      responseStream(`${JSON.stringify(record)}\n{"complete":true,"count":1}\n`)
    )
    await cancelled
    expect(createObjectURL).not.toHaveBeenCalled()
  })
})
