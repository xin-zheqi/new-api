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
import { describe, expect, it } from 'vitest'

import { createInvoiceApplicationSchema } from '../invoice-form'

const validApplication = {
  invoice_title: 'Example University',
  taxpayer_id: '91310000123456789X',
  bank_name: '',
  remark: '',
  subscription_ids: [1],
}

describe('invoice application validation', () => {
  it('requires a taxpayer ID before submission', () => {
    const result = createInvoiceApplicationSchema().safeParse({
      ...validApplication,
      taxpayer_id: '',
    })

    expect(result.success).toBe(false)
  })

  it('requires at least one eligible invoice item', () => {
    const result = createInvoiceApplicationSchema().safeParse({
      ...validApplication,
      subscription_ids: [],
    })

    expect(result.success).toBe(false)
  })
})
