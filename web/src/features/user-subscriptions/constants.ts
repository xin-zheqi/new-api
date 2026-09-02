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
import type { TFunction } from 'i18next'
import type { StatusVariant } from '@/components/status-badge'
import type { UserSubscriptionEffectiveStatus } from './types'

export const USER_SUBSCRIPTION_STATUSES = [
  'active',
  'exhausted',
  'expired',
  'cancelled',
] as const

export const USER_SUBSCRIPTION_SOURCES = [
  'order',
  'balance',
  'redemption',
  'admin',
] as const

export function getSubscriptionStatusMeta(
  status: string,
  t: TFunction
): { label: string; variant: StatusVariant } {
  switch (status as UserSubscriptionEffectiveStatus) {
    case 'active':
      return { label: t('Usable'), variant: 'success' }
    case 'exhausted':
      return { label: t('Used Up'), variant: 'warning' }
    case 'expired':
      return { label: t('Expired'), variant: 'neutral' }
    case 'cancelled':
      return { label: t('Invalidated'), variant: 'neutral' }
    default:
      return { label: status || '-', variant: 'neutral' }
  }
}

export function getSubscriptionSourceLabel(source: string, t: TFunction) {
  switch (source) {
    case 'order':
      return t('Order Purchase')
    case 'balance':
      return t('Balance Purchase')
    case 'redemption':
      return t('Redemption')
    case 'admin':
      return t('Admin Grant')
    default:
      return source || '-'
  }
}

export function getSubscriptionStatusOptions(t: TFunction) {
  return USER_SUBSCRIPTION_STATUSES.map((value) => ({
    value,
    label: getSubscriptionStatusMeta(value, t).label,
  }))
}

export function getSubscriptionSourceOptions(t: TFunction) {
  return USER_SUBSCRIPTION_SOURCES.map((value) => ({
    value,
    label: getSubscriptionSourceLabel(value, t),
  }))
}

export function canInvalidateSubscription(status: string) {
  return status === 'active' || status === 'exhausted'
}
