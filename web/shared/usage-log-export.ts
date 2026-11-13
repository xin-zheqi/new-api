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

// Framework-free so both frontends export the same columns and cache counts.
export interface UsageLogExportRecord {
  username?: string
  token_name?: string
  model_name?: string
  prompt_tokens?: number
  completion_tokens?: number
  quota?: number
  created_at: number
  cache_read_tokens?: number
  cache_write_tokens?: number
  is_stream?: boolean
  request_path?: string
  first_response_time_ms?: number | null
  use_time?: number
}

export interface UsageLogExportOptions {
  fetchExport: () => Promise<ReadableStream<Uint8Array>>
  headers: readonly string[]
  formatAmount: (quota: number) => number
  formatTime: (timestamp: number) => string
  requestTypeLabels: { streaming: string; nonStreaming: string }
  filename: string
  signal?: AbortSignal
}

function csvCell(value: string | number): string {
  if (typeof value === 'number') return String(value)
  // Quoting alone does not stop Excel/WPS from evaluating a name as a formula.
  const safe = /^[\s\p{Cc}]*[=+@-]/u.test(value) ? `'${value}` : value
  return `"${safe.replaceAll('"', '""')}"`
}

function tokenCount(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? value
    : 0
}

function requestInterface(path: unknown): string {
  if (typeof path !== 'string') return ''
  // Use the client's recorded route, not the channel's converted upstream API.
  // Strip query strings before falling back to an unfamiliar route.
  const route = path.split(/[?#]/, 1)[0].replace(/\/$/, '')
  if (route.endsWith('/chat/completions')) return 'chat'
  if (route.endsWith('/responses/compact')) return 'responses/compact'
  if (route.endsWith('/responses')) return 'responses'
  if (route.endsWith('/messages')) return 'messages'
  if (/\/models\/[^/]+:(?:streamGenerateContent|generateContent)$/.test(route)) {
    return 'gemini'
  }
  if (route.endsWith('/embeddings')) return 'embeddings'
  if (route.includes('/images/')) return 'images'
  if (route.includes('/audio/')) return 'audio'
  if (route.endsWith('/rerank')) return 'rerank'
  if (route.endsWith('/moderations')) return 'moderations'
  if (route.endsWith('/completions')) return 'completions'
  if (route.endsWith('/search')) return 'search'
  if (route.endsWith('/fetch')) return 'fetch'
  if (
    route === '/v1/videos' ||
    route.startsWith('/v1/videos/') ||
    route.startsWith('/v1/video/generations')
  ) {
    return 'videos'
  }
  return route
}

function durationSeconds(value: unknown, scale = 1): string | number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value / scale
    : ''
}

export function usageLogCsvRow(
  log: UsageLogExportRecord,
  options: Pick<
    UsageLogExportOptions,
    'formatAmount' | 'formatTime' | 'requestTypeLabels'
  >
): string {
  // Excel keeps 15 significant digits. Remove floating-point conversion noise
  // without rounding small charges to zero or changing stored billing values.
  const amount = Number(options.formatAmount(log.quota || 0).toPrecision(15))

  return [
    log.username || '',
    log.token_name || '',
    log.model_name || '',
    log.is_stream === true
      ? options.requestTypeLabels.streaming
      : options.requestTypeLabels.nonStreaming,
    requestInterface(log.request_path),
    tokenCount(log.prompt_tokens),
    tokenCount(log.completion_tokens),
    tokenCount(log.cache_read_tokens),
    tokenCount(log.cache_write_tokens),
    amount,
    options.formatTime(log.created_at),
    durationSeconds(log.first_response_time_ms, 1000),
    durationSeconds(log.use_time),
  ]
    .map(csvCell)
    .join(',')
}

/** Read one compact response; create the file only after its completion marker. */
export async function exportUsageLogsCsv(
  options: UsageLogExportOptions
): Promise<number> {
  options.signal?.throwIfAborted()
  const stream = await options.fetchExport()
  const reader = stream.getReader()
  const parts = ['\uFEFF', `${options.headers.map(csvCell).join(',')}\r\n`]
  const decoder = new TextDecoder()
  let buffer = ''
  let csvChunk = ''
  let exported = 0
  let complete = false
  // Cancelling a pending read also closes the server's database cursor promptly.
  const cancelReader = () => {
    void reader.cancel().catch(() => undefined)
  }
  options.signal?.addEventListener('abort', cancelReader, { once: true })

  try {
    while (true) {
      options.signal?.throwIfAborted()
      const { done, value } = await reader.read()
      options.signal?.throwIfAborted()
      buffer += decoder.decode(value, { stream: !done })
      if (done) buffer += '\n'
      let start = 0
      for (
        let end = buffer.indexOf('\n');
        end !== -1;
        end = buffer.indexOf('\n', start)
      ) {
        const line = buffer.slice(start, end).trim()
        start = end + 1
        if (!line) continue
        if (complete) {
          throw new Error('Unexpected data after log export completion')
        }
        const item = JSON.parse(line) as UsageLogExportRecord & {
          complete?: boolean
          count?: number
        }
        if (item?.complete === true) {
          if (item.count !== exported) {
            throw new Error('Log export count mismatch')
          }
          complete = true
          continue
        }
        if (!item || !Number.isFinite(item.created_at)) {
          throw new Error('Invalid log export record')
        }
        csvChunk += `${usageLogCsvRow(item, options)}\r\n`
        exported++
        if (csvChunk.length >= 65536) {
          parts.push(csvChunk)
          csvChunk = ''
        }
      }
      buffer = buffer.slice(start)
      if (done) break
    }
  } finally {
    options.signal?.removeEventListener('abort', cancelReader)
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
  options.signal?.throwIfAborted()
  if (!complete) throw new Error('Log export was interrupted')
  if (exported === 0) return 0
  if (csvChunk) parts.push(csvChunk)

  const url = URL.createObjectURL(
    new Blob(parts, { type: 'text/csv;charset=utf-8' })
  )
  const link = document.createElement('a')
  link.href = url
  link.download = options.filename
  document.body.appendChild(link)
  try {
    link.click()
  } finally {
    link.remove()
    // Give the browser time to start reading the download before releasing it.
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return exported
}
