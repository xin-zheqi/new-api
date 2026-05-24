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
import { api } from '@/lib/api'
import type {
  ApiResponse,
  PlanRecord,
  SubscriptionPlan,
  PlanPayload,
  UserSubscriptionRecord,
  CreateUserSubscriptionRequest,
  SubscriptionPayResponse,
  SubscriptionPayRequest,
  SelfSubscriptionData,
} from './types'

function toNumber(value: unknown, fallback = 0): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) {
      return parsed
    }
  }
  return fallback
}

function toStringValue(value: unknown, fallback = ''): string {
  if (typeof value === 'string') {
    return value
  }
  return fallback
}

function toBoolean(value: unknown, fallback = false): boolean {
  if (typeof value === 'boolean') {
    return value
  }
  if (typeof value === 'number') {
    return value !== 0
  }
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    if (normalized === 'true' || normalized === '1') {
      return true
    }
    if (normalized === 'false' || normalized === '0') {
      return false
    }
  }
  return fallback
}

function normalizeSubscriptionPlan(value: unknown): SubscriptionPlan | null {
  if (!value || typeof value !== 'object') {
    return null
  }
  const candidate = value as Record<string, unknown>
  const id = toNumber(candidate.id, 0)
  const title = toStringValue(candidate.title).trim()
  if (id <= 0 || title === '') {
    return null
  }
  return {
    id,
    title,
    subtitle: toStringValue(candidate.subtitle),
    price_amount: toNumber(candidate.price_amount, 0),
    currency: toStringValue(candidate.currency, 'USD') || 'USD',
    duration_unit:
      (toStringValue(candidate.duration_unit, 'month') as SubscriptionPlan['duration_unit']) ||
      'month',
    duration_value: toNumber(candidate.duration_value, 1),
    custom_seconds: toNumber(candidate.custom_seconds, 0),
    quota_reset_period:
      (toStringValue(candidate.quota_reset_period, 'never') as SubscriptionPlan['quota_reset_period']) ||
      'never',
    quota_reset_custom_seconds: toNumber(
      candidate.quota_reset_custom_seconds,
      0
    ),
    enabled: toBoolean(candidate.enabled, true),
    sort_order: toNumber(candidate.sort_order, 0),
    max_purchase_per_user: toNumber(candidate.max_purchase_per_user, 0),
    total_amount: toNumber(candidate.total_amount, 0),
    upgrade_group: toStringValue(candidate.upgrade_group),
    stripe_price_id: toStringValue(candidate.stripe_price_id),
    creem_product_id: toStringValue(candidate.creem_product_id),
    waffo_pancake_product_id: toStringValue(
      candidate.waffo_pancake_product_id
    ),
  }
}

export function normalizePlanRecords(data: unknown): PlanRecord[] {
  if (Array.isArray(data)) {
    return data.flatMap((item) => {
      if (item && typeof item === 'object' && 'plan' in item) {
        const plan = normalizeSubscriptionPlan(
          (item as Record<string, unknown>).plan
        )
        if (plan) {
          return [{ plan }]
        }
      }
      const plan = normalizeSubscriptionPlan(item)
      if (plan) {
        return [{ plan }]
      }
      return []
    })
  }
  if (data && typeof data === 'object') {
    const candidate = data as Record<string, unknown>
    if (Array.isArray(candidate.items)) {
      return normalizePlanRecords(candidate.items)
    }
    if (Array.isArray(candidate.plans)) {
      return normalizePlanRecords(candidate.plans)
    }
  }
  return []
}

// ============================================================================
// Admin Plan Management
// ============================================================================

export async function getAdminPlans(): Promise<ApiResponse<PlanRecord[]>> {
  const res = await api.get('/api/subscription/admin/plans')
  return {
    ...res.data,
    data: normalizePlanRecords(res.data?.data),
  }
}

export async function createPlan(
  data: PlanPayload
): Promise<ApiResponse<PlanRecord>> {
  const res = await api.post('/api/subscription/admin/plans', data)
  return res.data
}

export async function updatePlan(
  id: number,
  data: PlanPayload
): Promise<ApiResponse<PlanRecord>> {
  const res = await api.put(`/api/subscription/admin/plans/${id}`, data)
  return res.data
}

