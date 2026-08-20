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
import type { StatusVariant } from '@/components/status-badge'

import type { TicketStatus } from './types'

export const TICKET_TITLE_MAX_LENGTH = 120
export const TICKET_CONTENT_MAX_LENGTH = 4000
export const TICKET_IMAGE_MAX_SIZE = 5 * 1024 * 1024
export const TICKET_MESSAGE_MAX_COUNT = 100
export const TICKET_PAGE_SIZE = 10

export const TICKET_IMAGE_TYPES = [
  'image/jpeg',
  'image/png',
  'image/webp',
] as const

export const TICKET_IMAGE_ACCEPT = TICKET_IMAGE_TYPES.join(',')

export const TICKET_STATUS_VARIANTS: Record<TicketStatus, StatusVariant> = {
  waiting_admin: 'warning',
  waiting_user: 'info',
  closed: 'neutral',
}

export const ADMIN_TICKET_STATUS_OPTIONS: Array<{
  label: string
  value: TicketStatus
}> = [
  { label: 'Needs reply', value: 'waiting_admin' },
  { label: 'Waiting for user', value: 'waiting_user' },
  { label: 'Closed', value: 'closed' },
]

export const ticketQueryKeys = {
  all: ['support-tickets'] as const,
  subject: (subjectId: number) =>
    [...ticketQueryKeys.all, 'subject', subjectId] as const,
  userLists: (subjectId: number) =>
    [...ticketQueryKeys.subject(subjectId), 'user-list'] as const,
  userList: (subjectId: number, page: number, pageSize: number) =>
    [...ticketQueryKeys.userLists(subjectId), page, pageSize] as const,
  details: (subjectId: number) =>
    [...ticketQueryKeys.subject(subjectId), 'detail'] as const,
  detail: (subjectId: number, id: number, audience: 'user' | 'admin') =>
    [...ticketQueryKeys.details(subjectId), audience, id] as const,
  adminLists: (subjectId: number) =>
    [...ticketQueryKeys.subject(subjectId), 'admin-list'] as const,
  adminList: (subjectId: number, params: object) =>
    [...ticketQueryKeys.adminLists(subjectId), params] as const,
  attachment: (subjectId: number, ticketId: number, attachmentId: number) =>
    [
      ...ticketQueryKeys.subject(subjectId),
      'attachment',
      ticketId,
      attachmentId,
    ] as const,
}
