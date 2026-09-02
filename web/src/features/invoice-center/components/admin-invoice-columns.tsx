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

import { formatInvoiceMoney, formatInvoiceTime } from '../lib/invoice-form'
import type { InvoiceApplication } from '../types'
import { InvoiceStatusBadge } from './invoice-status-badge'

export function useAdminInvoiceColumns(
  onOpenApplication: (id: number) => void
): ColumnDef<InvoiceApplication>[] {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage

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
        size: 76,
        meta: { mobileOrder: 10 },
      },
      {
        accessorKey: 'user_id',
        header: t('User'),
        cell: ({ row }) => {
          const user = row.original.user
          const displayName =
            user?.display_name ||
            user?.username ||
            t('User #{{id}}', { id: row.original.user_id })
          return (
            <div className='flex min-w-40 flex-col gap-0.5'>
              <span className='[overflow-wrap:anywhere] break-words'>
                {displayName}
              </span>
              <span className='text-muted-foreground text-xs'>
                #{row.original.user_id}
                {user?.email ? ` | ${user.email}` : ''}
              </span>
            </div>
          )
        },
        enableSorting: false,
        size: 220,
        meta: { mobileOrder: 20 },
      },
      {
        accessorKey: 'invoice_title',
        header: t('Invoice title'),
        cell: ({ row }) => (
          <button
            type='button'
            className='focus-visible:ring-ring max-w-72 cursor-pointer text-start font-medium [overflow-wrap:anywhere] break-words hover:underline focus-visible:ring-2 focus-visible:outline-none'
            onClick={() => onOpenApplication(row.original.id)}
          >
            {row.original.invoice_title}
          </button>
        ),
        enableSorting: false,
        size: 280,
        meta: { mobileTitle: true },
      },
      {
        accessorKey: 'taxpayer_id',
        header: t('Taxpayer ID'),
        cell: ({ row }) => (
          <span className='max-w-48 [overflow-wrap:anywhere] break-words'>
            {row.original.taxpayer_id || '-'}
          </span>
        ),
        enableSorting: false,
        size: 180,
        meta: { mobileOrder: 30 },
      },
      {
        accessorKey: 'total_amount_micros',
        header: t('Amount'),
        cell: ({ row }) => (
          <span className='whitespace-nowrap tabular-nums'>
            {formatInvoiceMoney(
              row.original.total_amount_micros,
              row.original.currency,
              locale
            )}
          </span>
        ),
        enableSorting: false,
        size: 120,
        meta: { mobileOrder: 40 },
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        cell: ({ row }) => <InvoiceStatusBadge status={row.original.status} />,
        filterFn: (row, id, value) => value.includes(row.getValue(id)),
        enableSorting: false,
        size: 110,
        meta: { mobileBadge: true },
      },
      {
        accessorKey: 'updated_at',
        header: t('Last activity'),
        cell: ({ row }) => (
          <time className='text-muted-foreground whitespace-nowrap'>
            {formatInvoiceTime(
              row.original.updated_at || row.original.created_at
            )}
          </time>
        ),
        enableSorting: false,
        size: 180,
        meta: { mobileOrder: 50 },
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={t('View invoice application')}
            title={t('View invoice application')}
            onClick={() => onOpenApplication(row.original.id)}
          >
            <HugeiconsIcon icon={ViewIcon} />
          </Button>
        ),
        enableHiding: false,
        enableSorting: false,
        meta: { pinned: 'right' as const },
      },
    ],
    [locale, onOpenApplication, t]
  )
}
