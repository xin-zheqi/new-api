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
  other?: string
}

export interface UsageLogExportPage {
  items: UsageLogExportRecord[] | null
  page: number
  page_size: number
  total: number
}

export interface UsageLogExportOptions {
  fetchPage: (page: number, pageSize: number) => Promise<UsageLogExportPage>
  headers: readonly string[]
  formatAmount: (quota: number) => number
  formatTime: (timestamp: number) => string
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

export function usageLogCsvRow(
  log: UsageLogExportRecord,
  options: Pick<UsageLogExportOptions, 'formatAmount' | 'formatTime'>
): string {
  let other: Record<string, unknown> = {}
  try {
    const parsed: unknown = JSON.parse(log.other || '{}')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      other = parsed as Record<string, unknown>
    }
  } catch {
    // Older/error logs may have no valid metadata; their token columns are 0.
  }
  const write5m = tokenCount(other.cache_creation_tokens_5m)
  const write1h = tokenCount(other.cache_creation_tokens_1h)
  const cacheWrite =
    write5m > 0 || write1h > 0
      ? write5m + write1h
      : tokenCount(other.cache_creation_tokens)
  // Excel keeps 15 significant digits. Remove floating-point conversion noise
  // without rounding small charges to zero or changing stored billing values.
  const amount = Number(options.formatAmount(log.quota || 0).toPrecision(15))

  return [
    log.username || '',
    log.token_name || '',
    log.model_name || '',
    tokenCount(log.prompt_tokens),
    tokenCount(log.completion_tokens),
    tokenCount(other.cache_tokens),
    cacheWrite,
    amount,
    options.formatTime(log.created_at),
  ]
    .map(csvCell)
    .join(',')
}

/** Fetch all filtered pages before downloading, never a partial report. */
export async function exportUsageLogsCsv(
  options: UsageLogExportOptions
): Promise<number> {
  const pageSize = 100 // Matches the existing log API's maximum page size.
  const parts = ['\uFEFF', `${options.headers.map(csvCell).join(',')}\r\n`]
  let total: number | undefined
  let exported = 0

  for (let page = 1; ; page++) {
    options.signal?.throwIfAborted()
    const data = await options.fetchPage(page, pageSize)
    options.signal?.throwIfAborted()
    const items = data.items ?? []
    if (
      !Number.isSafeInteger(data.total) ||
      data.total < 0 ||
      data.page !== page ||
      data.page_size !== pageSize ||
      !Array.isArray(items) ||
      (total !== undefined && data.total !== total)
    ) {
      throw new Error('Log export pagination changed')
    }
    total = data.total
    if (items.length !== Math.min(pageSize, total - exported)) {
      throw new Error('Log export page is incomplete')
    }
    if (total === 0) return 0
    // Keep only CSV chunks in memory; do not retain whole log histories.
    parts.push(
      `${items.map((log) => usageLogCsvRow(log, options)).join('\r\n')}\r\n`
    )
    exported += items.length
    if (exported === total) break
  }

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
