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

import type { UserSubscriptionRecord } from '@/features/subscriptions/types'

import { SubscriptionPlansCard } from '../subscription-plans-card'

const subscriptionApi = vi.hoisted(() => ({
  getPublicPlans: vi.fn(),
  getSelfSubscriptionFull: vi.fn(),
  updateBillingPreference: vi.fn(),
}))

vi.mock('@/features/subscriptions/api', () => subscriptionApi)
vi.mock(
  '@/features/subscriptions/components/dialogs/subscription-purchase-dialog',
  () => ({ SubscriptionPurchaseDialog: () => null })
)

function createSubscription(
  id: number,
  status: 'active' | 'expired'
): UserSubscriptionRecord {
  const now = Math.floor(Date.now() / 1000)
  return {
    subscription: {
      id,
      user_id: 1,
      plan_id: 1,
      status,
      source: 'order',
      start_time: now - 86_400,
      end_time: status === 'active' ? now + 86_400 : now - 1,
      amount_total: 1_000,
      amount_used: 100,
      next_reset_time: 0,
    },
  }
}

describe('wallet subscription history pagination', () => {
  beforeEach(() => {
    const allSubscriptions = [
      createSubscription(1, 'active'),
      createSubscription(2, 'active'),
      createSubscription(3, 'expired'),
      createSubscription(4, 'expired'),
      createSubscription(5, 'expired'),
      createSubscription(6, 'expired'),
    ]
    subscriptionApi.getPublicPlans.mockResolvedValue({
      success: true,
      data: [],
    })
    subscriptionApi.getSelfSubscriptionFull.mockResolvedValue({
      success: true,
      data: {
        billing_preference: 'subscription_first',
        subscriptions: allSubscriptions.slice(0, 2),
        all_subscriptions: allSubscriptions,
      },
    })
  })

  test('keeps full totals while rendering five records per page', async () => {
    const user = userEvent.setup()
    render(<SubscriptionPlansCard topupInfo={null} mySubscriptionsOnly />)

    expect(await screen.findByText('2 active')).toBeInTheDocument()
    expect(screen.getByText('4 expired')).toBeInTheDocument()
    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
    expect(screen.getByText('Subscription #1')).toBeInTheDocument()
    expect(screen.queryByText('Subscription #6')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Next page' }))

    expect(screen.getByText('Page 2 of 2')).toBeInTheDocument()
    expect(screen.getByText('Subscription #6')).toBeInTheDocument()
    expect(screen.queryByText('Subscription #1')).not.toBeInTheDocument()
  })
})
