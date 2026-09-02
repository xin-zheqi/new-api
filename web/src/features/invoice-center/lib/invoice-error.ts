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
import { isAxiosError } from 'axios'
import type { TFunction } from 'i18next'

function readServerMessage(error: unknown): string | undefined {
  if (isAxiosError(error)) {
    const data = error.response?.data
    if (
      typeof data === 'object' &&
      data !== null &&
      'message' in data &&
      typeof data.message === 'string'
    ) {
      return data.message
    }
  }
  return error instanceof Error && error.message ? error.message : undefined
}

export function getInvoiceErrorMessage(
  error: unknown,
  t: TFunction,
  fallback: string
): string {
  const message = readServerMessage(error)
  if (!message) return t(fallback)

  const applicationDayMatch =
    /^invoice applications are accepted on day (\d{1,2}) of each month$/.exec(
      message
    )
  if (applicationDayMatch) {
    return t('Invoice applications are accepted on day {{day}} of each month', {
      day: Number(applicationDayMatch[1]),
    })
  }

  if (
    message.trim().toLowerCase() ===
    'invoice center is only available for university or enterprise users'
  ) {
    return t(
      'Invoice center is only available for university or enterprise users.'
    )
  }

  if (
    message.trim().toLowerCase() ===
    'subscriptions in one invoice application must use the same currency'
  ) {
    return t('Each invoice application can include only one currency.')
  }

  return t(message, { defaultValue: t(fallback) })
}
