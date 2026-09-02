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
import { describe, expect, test } from 'vitest'

import { formatPaymentAmount } from './format'

describe('wallet payment amount formatting', () => {
  test('uses the currency recorded with the payment', () => {
    const value = formatPaymentAmount(12.345678, 'cny', 'en-US')

    expect(value).toMatch(/CNY/)
    expect(value).toMatch(/12\.345678/)
  })

  test('does not guess a currency for legacy records', () => {
    expect(formatPaymentAmount(12.34, '', 'en-US')).toBe('-')
    expect(formatPaymentAmount(Number.NaN, 'USD', 'en-US')).toBe('-')
    expect(formatPaymentAmount(12.34, 'USDT', 'en-US')).toBe('-')
    expect(formatPaymentAmount(12.34, 'XXX', 'en-US')).toBe('-')
    expect(formatPaymentAmount(12.34, 'XTS', 'en-US')).toBe('-')
    expect(formatPaymentAmount(12.34, 'ZZZ', 'en-US')).toBe('-')
  })
})
