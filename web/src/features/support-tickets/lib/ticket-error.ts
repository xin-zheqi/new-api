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
import { AxiosError } from 'axios'
import type { TFunction } from 'i18next'

import type { TicketErrorCode } from '../types'

const TICKET_ERROR_KEYS: Record<TicketErrorCode, string> = {
  ticket_not_found: 'Ticket not found.',
  ticket_active_exists: 'You already have an open ticket.',
  ticket_waiting_admin: 'Waiting for support to reply.',
  ticket_waiting_user: 'Waiting for the user to reply.',
  ticket_closed: 'This ticket is closed.',
  ticket_message_limit: 'This ticket has reached the message limit.',
  ticket_attachment_limit:
    'This ticket has reached the attachment storage limit.',
  ticket_state_changed: 'Ticket status changed. Refresh and try again.',
  ticket_invalid_filter: 'Invalid ticket filter.',
  ticket_user_id_invalid: 'Enter a valid user ID.',
  ticket_request_too_large: 'The request is too large.',
  ticket_invalid_multipart: 'Invalid upload request.',
  ticket_invalid_fields: 'Check the required fields and try again.',
  ticket_missing_fields: 'Check the required fields and try again.',
  ticket_image_count_invalid: 'Only one image can be attached.',
  ticket_title_invalid: 'Enter a valid ticket subject.',
  ticket_content_invalid: 'Enter a valid ticket message.',
  ticket_image_size_invalid: 'Image size must not exceed 5 MB.',
  ticket_image_name_invalid: 'Invalid image file name.',
  ticket_image_type_invalid: 'Only JPEG, PNG, and WebP images are supported.',
  ticket_image_dimensions_invalid: 'Image dimensions are invalid.',
  ticket_image_extension_mismatch:
    'Only JPEG, PNG, and WebP images are supported.',
  ticket_image_mime_mismatch: 'Only JPEG, PNG, and WebP images are supported.',
  ticket_image_busy: 'Image processing is busy. Please try again.',
  ticket_request_failed: 'Ticket request failed. Please try again.',
  ticket_operation_failed: 'Ticket request failed. Please try again.',
}

export class TicketApiError extends Error {
  code?: string

  constructor(code: string | undefined, fallbackMessage: string) {
    super(fallbackMessage)
    this.name = 'TicketApiError'
    this.code = code
  }
}

function getErrorCode(error: unknown): string | undefined {
  if (error instanceof TicketApiError) return error.code
  if (!(error instanceof AxiosError)) return undefined

  const responseData = error.response?.data
  if (!responseData || typeof responseData !== 'object') return undefined
  const code = (responseData as { code?: unknown }).code
  return typeof code === 'string' ? code : undefined
}

export function getTicketErrorMessage(
  error: unknown,
  t: TFunction,
  fallbackKey = 'Ticket request failed. Please try again.'
): string {
  if (error instanceof AxiosError && error.response?.status === 429) {
    return t('Too many requests. Please try again later.')
  }
  const code = getErrorCode(error) as TicketErrorCode | undefined
  const key = code ? TICKET_ERROR_KEYS[code] : undefined
  return t(key ?? fallbackKey)
}
