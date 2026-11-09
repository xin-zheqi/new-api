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
  type UsageLogExportPage,
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
  fetchPage: UsageLogExportOptions['fetchPage'],
  signal?: AbortSignal
): UsageLogExportOptions {
  return {
    fetchPage,
    ...rowFormatting,
    headers,
    filename: 'usage-logs.csv',
    signal,
  }
}

function readBlob(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.addEventListener('error', () => reject(reader.error), { once: true })
    reader.addEventListener(
      'load',
      () => {
        // Reading bytes keeps the UTF-8 BOM observable instead of decoding it away.
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
  test('exports the nine requested fields and keeps raw token counts and exact numeric charges', () => {
    const row = usageLogCsvRow(
      {
        ...record,
        quota: 1,
        other: JSON.stringify({
          cache_tokens: 800,
          cache_creation_tokens: 120,
          billing_source: 'subscription',
        }),
      },
      rowFormatting
    )
    expect(row).toBe(
      '"alice","work","public-model",1234,56,800,120,0.000002,"time-100"'
    )
  })

  test.each([
    ['aggregate', { cache_creation_tokens: 150 }, 150],
    [
      'split durations replace the aggregate',
      {
        cache_creation_tokens: 150,
        cache_creation_tokens_5m: 20,
        cache_creation_tokens_1h: 30,
      },
      50,
    ],
    ['one split duration', { cache_creation_tokens_1h: 30 }, 30],
    [
      'zero splits fall back to aggregate',
      {
        cache_creation_tokens: 150,
        cache_creation_tokens_5m: 0,
        cache_creation_tokens_1h: 0,
      },
      150,
    ],
    [
      'invalid counts are not charges',
      { cache_tokens: -1, cache_creation_tokens: '40' },
      0,
    ],
  ])('handles %s cache metadata', (_, metadata, expectedWrite) => {
    expect(
      usageLogCsvRow(
        { ...record, other: JSON.stringify(metadata) },
        rowFormatting
      )
    ).toBe(
      `"alice","work","public-model",1234,56,0,${expectedWrite},0.0025,"time-100"`
    )
  })

  test.each([undefined, '', '{invalid', 'null', '[]', '42'])(
    'keeps old/error logs with missing or malformed metadata: %s',
    (other) => {
      expect(usageLogCsvRow({ ...record, other }, rowFormatting)).toBe(
        '"alice","work","public-model",1234,56,0,0,0.0025,"time-100"'
      )
    }
  )

  test('escapes quotes, commas and embedded newlines without altering names', () => {
    expect(
      usageLogCsvRow(
        { ...record, username: '王,"甲"', token_name: 'first\r\nsecond' },
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

describe('complete usage log CSV download', () => {
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

  test('downloads all pages once, including repeated display IDs, with a BOM and nine column headers', async () => {
    // The self-log endpoint repeats synthetic display IDs across independent views.
    const rows = Array.from({ length: 101 }, (_, index) => ({
      ...record,
      id: 1,
      username: `user-${index}`,
    }))
    const fetchPage = vi
      .fn<UsageLogExportOptions['fetchPage']>()
      .mockResolvedValueOnce({
        items: rows.slice(0, 100),
        page: 1,
        page_size: 100,
        total: 101,
      })
      .mockResolvedValueOnce({
        items: rows.slice(100),
        page: 2,
        page_size: 100,
        total: 101,
      })

    vi.useFakeTimers()
    expect(await exportUsageLogsCsv(exportOptions(fetchPage))).toBe(101)
    expect(fetchPage.mock.calls).toEqual([
      [1, 100],
      [2, 100],
    ])
    expect(createObjectURL).toHaveBeenCalledOnce()
    expect(downloadedFilename).toBe('usage-logs.csv')
    expect(document.querySelector('a[download]')).toBeNull()
    expect(revokeObjectURL).not.toHaveBeenCalled()
    vi.runAllTimers()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:usage-logs-export')
    vi.useRealTimers()

    const blob = createObjectURL.mock.calls[0][0]
    expect(blob.type).toBe('text/csv;charset=utf-8')
    const csv = await readBlob(blob)
    const lines = csv.split('\r\n')
    expect(lines[0]).toBe(
      `\uFEFF${headers.map((header) => `"${header}"`).join(',')}`
    )
    expect(lines).toHaveLength(103) // Header, 101 rows, final CRLF.
    expect(lines[1]).toBe(
      '"user-0","work","public-model",1234,56,0,0,0.0025,"time-100"'
    )
    expect(lines[101]).toBe(
      '"user-100","work","public-model",1234,56,0,0,0.0025,"time-100"'
    )
  })

  test('does not create a download when no logs match', async () => {
    const fetchPage = vi
      .fn<UsageLogExportOptions['fetchPage']>()
      .mockResolvedValue({ items: null, page: 1, page_size: 100, total: 0 })
    expect(await exportUsageLogsCsv(exportOptions(fetchPage))).toBe(0)
    expect(createObjectURL).not.toHaveBeenCalled()
    expect(downloadedFilename).toBeUndefined()
  })

  test('never downloads a partial report when a later page fails', async () => {
    const fetchPage = vi
      .fn<UsageLogExportOptions['fetchPage']>()
      .mockResolvedValueOnce({
        items: Array.from({ length: 100 }, () => record),
        page: 1,
        page_size: 100,
        total: 101,
      })
      .mockRejectedValueOnce(new Error('Connection interrupted'))
    await expect(exportUsageLogsCsv(exportOptions(fetchPage))).rejects.toThrow(
      'Connection interrupted'
    )
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  test.each([
    ['wrong page', { items: [record], page: 2, page_size: 100, total: 1 }],
    ['wrong page size', { items: [record], page: 1, page_size: 20, total: 1 }],
    ['invalid total', { items: [record], page: 1, page_size: 100, total: -1 }],
    ['incomplete page', { items: [], page: 1, page_size: 100, total: 1 }],
    [
      'surplus rows',
      { items: [record, record], page: 1, page_size: 100, total: 1 },
    ],
  ])('rejects %s rather than claiming a complete export', async (_, page) => {
    const fetchPage = vi
      .fn<UsageLogExportOptions['fetchPage']>()
      .mockResolvedValue(page)
    await expect(exportUsageLogsCsv(exportOptions(fetchPage))).rejects.toThrow()
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  test('aborts when the total changes during pagination', async () => {
    const fetchPage = vi
      .fn<UsageLogExportOptions['fetchPage']>()
      .mockResolvedValueOnce({
        items: Array.from({ length: 100 }, () => record),
        page: 1,
        page_size: 100,
        total: 101,
      })
      .mockResolvedValueOnce({
        items: [record, record],
        page: 2,
        page_size: 100,
        total: 102,
      })
    await expect(exportUsageLogsCsv(exportOptions(fetchPage))).rejects.toThrow()
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  test('does not query after cancellation before export', async () => {
    const controller = new AbortController()
    controller.abort()
    const fetchPage = vi.fn<UsageLogExportOptions['fetchPage']>()
    await expect(
      exportUsageLogsCsv(exportOptions(fetchPage, controller.signal))
    ).rejects.toMatchObject({ name: 'AbortError' })
    expect(fetchPage).not.toHaveBeenCalled()
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  test('does not download if cancelled while the final page is pending', async () => {
    const controller = new AbortController()
    let resolvePage!: (page: UsageLogExportPage) => void
    const fetchPage = vi.fn<UsageLogExportOptions['fetchPage']>(
      () =>
        new Promise((resolve) => {
          resolvePage = resolve
        })
    )
    const exporting = exportUsageLogsCsv(
      exportOptions(fetchPage, controller.signal)
    )
    const cancelled = expect(exporting).rejects.toMatchObject({
      name: 'AbortError',
    })
    controller.abort()
    resolvePage({ items: [record], page: 1, page_size: 100, total: 1 })
    await cancelled
    expect(createObjectURL).not.toHaveBeenCalled()
  })
})
