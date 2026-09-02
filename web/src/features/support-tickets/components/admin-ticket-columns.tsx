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
import { ViewIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { formatTicketTime } from '../lib/ticket-form'
import type { TicketSummary } from '../types'
import { TicketStatusBadge } from './ticket-status-badge'

export function useAdminTicketColumns(
  onOpenTicket: (id: number) => void
): ColumnDef<TicketSummary>[] {
  const { t } = useTranslation()

  return useMemo(
    () => [
      {
        accessorKey: 'id',
        header: t('ID'),
        cell: ({ row }) => (
          <span className='font-mono text-xs tabular-nums'>
            #{row.original.id}
          </span>
        ),
        size: 80,
        meta: { mobileOrder: 10 },
      },
      {
        accessorKey: 'title',
        header: t('Subject'),
        cell: ({ row }) => (
          <button
            type='button'
            className='focus-visible:ring-ring max-w-80 cursor-pointer text-start font-medium [overflow-wrap:anywhere] break-words hover:underline focus-visible:ring-2 focus-visible:outline-none'
            onClick={() => onOpenTicket(row.original.id)}
          >
            {row.original.title}
          </button>
        ),
        enableSorting: false,
        size: 300,
        meta: { mobileTitle: true },
      },
      {
        accessorKey: 'user_id',
        header: t('User'),
        cell: ({ row }) => {
          const displayName =
            row.original.display_name ||
            row.original.username ||
            t('User #{{id}}', { id: row.original.user_id })
          return (
            <div className='flex min-w-36 flex-col gap-0.5'>
              <span className='[overflow-wrap:anywhere] break-words'>
                {displayName}
              </span>
              <span className='text-muted-foreground text-xs'>
                #{row.original.user_id}
                {row.original.email ? ` | ${row.original.email}` : ''}
              </span>
            </div>
          )
        },
        enableSorting: false,
        size: 220,
        meta: { mobileOrder: 20 },
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        cell: ({ row }) => (
          <TicketStatusBadge status={row.original.status} audience='admin' />
        ),
        filterFn: (row, id, value) => value.includes(row.getValue(id)),
        enableSorting: false,
        size: 150,
        meta: { mobileBadge: true },
      },
      {
        accessorKey: 'message_count',
        header: t('Messages'),
        cell: ({ row }) => (
          <span className='tabular-nums'>{row.original.message_count}</span>
        ),
        enableSorting: false,
        size: 90,
        meta: { mobileOrder: 30 },
      },
      {
        accessorKey: 'last_message_at',
        header: t('Last activity'),
        cell: ({ row }) => (
          <time className='text-muted-foreground whitespace-nowrap'>
            {formatTicketTime(row.original.last_message_at)}
          </time>
        ),
        enableSorting: false,
        size: 180,
        meta: { mobileOrder: 40 },
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={t('View ticket')}
            title={t('View ticket')}
            onClick={() => onOpenTicket(row.original.id)}
          >
            <HugeiconsIcon icon={ViewIcon} />
          </Button>
        ),
        enableHiding: false,
        enableSorting: false,
        meta: { pinned: 'right' as const },
      },
    ],
    [onOpenTicket, t]
  )
}
