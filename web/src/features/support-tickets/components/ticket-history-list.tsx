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
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  MessageQuestionIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
import type { TicketSummary } from '../types'
import { TicketStatusBadge } from './ticket-status-badge'

export function TicketHistoryList(props: {
  items: TicketSummary[]
  selectedId: number | null
  page: number
  pageSize: number
  total: number
  isLoading: boolean
  errorMessage?: string
  onSelect: (id: number) => void
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const pageCount = Math.max(1, Math.ceil(props.total / props.pageSize))

  let content: ReactNode
  if (props.isLoading) {
    content = (
      <div className='flex flex-col gap-2 p-2'>
        {[1, 2, 3, 4, 5].map((item) => (
          <Skeleton key={item} className='h-24 w-full' />
        ))}
      </div>
    )
  } else if (props.errorMessage) {
    content = (
      <div className='p-3'>
        <Alert variant='destructive'>
          <AlertDescription>{props.errorMessage}</AlertDescription>
        </Alert>
      </div>
    )
  } else if (props.items.length === 0) {
    content = (
      <Empty className='h-full'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={MessageQuestionIcon} />
          </EmptyMedia>
          <EmptyTitle>{t('No support tickets')}</EmptyTitle>
          <EmptyDescription>
            {t('Your support ticket history will appear here.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else {
    content = (
      <div className='flex flex-col gap-1.5 p-2'>
        {props.items.map((ticket) => (
          <button
            key={ticket.id}
            type='button'
            aria-pressed={props.selectedId === ticket.id}
            className={cn(
              'focus-visible:ring-ring hover:bg-muted/60 flex min-h-24 w-full flex-col gap-2 rounded-md border p-3 text-start transition-colors focus-visible:ring-2 focus-visible:outline-none',
              props.selectedId === ticket.id &&
                'border-primary/50 bg-primary/5 hover:bg-primary/10'
            )}
            onClick={() => props.onSelect(ticket.id)}
          >
            <div className='flex w-full min-w-0 items-start justify-between gap-2'>
              <span className='min-w-0 flex-1 text-sm font-medium [overflow-wrap:anywhere] break-words'>
                {ticket.title}
              </span>
              <TicketStatusBadge status={ticket.status} audience='user' />
            </div>
            <div className='text-muted-foreground flex w-full flex-wrap items-center justify-between gap-x-2 gap-y-1 text-xs'>
              <span>{t('Ticket #{{id}}', { id: ticket.id })}</span>
              <span>
                {t('{{count}} messages', { count: ticket.message_count })}
              </span>
            </div>
            <time className='text-muted-foreground text-xs'>
              {formatTicketTime(ticket.last_message_at)}
            </time>
          </button>
        ))}
      </div>
    )
  }

  return (
    <aside className='bg-background flex size-full min-h-0 flex-col overflow-hidden rounded-md border'>
      <div className='border-border/70 shrink-0 border-b px-3 py-2.5'>
        <h2 className='text-sm font-semibold'>{t('Ticket history')}</h2>
      </div>
      <div className='min-h-0 flex-1 overflow-y-auto'>{content}</div>
      {props.total > props.pageSize ? (
        <div className='border-border/70 flex shrink-0 items-center justify-between gap-2 border-t p-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={props.page <= 1}
            aria-label={t('Previous page')}
            title={t('Previous page')}
            onClick={() => props.onPageChange(props.page - 1)}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} />
          </Button>
          <span className='text-muted-foreground text-xs tabular-nums'>
            {t('Page {{page}} of {{total}}', {
              page: props.page,
              total: pageCount,
            })}
          </span>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={props.page >= pageCount}
            aria-label={t('Next page')}
            title={t('Next page')}
            onClick={() => props.onPageChange(props.page + 1)}
          >
            <HugeiconsIcon icon={ArrowRight01Icon} />
          </Button>
        </div>
      ) : null}
    </aside>
  )
}
