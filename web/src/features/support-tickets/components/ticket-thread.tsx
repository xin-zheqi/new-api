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
import { MessageQuestionIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Conversation,
  ConversationContent,
  ConversationScrollButton,
} from '@/components/ai-elements/conversation'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { formatTicketTime } from '../lib/ticket-form'
import type { TicketDetail, TicketMessage } from '../types'
import { TicketAttachmentImage } from './ticket-attachment'
import { TicketStatusBadge } from './ticket-status-badge'

function TicketMessageItem(props: {
  ticket: TicketDetail
  message: TicketMessage
  viewerRole: 'user' | 'admin'
}) {
  const { t } = useTranslation()
  const isOwn = props.message.sender_role === props.viewerRole
  const userName =
    props.ticket.display_name ||
    props.ticket.username ||
    t('User #{{id}}', { id: props.ticket.user_id })
  let authorName = userName
  if (isOwn) {
    authorName = t('You')
  } else if (props.message.sender_role === 'admin') {
    authorName = t('Support')
  }
  const userInitials = [...userName].slice(0, 2).join('').toUpperCase()

  return (
    <article
      className={cn(
        'flex items-end gap-2 [content-visibility:auto] [contain-intrinsic-size:0_96px]',
        isOwn && 'flex-row-reverse'
      )}
    >
      <Avatar className='ring-border size-8 shrink-0 ring-1'>
        <AvatarFallback>
          {props.message.sender_role === 'admin' ? 'S' : userInitials}
        </AvatarFallback>
      </Avatar>
      <div
        className={cn(
          'flex min-w-0 max-w-[85%] flex-col gap-1',
          isOwn && 'items-end'
        )}
      >
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-2 text-xs'>
          <span className='font-medium'>{authorName}</span>
          <time>{formatTicketTime(props.message.created_at)}</time>
        </div>
        <div
          className={cn(
            'min-w-0 rounded-md px-3 py-2 text-sm',
            isOwn ? 'bg-primary text-primary-foreground' : 'bg-muted'
          )}
        >
          <p className='[overflow-wrap:anywhere] break-words whitespace-pre-wrap'>
            {props.message.content}
          </p>
        </div>
        {props.message.attachment && (
          <TicketAttachmentImage
            ticketId={props.ticket.id}
            attachment={props.message.attachment}
          />
        )}
      </div>
    </article>
  )
}

export function TicketThread(props: {
  ticket: TicketDetail | null | undefined
  isLoading?: boolean
  viewerRole: 'user' | 'admin'
  actions?: ReactNode
  composer?: ReactNode
  className?: string
}) {
  const { t } = useTranslation()

  if (props.isLoading) {
    return (
      <div
        className={cn(
          'flex size-full flex-col rounded-md border',
          props.className
        )}
      >
        <div className='flex items-center justify-between gap-3 border-b p-4'>
          <Skeleton className='h-6 w-56 max-w-full' />
          <Skeleton className='h-6 w-24' />
        </div>
        <div className='flex flex-1 flex-col gap-5 overflow-hidden p-4'>
          <Skeleton className='h-20 w-3/4' />
          <Skeleton className='ms-auto h-24 w-3/4' />
          <Skeleton className='h-16 w-2/3' />
        </div>
      </div>
    )
  }

  if (!props.ticket) {
    return (
      <Empty className={cn('size-full rounded-md border', props.className)}>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={MessageQuestionIcon} />
          </EmptyMedia>
          <EmptyTitle>{t('Select a ticket')}</EmptyTitle>
          <EmptyDescription>
            {t('Choose a ticket to view its conversation.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  const ticket = props.ticket

  return (
    <section
      className={cn(
        'bg-background flex size-full min-h-0 min-w-0 flex-col overflow-hidden rounded-md border',
        props.className
      )}
    >
      <header className='border-border/70 flex shrink-0 flex-col gap-2 border-b px-3 py-3 sm:px-4'>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
          <div className='min-w-0 flex-1'>
            <h2 className='text-sm font-semibold [overflow-wrap:anywhere] break-words sm:text-base'>
              {ticket.title}
            </h2>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Ticket #{{id}}', { id: ticket.id })} ·{' '}
              {formatTicketTime(ticket.created_at)}
            </p>
          </div>
          <TicketStatusBadge
            status={ticket.status}
            audience={props.viewerRole}
          />
        </div>
        {props.actions && (
          <div className='flex flex-wrap items-center gap-2'>
            {props.actions}
          </div>
        )}
      </header>

      <Conversation>
        <ConversationContent className='flex flex-col gap-5 p-3 sm:p-4'>
          {ticket.messages.map((message) => (
            <TicketMessageItem
              key={message.id}
              ticket={ticket}
              message={message}
              viewerRole={props.viewerRole}
            />
          ))}
        </ConversationContent>
        <ConversationScrollButton />
      </Conversation>
      {props.composer}
    </section>
  )
}
