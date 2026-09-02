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
  AdminInvoiceList,
  AdminInvoiceListParams,
  ApiResult,
  InvoiceApplication,
  InvoiceApplicationPayload,
  InvoiceCenterData,
  InvoiceCenterParams,
} from './types'

function unwrapResult<T>(result: ApiResult<T>, fallbackMessage: string): T {
  if (!result.success) {
    throw new Error(result.message || fallbackMessage)
  }
  return result.data
}

export async function getInvoiceCenter(
  params: InvoiceCenterParams
): Promise<InvoiceCenterData> {
  const response = await api.get<ApiResult<InvoiceCenterData>>(
    '/api/user/invoice',
    {
      params: { p: params.page, size: params.pageSize },
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  return unwrapResult(response.data, 'Failed to load invoice center.')
}

export async function createInvoiceApplication(
  payload: InvoiceApplicationPayload
): Promise<InvoiceApplication> {
  const response = await api.post<ApiResult<InvoiceApplication>>(
    '/api/user/invoice/apply',
    payload,
    { skipBusinessError: true, skipErrorHandler: true }
  )
  return unwrapResult(response.data, 'Failed to submit invoice application.')
}

export async function getAdminInvoices(
  params: AdminInvoiceListParams
): Promise<AdminInvoiceList> {
  const response = await api.get<
    ApiResult<InvoiceApplication[]> & {
      total: number
      page?: number
      size?: number
    }
  >('/api/invoice/admin/applications', {
    skipBusinessError: true,
    skipErrorHandler: true,
    params: {
      p: params.page,
      size: params.pageSize,
      keyword: params.keyword,
      status: params.status,
      user_id: params.userId,
    },
  })
  const items = unwrapResult(
    response.data,
    'Failed to load invoice applications.'
  )
  return {
    items,
    total: response.data.total ?? 0,
    page: response.data.page ?? params.page,
    pageSize: response.data.size ?? params.pageSize,
  }
}

async function runAdminAction(
  request: Promise<{ data: ApiResult<unknown> }>,
  fallbackMessage: string
): Promise<void> {
  const response = await request
  unwrapResult(response.data, fallbackMessage)
}

export async function uploadInvoicePDF(id: number, file: File): Promise<void> {
  const form = new FormData()
  form.append('file', file)
  await runAdminAction(
    api.post(`/api/invoice/admin/applications/${id}/pdf`, form, {
      skipBusinessError: true,
      skipErrorHandler: true,
    }),
    'Failed to upload invoice PDF.'
  )
}

export async function deleteInvoicePDF(id: number): Promise<void> {
  await runAdminAction(
    api.delete(`/api/invoice/admin/applications/${id}/pdf`, {
      skipBusinessError: true,
      skipErrorHandler: true,
    }),
    'Failed to delete invoice PDF.'
  )
}

export async function completeInvoiceApplication(id: number): Promise<void> {
  await runAdminAction(
    api.post(`/api/invoice/admin/applications/${id}/complete`, undefined, {
      skipBusinessError: true,
      skipErrorHandler: true,
    }),
    'Failed to complete invoice application.'
  )
}

export async function rejectInvoiceApplication(
  id: number,
  reason: string
): Promise<void> {
  await runAdminAction(
    api.post(
      `/api/invoice/admin/applications/${id}/reject`,
      { reason },
      { skipBusinessError: true, skipErrorHandler: true }
    ),
    'Failed to reject invoice application.'
  )
}

export async function downloadInvoicePDF(
  id: number,
  audience: 'user' | 'admin'
): Promise<Blob> {
  const path =
    audience === 'admin'
      ? `/api/invoice/admin/applications/${id}/download`
      : `/api/user/invoice/${id}/download`
  const response = await api.get<Blob>(path, {
    responseType: 'blob',
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  return response.data
}
