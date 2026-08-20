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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import {
  TICKET_CONTENT_MAX_LENGTH,
  TICKET_IMAGE_MAX_SIZE,
  TICKET_IMAGE_TYPES,
  TICKET_TITLE_MAX_LENGTH,
} from '../constants'

export function getTicketCreateSchema(t: TFunction) {
  return z.object({
    title: z
      .string()
      .trim()
      .min(1, t('Required'))
      .refine(
        (value) => [...value].length <= TICKET_TITLE_MAX_LENGTH,
        t('Title must be {{count}} characters or fewer.', {
          count: TICKET_TITLE_MAX_LENGTH,
        })
      ),
    content: z
      .string()
      .trim()
      .min(1, t('Required'))
      .refine(
        (value) => [...value].length <= TICKET_CONTENT_MAX_LENGTH,
        t('Message must be {{count}} characters or fewer.', {
          count: TICKET_CONTENT_MAX_LENGTH,
        })
      ),
  })
}

export function getTicketReplySchema(t: TFunction) {
  return getTicketCreateSchema(t).pick({ content: true })
}

export type TicketCreateFormValues = z.infer<
  ReturnType<typeof getTicketCreateSchema>
>
export type TicketReplyFormValues = z.infer<
  ReturnType<typeof getTicketReplySchema>
>

export function getTicketImageError(file: File, t: TFunction): string | null {
  if (!TICKET_IMAGE_TYPES.some((type) => type === file.type)) {
    return t('Only JPEG, PNG, and WebP images are supported.')
  }
  if (file.size < 1 || file.size > TICKET_IMAGE_MAX_SIZE) {
    return t('Image size must not exceed 5 MB.')
  }
  return null
}

export function truncateTicketText(value: string, maxLength: number): string {
  const characters = [...value]
  return characters.length > maxLength
    ? characters.slice(0, maxLength).join('')
    : value
}

export function formatTicketTime(timestamp: number): string {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

export function formatAttachmentSize(size: number): string {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${Math.ceil(size / 1024)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}
