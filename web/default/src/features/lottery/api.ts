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
  CreateLotteryPayload,
  LotteryActivity,
  LotteryDrawStatusFilter,
  LotteryPrize,
  LotterySettings,
  PageResponse,
} from './types'

export async function getLotterySettings() {
  const res = await api.get<ApiResponse<LotterySettings>>(
    '/api/lottery/settings'
  )
  return res.data
}

export async function getLotteries(params?: {
  draw_status?: LotteryDrawStatusFilter
}) {
  const res = await api.get<ApiResponse<LotteryActivity[]>>('/api/lottery/', {
    params,
  })
  return res.data
}

export async function joinLottery(id: number) {
  const res = await api.post<ApiResponse<null>>(`/api/lottery/${id}/join`)
  return res.data
}

export async function getMyLotteryPrizes(params: {
  p: number
  page_size: number
}) {
  const res = await api.get<ApiResponse<PageResponse<LotteryPrize>>>(
    '/api/lottery/my-prizes',
    { params }
  )
  return res.data
}

export async function getAdminLotteries(params: {
  p: number
  page_size: number
  keyword?: string
  mode?: string
  status?: string
  draw_status?: LotteryDrawStatusFilter
}) {
  const res = await api.get<ApiResponse<PageResponse<LotteryActivity>>>(
    '/api/lottery/admin/',
    { params }
  )
  return res.data
}

export async function createLottery(payload: CreateLotteryPayload) {
  const res = await api.post<ApiResponse<LotteryActivity>>(
    '/api/lottery/admin/',
    payload
  )
  return res.data
}

export async function updateLottery(id: number, payload: CreateLotteryPayload) {
  const res = await api.put<ApiResponse<LotteryActivity>>(
    `/api/lottery/admin/${id}`,
    payload
  )
  return res.data
}

export async function updateLotteryStatus(id: number, status: number) {
  const res = await api.patch<ApiResponse<null>>(
    `/api/lottery/admin/${id}/status`,
    { status }
  )
  return res.data
}

export async function drawLotteryRound(roundId: number) {
  const res = await api.post<ApiResponse<null>>(
    `/api/lottery/admin/rounds/${roundId}/draw`
  )
  return res.data
}

export async function deleteLottery(id: number) {
  const res = await api.delete<ApiResponse<null>>(`/api/lottery/admin/${id}`)
  return res.data
}
