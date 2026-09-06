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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { PlanRecord } from '../../../types'
import { SubscriptionPurchaseDialog } from '../subscription-purchase-dialog'

const subscriptionApi = vi.hoisted(() => ({
  paySubscriptionBalance: vi.fn(),
  paySubscriptionCreem: vi.fn(),
  paySubscriptionEpay: vi.fn(),
  paySubscriptionStripe: vi.fn(),
  paySubscriptionWaffoPancake: vi.fn(),
  paySubscriptionWechat: vi.fn(),
}))

vi.mock('../../../api', () => subscriptionApi)

const plan: PlanRecord = {
  plan: {
    id: 1,
    title: 'Monthly Plan',
    subtitle: '',
    price_amount: 2,
    currency: 'USD',
    duration_unit: 'month',
    duration_value: 1,
    custom_seconds: 0,
    quota_reset_period: 'never',
    quota_reset_custom_seconds: 0,
    enabled: true,
    sort_order: 0,
    allow_wallet_overflow: true,
    invoice_eligible: false,
    max_purchase_per_user: 0,
    total_amount: 1_000,
    upgrade_group: '',
    downgrade_group: '',
    stripe_price_id: '',
    creem_product_id: '',
    waffo_pancake_product_id: '',
  },
}

describe('subscription balance payment', () => {
  beforeEach(() => {
    subscriptionApi.paySubscriptionBalance.mockResolvedValue({
      success: true,
      message: 'success',
    })
  })

  test('does not expose balance payment when the global switch is disabled', () => {
    render(
      <SubscriptionPurchaseDialog
        open
        onOpenChange={() => undefined}
        plan={plan}
        enableBalanceSubscription={false}
        userQuota={10_000_000}
      />
    )

    expect(
      screen.queryByRole('button', { name: 'Pay with Balance' })
    ).not.toBeInTheDocument()
  })

  test('shows current balance and purchases when the global switch is enabled', async () => {
    const user = userEvent.setup()
    const onPurchaseSuccess = vi.fn()
    render(
      <SubscriptionPurchaseDialog
        open
        onOpenChange={() => undefined}
        plan={plan}
        enableBalanceSubscription
        userQuota={10_000_000}
        onPurchaseSuccess={onPurchaseSuccess}
      />
    )

    expect(screen.getByText('Required balance')).toBeInTheDocument()
    expect(screen.getByText('Available balance')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Pay with Balance' }))

    expect(subscriptionApi.paySubscriptionBalance).toHaveBeenCalledWith({
      plan_id: 1,
    })
    expect(onPurchaseSuccess).toHaveBeenCalledOnce()
  })

  test('disables balance payment when the wallet cannot cover the plan', () => {
    render(
      <SubscriptionPurchaseDialog
        open
        onOpenChange={() => undefined}
        plan={plan}
        enableBalanceSubscription
        userQuota={999_999}
      />
    )

    expect(screen.getByText('Insufficient balance')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Pay with Balance' })
    ).toBeDisabled()
  })
})
