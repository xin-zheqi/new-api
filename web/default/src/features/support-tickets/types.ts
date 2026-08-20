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
export type TicketStatus = 'waiting_admin' | 'waiting_user' | 'closed'

export type TicketErrorCode =
  | 'ticket_not_found'
  | 'ticket_active_exists'
  | 'ticket_waiting_admin'
  | 'ticket_waiting_user'
  | 'ticket_closed'
  | 'ticket_message_limit'
  | 'ticket_state_changed'
  | 'ticket_invalid_filter'
  | 'ticket_user_id_invalid'
  | 'ticket_request_too_large'
  | 'ticket_invalid_multipart'
  | 'ticket_invalid_fields'
  | 'ticket_missing_fields'
  | 'ticket_image_count_invalid'
  | 'ticket_title_invalid'
  | 'ticket_content_invalid'
  | 'ticket_image_size_invalid'
  | 'ticket_image_name_invalid'
  | 'ticket_image_type_invalid'
  | 'ticket_image_dimensions_invalid'
  | 'ticket_image_extension_mismatch'
  | 'ticket_image_mime_mismatch'
  | 'ticket_image_busy'
  | 'ticket_request_failed'
  | 'ticket_operation_failed'

export type TicketAttachment = {
  id: number
  file_name: string
  mime_type: string
  size: number
  width: number
  height: number
}

export type TicketMessage = {
  id: number
  sender_role: 'user' | 'admin'
  content: string
  created_at: number
  attachment?: TicketAttachment | null
}

export type TicketSummary = {
  id: number
  user_id: number
  title: string
  status: TicketStatus
  created_at: number
  updated_at: number
  last_message_at: number
  closed_at: number
  message_count: number
  username?: string
  display_name?: string
  email?: string
}

export type TicketDetail = TicketSummary & {
  messages: TicketMessage[]
}

export type TicketListData = {
  items: TicketSummary[]
  total: number
  page: number
  page_size: number
  active_ticket_id?: number | null
}

export type TicketApiResponse<T> = {
  success: boolean
  code?: TicketErrorCode | string
  message?: string
  data?: T
}

export type TicketWritePayload = {
  content: string
  image?: File | null
}

export type TicketCreatePayload = TicketWritePayload & {
  title: string
}

export type AdminTicketListParams = {
  page: number
  page_size: number
  status?: TicketStatus
  keyword?: string
  user_id?: string
}
