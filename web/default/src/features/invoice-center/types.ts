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
export type InvoiceStatus = 'pending' | 'completed' | 'rejected'

export type InvoiceSubscription = {
  id: number
  plan_title: string
  paid_amount_micros: number
  paid_currency: string
  start_time: number
  end_time: number
  created_at: number
  source?: string
  top_up_id?: number
  redemption_id?: number
  item_type?: 'subscription' | 'top_up' | 'redemption_recharge'
}

export type InvoiceItem = {
  id: number
  invoice_application_id?: number
  user_subscription_id: number
  top_up_id?: number
  redemption_id?: number
  item_type?: 'subscription' | 'top_up' | 'redemption_recharge'
  plan_title: string
  paid_amount_micros: number
  currency: string
  start_time: number
  end_time: number
}

export type InvoiceApplicationUser = {
  id: number
  username: string
  display_name?: string
  email?: string
  identity?: string
}

export type InvoiceApplication = {
  id: number
  user_id: number
  application_month: string
  invoice_title: string
  taxpayer_id: string
  bank_name: string
  remark: string
  total_amount_micros: number
  currency: string
  status: InvoiceStatus
  pdf_name?: string
  rejection_reason?: string
  rejected_at: number
  rejected_by: number
  created_at: number
  completed_at: number
  updated_at: number
  items: InvoiceItem[]
  user?: InvoiceApplicationUser
}

export type InvoiceCenterData = {
  application_day: number
  application_open: boolean
  identity_eligible: boolean
  lookback_days: number
  monthly_limit: number
  remaining_applications: number
  subscriptions: InvoiceSubscription[]
  applications: InvoiceApplication[]
  applications_total: number
  page: number
  size: number
}

export type InvoiceCenterParams = {
  page: number
  pageSize: number
}

export type InvoiceApplicationPayload = {
  invoice_title: string
  taxpayer_id: string
  bank_name: string
  remark: string
  subscription_ids: number[]
  redemption_ids: number[]
}

export type AdminInvoiceListParams = {
  page: number
  pageSize: number
  keyword?: string
  status?: InvoiceStatus
  userId?: string
}

export type AdminInvoiceList = {
  items: InvoiceApplication[]
  total: number
  page: number
  pageSize: number
}

export type ApiResult<T> = {
  success: boolean
  message: string
  data: T
}
