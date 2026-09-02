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
export type TokenModelUsageModel = {
  model_name: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  requests: number
}

export type TokenModelUsageItem = {
  token_id: number
  token_name: string
  key: string
  status: number
  created_time: number
  accessed_time: number
  expired_time: number
  remain_quota: number
  used_quota: number
  unlimited_quota: boolean
  quota: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  requests: number
  model_count: number
  models: TokenModelUsageModel[]
}

export type TokenModelUsageSummary = {
  total_key_count: number
  active_key_count: number
  model_count: number
  quota: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  requests: number
}

export type TokenUsagePage = {
  page: number
  page_size: number
  total: number
  items: TokenModelUsageItem[]
}

export type TokenUsageResponse = {
  success: boolean
  message?: string
  data?: {
    page: TokenUsagePage
    summary: TokenModelUsageSummary
  }
}
