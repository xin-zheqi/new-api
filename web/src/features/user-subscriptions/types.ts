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
export type UserSubscriptionEffectiveStatus =
  | 'active'
  | 'exhausted'
  | 'expired'
  | 'cancelled'

export interface AdminUserSubscription {
  id: number
  user_id: number
  plan_id: number
  username: string
  display_name: string
  email: string
  plan_title: string
  amount_total: number
  amount_used: number
  amount_remaining: number
  usage_percent: number
  start_time: number
  end_time: number
  status: string
  effective_status: UserSubscriptionEffectiveStatus | string
  source: string
  last_reset_time: number
  next_reset_time: number
  upgrade_group: string
  prev_user_group: string
  created_at: number
  updated_at: number
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetUserSubscriptionsParams {
  p?: number
  page_size?: number
  keyword?: string
  status?: string
  plan_id?: number
  user_id?: number
  source?: string
}

export interface GetUserSubscriptionsResponse {
  success: boolean
  message?: string
  data?: {
    items: AdminUserSubscription[]
    total: number
    page: number
    page_size: number
  }
}

export type UserSubscriptionsDialogType = 'create' | 'details' | 'invalidate'
