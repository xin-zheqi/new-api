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
import { isHttpUrl } from './content-format'

export function getSafeHttpCheckoutUrl(value: unknown): string | null {
  if (typeof value !== 'string') return null

  const trimmed = value.trim()
  if (!trimmed || !isHttpUrl(trimmed)) return null

  return new URL(trimmed).href
}

export function openHttpCheckoutUrl(value: unknown): boolean {
  const url = getSafeHttpCheckoutUrl(value)
  if (!url) return false

  window.open(url, '_blank', 'noopener,noreferrer')
  return true
}

export function redirectToHttpCheckoutUrl(value: unknown): boolean {
  const url = getSafeHttpCheckoutUrl(value)
  if (!url) return false

  window.location.href = url
  return true
}

export function submitHttpCheckoutForm(
  value: unknown,
  params: Record<string, unknown>
): boolean {
  const url = getSafeHttpCheckoutUrl(value)
  if (!url) return false

  const form = document.createElement('form')
  form.action = url
  form.method = 'POST'

  const isSafari =
    navigator.userAgent.includes('Safari') &&
    !navigator.userAgent.includes('Chrome')
  if (!isSafari) {
    form.target = '_blank'
    form.rel = 'noopener noreferrer'
  }

  Object.entries(params).forEach(([key, paramValue]) => {
    const input = document.createElement('input')
    input.type = 'hidden'
    input.name = key
    input.value = String(paramValue)
    form.appendChild(input)
  })

  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
  return true
}