export async function patchPlanStatus(
  id: number,
  enabled: boolean
): Promise<ApiResponse> {
  const res = await api.patch(`/api/subscription/admin/plans/${id}`, {
    enabled,
  })
  return res.data
}

// ============================================================================
// Admin User Subscription Management
// ============================================================================

export async function getUserSubscriptions(
  userId: number
): Promise<ApiResponse<UserSubscriptionRecord[]>> {
  const res = await api.get(
    `/api/subscription/admin/users/${userId}/subscriptions`
  )
  return res.data
}

export async function createUserSubscription(
  userId: number,
  data: CreateUserSubscriptionRequest
): Promise<ApiResponse<{ message?: string }>> {
  const res = await api.post(
    `/api/subscription/admin/users/${userId}/subscriptions`,
    data
  )
  return res.data
}

export async function invalidateUserSubscription(
  subId: number
): Promise<ApiResponse<{ message?: string }>> {
  const res = await api.post(
    `/api/subscription/admin/user_subscriptions/${subId}/invalidate`
  )
  return res.data
}

export async function deleteUserSubscription(
  subId: number
): Promise<ApiResponse> {
  const res = await api.delete(
    `/api/subscription/admin/user_subscriptions/${subId}`
  )
  return res.data
}

// ============================================================================
// User-facing Subscription Payment
// ============================================================================

export async function paySubscriptionStripe(
  data: SubscriptionPayRequest
): Promise<SubscriptionPayResponse> {
  const res = await api.post('/api/subscription/stripe/pay', data)
  return res.data
}

export async function paySubscriptionCreem(
  data: SubscriptionPayRequest
): Promise<SubscriptionPayResponse> {
  const res = await api.post('/api/subscription/creem/pay', data)
  return res.data
}

export async function paySubscriptionWaffoPancake(
  data: SubscriptionPayRequest
): Promise<SubscriptionPayResponse> {
  const res = await api.post('/api/subscription/waffo-pancake/pay', data)
  return res.data
}

// Mints a Pancake OnetimeProduct (see controller for the OnetimeProduct vs
// SubscriptionProduct rationale) using persisted creds + StoreID.
export async function createWaffoPancakeSubscriptionProduct(data: {
  name: string
  amount: string
}): Promise<
  ApiResponse<{ product_id: string; product_name: string; store_id: string }>
> {
  const res = await api.post(
    '/api/option/waffo-pancake/subscription-product',
    data
  )
  return res.data
}

// Returns the OnetimeProducts in the saved Pancake store; empty when the
// gateway isn't fully configured.
export async function listWaffoPancakeSubscriptionProductOptions(): Promise<
  ApiResponse<{
    store_id: string
    products: { id: string; name: string; status: string }[]
  }>
> {
  const res = await api.post(
    '/api/option/waffo-pancake/subscription-product-options'
  )
  return res.data
}

export async function paySubscriptionEpay(
  data: SubscriptionPayRequest & { payment_method: string }
): Promise<SubscriptionPayResponse & { url?: string }> {
  const res = await api.post('/api/subscription/epay/pay', data)
  return {
    ...res.data,
    url: res.data.url || (res as unknown as { url?: string }).url,
  }
}

// ============================================================================
// User Self Subscriptions
// ============================================================================

export async function getSelfSubscriptions(): Promise<
  ApiResponse<UserSubscriptionRecord[]>
> {
  const res = await api.get('/api/subscription/self')
  return res.data
}

export async function getSelfSubscriptionFull(): Promise<
  ApiResponse<SelfSubscriptionData>
> {
  const res = await api.get('/api/subscription/self')
  return res.data
}

export async function getPublicPlans(): Promise<ApiResponse<PlanRecord[]>> {
  const res = await api.get('/api/subscription/plans')
  return {
    ...res.data,
    data: normalizePlanRecords(res.data?.data),
  }
}

export async function updateBillingPreference(
  preference: string
): Promise<ApiResponse<{ billing_preference?: string }>> {
  const res = await api.put('/api/subscription/self/preference', {
    billing_preference: preference,
  })
  return res.data
}

export async function getGroups(): Promise<ApiResponse<string[]>> {
  const res = await api.get('/api/group')
  return res.data
}
