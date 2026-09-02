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

import { TICKET_IMAGE_MAX_SIZE, TICKET_IMAGE_TYPES } from './constants'
import { TicketApiError } from './lib/ticket-error'
import type {
  AdminTicketListParams,
  TicketApiResponse,
  TicketCreatePayload,
  TicketDetail,
  TicketListData,
  TicketWritePayload,
} from './types'

function ticketFormData(payload: TicketWritePayload): FormData {
  const form = new FormData()
  form.append('content', payload.content)
  if (payload.image) form.append('image', payload.image)
  return form
}

function requireTicketData<T>(
  response: TicketApiResponse<T>,
  fallbackMessage: string
): T {
  if (
    !response.success ||
    response.data === undefined ||
    response.data === null
  ) {
    throw new TicketApiError(response.code, fallbackMessage)
  }
  return response.data
}

export async function getMyTickets(
  page: number,
  pageSize: number
): Promise<TicketListData> {
  const response = await api.get<TicketApiResponse<TicketListData>>(
    '/api/ticket/self',
    {
      params: { p: page, page_size: pageSize },
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  return requireTicketData(response.data, 'Failed to load support tickets')
}

export async function getTicket(id: number): Promise<TicketDetail> {
  const response = await api.get<TicketApiResponse<TicketDetail>>(
    `/api/ticket/${id}`,
    { skipBusinessError: true, skipErrorHandler: true }
  )
  return requireTicketData(response.data, 'Failed to load support ticket')
}

export async function createTicket(
  payload: TicketCreatePayload
): Promise<TicketDetail> {
  const form = ticketFormData(payload)
  form.append('title', payload.title)
  const response = await api.post<TicketApiResponse<TicketDetail>>(
    '/api/ticket',
    form,
    { skipBusinessError: true, skipErrorHandler: true }
  )
  return requireTicketData(response.data, 'Failed to create support ticket')
}

export async function replyToTicket(
  id: number,
  payload: TicketWritePayload
): Promise<TicketDetail> {
  const response = await api.post<TicketApiResponse<TicketDetail>>(
    `/api/ticket/${id}/reply`,
    ticketFormData(payload),
    { skipBusinessError: true, skipErrorHandler: true }
  )
  return requireTicketData(response.data, 'Failed to send reply')
}

export async function getAdminTickets(
  params: AdminTicketListParams
): Promise<TicketListData> {
  const response = await api.get<TicketApiResponse<TicketListData>>(
    '/api/ticket/admin',
    {
      params: {
        p: params.page,
        page_size: params.page_size,
        status: params.status,
        keyword: params.keyword,
        user_id: params.user_id,
      },
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  return requireTicketData(response.data, 'Failed to load support tickets')
}

export async function getAdminTicket(id: number): Promise<TicketDetail> {
  const response = await api.get<TicketApiResponse<TicketDetail>>(
    `/api/ticket/admin/${id}`,
    { skipBusinessError: true, skipErrorHandler: true }
  )
  return requireTicketData(response.data, 'Failed to load support ticket')
}

export async function replyToTicketAsAdmin(
  id: number,
  payload: TicketWritePayload
): Promise<TicketDetail> {
  const response = await api.post<TicketApiResponse<TicketDetail>>(
    `/api/ticket/admin/${id}/reply`,
    ticketFormData(payload),
    { skipBusinessError: true, skipErrorHandler: true }
  )
  return requireTicketData(response.data, 'Failed to send reply')
}

export async function closeTicket(id: number): Promise<TicketDetail> {
  const response = await api.post<TicketApiResponse<TicketDetail>>(
    `/api/ticket/admin/${id}/close`,
    undefined,
    { skipBusinessError: true, skipErrorHandler: true }
  )
  return requireTicketData(response.data, 'Failed to close support ticket')
}

export async function getTicketAttachment(
  ticketId: number,
  attachmentId: number,
  signal?: AbortSignal
): Promise<Blob> {
  const response = await api.get(
    `/api/ticket/${ticketId}/attachment/${attachmentId}`,
    {
      responseType: 'blob',
      skipBusinessError: true,
      skipErrorHandler: true,
      signal,
    }
  )
  const blob = response.data as Blob
  const mimeType = blob.type.split(';', 1)[0].trim().toLowerCase()
  if (blob.size < 1 || blob.size > TICKET_IMAGE_MAX_SIZE) {
    throw new TicketApiError(
      'ticket_image_size_invalid',
      'Invalid attachment size'
    )
  }
  if (!TICKET_IMAGE_TYPES.some((type) => type === mimeType)) {
    throw new TicketApiError(
      'ticket_image_mime_mismatch',
      'Invalid attachment type'
    )
  }
  return blob
}
