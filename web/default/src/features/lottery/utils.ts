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
export function toUnixSeconds(value: string) {
  if (!value) return 0
  const time = new Date(value).getTime()
  if (!Number.isFinite(time)) return 0
  return Math.floor(time / 1000)
}

export function formatTime(value?: number) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

export function toDateTimeLocal(value?: number) {
  if (!value) return ''
  const date = new Date(value * 1000)
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

export function parsePrizeCodes(value: string) {
  const seen = new Set<string>()
  const codes: string[] = []
  for (const rawCode of value.split(/\r?\n/)) {
    const code = rawCode.trim()
    if (!code || seen.has(code)) continue
    seen.add(code)
    codes.push(code)
  }
  return codes
}

export function isRoundDrawn(status?: string) {
  return status === 'finished' || status === 'insufficient_prizes'
}

export function isRoundUndrawn(status?: string) {
  return status === 'pending' || status === 'open' || status === 'drawing'
}

export function getRoundStatusDotClass(status?: string) {
  if (status === 'open') return 'bg-emerald-500'
  if (status === 'pending') return 'bg-sky-500'
  if (status === 'drawing') return 'bg-amber-500'
  if (status === 'finished') return 'bg-indigo-500'
  if (status === 'insufficient_prizes') return 'bg-destructive'
  if (status === 'cancelled') return 'bg-muted-foreground'
  return 'bg-muted-foreground'
}

export function getRoundStatusLabel(
  status: string,
  t: (key: string) => string
) {
  const map: Record<string, string> = {
    pending: t('Not started'),
    open: t('Registration open'),
    drawing: t('Draw in progress'),
    finished: t('Draw completed'),
    cancelled: t('Cancelled'),
    insufficient_prizes: t('Insufficient prizes'),
  }
  return map[status] ?? status
}

export function weekdayLabel(day: number, t: (key: string) => string) {
  const labels = [
    t('Sunday'),
    t('Monday'),
    t('Tuesday'),
    t('Wednesday'),
    t('Thursday'),
    t('Friday'),
    t('Saturday'),
  ]
  return labels[day] ?? String(day)
}
