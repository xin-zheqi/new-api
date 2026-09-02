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
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { Progress } from '@/components/ui/progress'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import {
  getSubscriptionSourceLabel,
  getSubscriptionStatusMeta,
} from '../constants'
import type { AdminUserSubscription } from '../types'
import { UserSubscriptionRowActions } from './user-subscriptions-row-actions'

function toFiniteNumber(value: unknown, fallback = 0) {
  const num = Number(value)
  return Number.isFinite(num) ? num : fallback
}

function getRemainingQuotaPercent(sub: AdminUserSubscription) {
  const totalAmount = toFiniteNumber(sub.amount_total)
  if (totalAmount <= 0) return 0
  const remainingAmount = Math.max(0, toFiniteNumber(sub.amount_remaining))
  return Math.min(100, Math.max(0, (remainingAmount / totalAmount) * 100))
}

function QuotaUsageCell({
  sub,
  t,
}: {
  sub: AdminUserSubscription
  t: TFunction
}) {
  const totalAmount = toFiniteNumber(sub.amount_total)
  const usedAmount = toFiniteNumber(sub.amount_used)
  const remainingAmount =
    totalAmount > 0
      ? Math.max(
          0,
          toFiniteNumber(sub.amount_remaining, totalAmount - usedAmount)
        )
      : 0

  if (totalAmount <= 0) {
    return (
      <Tooltip>
        <TooltipTrigger
          render={
            <span className='text-muted-foreground cursor-help'>
              {t('Unlimited')}
            </span>
          }
        />
        <TooltipContent className='max-w-none'>
          <div className='space-y-1 text-xs'>
            <div>
              {t('Used Quota')}: {formatQuota(usedAmount)}
            </div>
          </div>
        </TooltipContent>
      </Tooltip>
    )
  }

  const percent = getRemainingQuotaPercent(sub)

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div className='min-w-[160px] cursor-help space-y-1'>
            <div className='text-xs tabular-nums'>
              {formatQuota(remainingAmount)} / {formatQuota(totalAmount)}
            </div>
            <Progress value={percent} className='w-full' />
          </div>
        }
      />
      <TooltipContent className='max-w-none'>
        <div className='space-y-1 text-xs'>
          <div>
            {t('Used Quota')}: {formatQuota(usedAmount)}
          </div>
          <div>
            {t('Remaining Quota')}: {formatQuota(remainingAmount)} (
            {percent.toFixed(0)}%)
          </div>
          <div>
            {t('Total Quota')}: {formatQuota(totalAmount)}
          </div>
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

export function useUserSubscriptionsColumns(): ColumnDef<AdminUserSubscription>[] {
  const { t } = useTranslation()
  const remainingTotalQuotaTitle = `${t('Remaining Quota')}/${t('Total Quota')}`

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
          const name = sub.display_name || sub.username || t('Unknown User')
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
        accessorFn: (row) => getRemainingQuotaPercent(row),
        meta: { label: remainingTotalQuotaTitle },
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={remainingTotalQuotaTitle}
          />
        ),
        cell: ({ row }) => <QuotaUsageCell sub={row.original} t={t} />,
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
            <div>
              {t('Start')}: {formatTimestampToDate(row.original.start_time)}
            </div>
            <div>
              {t('End')}: {formatTimestampToDate(row.original.end_time)}
            </div>
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
    [remainingTotalQuotaTitle, t]
  )
}
