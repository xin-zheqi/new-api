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
  AuditPolicy,
  AuditRecord,
  AuditResult,
  PolicyResponse,
} from './types'

type Envelope<T> = { success: boolean; message: string; data: T }
export async function auditRequest<T>(
  path: string,
  method = 'GET',
  data?: unknown
): Promise<T> {
  const response = await api.request<Envelope<T>>({
    url: `/api/content-audit${path}`,
    method,
    data,
    headers: { 'Cache-Control': 'no-store' },
  })
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data
}
export const getAuditPolicy = () => auditRequest<PolicyResponse>('/policy')
export const saveAuditPolicy = (policy: AuditPolicy) =>
  auditRequest('/policy', 'PUT', policy)
export const testAuditPolicy = (policy: AuditPolicy, text: string) =>
  auditRequest<{ result: AuditResult; text: string }>('/test', 'POST', {
    policy,
    text,
  })
export const getAuditRecords = (params: URLSearchParams) =>
  auditRequest<{ items: AuditRecord[]; total: number }>(`/records?${params}`)
