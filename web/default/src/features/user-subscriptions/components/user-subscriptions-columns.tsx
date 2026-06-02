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
import { useMemo } from 'react'
import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { Progress } from '@/components/ui/progress'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import {
  getSubscriptionSourceLabel,
  getSubscriptionStatusMeta,
} from '../constants'
import type { AdminUserSubscription } from '../types'
import { UserSubscriptionRowActions } from './user-subscriptions-row-actions'

export function useUserSubscriptionsColumns(): ColumnDef<AdminUserSubscription>[] {
  const { t } = useTranslation()

  return useMemo(
    (): ColumnDef<AdminUserSubscription>[] => [
      {
        accessorKey: 'id',
        id: 'id',
        meta: { label: 'ID', mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title='ID' />
        ),
        cell: ({ row }) => <TableId value={row.original.id} />,
        size: 70,
      },
      {
        id: 'user',
        accessorFn: (row) =>
          `${row.username} ${row.display_name} ${row.email} ${row.user_id}`,
        meta: { label: t('User'), mobileTitle: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('User')} />
        ),
        cell: ({ row }) => {
          const sub = row.original
          const name =
            sub.display_name || sub.username || t('Unknown User')
          return (
            <div className='max-w-[220px]'>
              <div className='truncate font-medium'>{name}</div>
              <div className='text-muted-foreground truncate text-xs'>
                ID: {sub.user_id}
                {sub.email ? ` · ${sub.email}` : ''}
              </div>
            </div>
          )
        },
        size: 220,
      },
      {
        id: 'plan_id',
        accessorFn: (row) => String(row.plan_id),
        meta: { label: t('Plan') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Plan')} />
        ),
        cell: ({ row }) => {
          const sub = row.original
          return (
            <div className='max-w-[180px]'>
              <div className='truncate font-medium'>
                {sub.plan_title || t('Unknown Plan')}
              </div>
              <div className='text-muted-foreground text-xs'>
                ID: {sub.plan_id}
              </div>
            </div>
          )
        },
        size: 180,
      },
      {
        id: 'status',
        accessorFn: (row) => row.effective_status,
        meta: { label: t('Status'), mobileBadge: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Status')} />
        ),
        cell: ({ row }) => {
          const meta = getSubscriptionStatusMeta(
            row.original.effective_status,
            t
          )
          return (
            <StatusBadge
              label={meta.label}
              variant={meta.variant}
              copyable={false}
            />
          )
        },
        size: 100,
      },
      {
        id: 'quota',
        accessorFn: (row) => row.usage_percent,
        meta: { label: t('Quota Usage') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Quota Usage')} />
        ),
        cell: ({ row }) => {
          const sub = row.original
          if (sub.amount_total <= 0) {
            return <span className='text-muted-foreground'>{t('Unlimited')}</span>
          }
          return (
            <div className='min-w-[160px] space-y-1'>
              <div className='text-xs tabular-nums'>
                {formatQuota(sub.amount_used)} / {formatQuota(sub.amount_total)}
              </div>
              <Progress value={sub.usage_percent} className='w-full' />
            </div>
          )
        },
        size: 180,
      },
      {
        id: 'period',
        accessorFn: (row) => row.end_time,
        meta: { label: t('Validity'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Validity')} />
        ),
        cell: ({ row }) => (
          <div className='text-muted-foreground min-w-[150px] text-xs'>
            <div>{t('Start')}: {formatTimestampToDate(row.original.start_time)}</div>
            <div>{t('End')}: {formatTimestampToDate(row.original.end_time)}</div>
          </div>
        ),
        size: 170,
      },
      {
        id: 'next_reset_time',
        accessorFn: (row) => row.next_reset_time,
        meta: { label: t('Reset Time'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Reset Time')} />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground text-xs'>
            {formatTimestampToDate(row.original.next_reset_time)}
          </span>
        ),
        size: 130,
      },
      {
        id: 'source',
        accessorFn: (row) => row.source,
        meta: { label: t('Source'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Source')} />
        ),
        cell: ({ row }) => (
          <StatusBadge
            label={getSubscriptionSourceLabel(row.original.source, t)}
            variant='neutral'
            copyable={false}
          />
        ),
        size: 120,
      },
      {
        id: 'created_at',
        accessorFn: (row) => row.created_at,
        meta: { label: t('Created'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Created')} />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground text-xs'>
            {formatTimestampToDate(row.original.created_at)}
          </span>
        ),
        size: 130,
      },
      {
        id: 'actions',
        cell: ({ row }) => <UserSubscriptionRowActions row={row} />,
        size: 80,
      },
    ],
    [t]
  )
}
