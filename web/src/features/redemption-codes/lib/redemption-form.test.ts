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

import { transformFormDataToPayload } from './redemption-form'

describe('redemption form payload', () => {
  test('never marks a subscription redemption as invoice eligible', () => {
    const payload = transformFormDataToPayload({
      name: 'Subscription gift',
      redeem_type: 'subscription',
      quota_dollars: 0,
      subscription_plan_id: 42,
      count: 1,
      invoice_amount: 0,
      invoice_currency: 'CNY',
    })

    expect(payload.invoice_eligible).toBe(false)
    expect(payload.subscription_plan_id).toBe(42)
  })
})
