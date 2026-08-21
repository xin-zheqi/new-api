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
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { formatInvoiceMoney } from './invoice-form'

describe('formatInvoiceMoney', () => {
  it('formats paid micros as an explicit currency amount', () => {
    const value = formatInvoiceMoney(1_234_567, 'usd', 'en-US')

    assert.match(value, /USD/)
    assert.match(value, /1\.234567/)
  })

  it('does not invent an amount for historical records without a snapshot', () => {
    assert.equal(formatInvoiceMoney(0, 'CNY', 'zh-CN'), '-')
    assert.equal(formatInvoiceMoney(1_000_000, '', 'zh-CN'), '-')
    assert.equal(
      formatInvoiceMoney(Number.MAX_SAFE_INTEGER + 1, 'USD', 'en-US'),
      '-'
    )
    assert.equal(formatInvoiceMoney(1_000_000, 'ZZZ', 'en-US'), '-')
  })
})
