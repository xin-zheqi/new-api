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
export type LotteryMode = 'once' | 'scheduled'

export type LotterySettings = {
  enabled: boolean
}

export type LotteryEligibilityIssue = {
  code: string
  message: string
  required_amount?: number
  current_amount?: number
  remaining_amount?: number
  window_days?: number
  count_redemption_as_recharge?: boolean
}

export type LotteryEligibilityStatus = {
  eligible: boolean
  issues?: LotteryEligibilityIssue[]
}

export type LotteryRound = {
  id: number
  lottery_id: number
  round_key: string
  status: string
  registration_start: number
  registration_end: number
  draw_time: number
  drawn_at: number
}

export type LotteryParticipant = {
  id: number
  masked_name: string
  joined_at: number
  is_winner?: boolean
}

export type LotteryWinner = {
  user_id?: number
  username?: string
  masked_name: string
  won_at: number
  prizes?: string[]
  prize_details?: LotteryWinnerPrize[]
}

export type LotteryWinnerPrize = {
  id: number
  prize_name: string
  code: string
}

export type LotteryRoundDetail = {
  round: LotteryRound
  participant_count: number
  winners?: LotteryWinner[]
}

export type LotteryActivity = {
  id: number
  title: string
  description: string
  prize_name: string
  mode: LotteryMode
  status: number
  winner_count: number
  prize_per_winner: number
  require_recharge: boolean
  min_recharge_amount: number
  recharge_window_days: number
  count_redemption_as_recharge: boolean
  min_account_age_days: number
  min_request_count: number
  require_email_verified: boolean
  schedule_weekdays?: number[]
  schedule_start_time?: string
  schedule_end_time?: string
  schedule_draw_time?: string
  round?: LotteryRound
  participant_count: number
  participants: LotteryParticipant[]
  joined: boolean
  won: boolean
  eligibility?: LotteryEligibilityStatus
  can_edit?: boolean
  winners?: LotteryWinner[]
  rounds?: LotteryRoundDetail[]
  assigned_prize_count?: number
  available_prize_count?: number
  prize_codes?: string[]
  created_at: number
  deleted?: boolean
}

export type LotteryDrawStatusFilter = 'all' | 'undrawn' | 'drawn'

export type LotteryPrize = {
  id: number
  lottery_id: number
  round_id: number
  title: string
  prize_name: string
  code: string
  won_at: number
  draw_time: number
}

export type PageResponse<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type ApiResponse<T> = {
  success: boolean
  message: string
  data: T
}

export type CreateLotteryPayload = {
  title: string
  description: string
  prize_name: string
  mode: LotteryMode
  winner_count: number
  prize_per_winner: number
  require_recharge: boolean
  min_recharge_amount: number
  recharge_window_days: number
  count_redemption_as_recharge: boolean
  min_account_age_days: number
  min_request_count: number
  require_email_verified: boolean
  registration_start: number
  registration_end: number
  draw_time: number
  schedule_weekdays: number[]
  schedule_start_time: string
  schedule_end_time: string
  schedule_draw_time: string
  prize_codes: string[]
}
