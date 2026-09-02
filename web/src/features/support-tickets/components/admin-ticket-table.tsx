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
import { Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { useAuthStore } from '@/stores/auth-store'

import { getAdminTickets } from '../api'
import { ADMIN_TICKET_STATUS_OPTIONS, ticketQueryKeys } from '../constants'
import { getTicketErrorMessage } from '../lib/ticket-error'
import type { AdminTicketListParams } from '../types'
import { useAdminTicketColumns } from './admin-ticket-columns'

const route = getRouteApi('/_authenticated/support-tickets/admin')

export function AdminTicketTable(props: {
  onOpenTicket: (id: number) => void
}) {
  const { t } = useTranslation()
  const subjectId = useAuthStore((state) => state.auth.user?.id ?? 0)
  const isMobile = useMediaQuery('(max-width: 640px)')
  const columns = useAdminTicketColumns(props.onOpenTicket)
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'user_id', searchKey: 'userId', type: 'string' },
    ],
  })
  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const userIdFilter =
    (columnFilters.find((filter) => filter.id === 'user_id')?.value as
      | string
      | undefined) ?? ''
  const changeGlobalFilter: NonNullable<typeof onGlobalFilterChange> = (
    updater
  ) => {
    const next =
      typeof updater === 'function' ? updater(globalFilter ?? '') : updater
    const bounded = [...next].slice(0, 100).join('')
    onGlobalFilterChange?.(bounded)
  }
  const adminPagination = {
    ...pagination,
    pageSize: Math.min(pagination.pageSize, 50),
  }
  const changePagination: typeof onPaginationChange = (updater) => {
    const next =
      typeof updater === 'function' ? updater(adminPagination) : updater
    onPaginationChange({ ...next, pageSize: Math.min(next.pageSize, 50) })
  }
  const params: AdminTicketListParams = {
    page: adminPagination.pageIndex + 1,
    page_size: adminPagination.pageSize,
    keyword: globalFilter || undefined,
    status:
      statusFilter[0] === 'waiting_admin' ||
      statusFilter[0] === 'waiting_user' ||
      statusFilter[0] === 'closed'
        ? statusFilter[0]
        : undefined,
    user_id: userIdFilter || undefined,
  }
  const listQuery = useQuery({
    queryKey: ticketQueryKeys.adminList(subjectId, params),
    queryFn: () => getAdminTickets(params),
    enabled: subjectId > 0,
    placeholderData: (previousData) => previousData,
    refetchInterval: 30_000,
  })
  const statusOptions = useMemo(
    () =>
      ADMIN_TICKET_STATUS_OPTIONS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t]
  )
  const { table } = useDataTable({
    data: listQuery.data?.items ?? [],
    columns,
    columnFilters,
    globalFilter,
    pagination: adminPagination,
    onPaginationChange: changePagination,
    onGlobalFilterChange: changeGlobalFilter,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: listQuery.data?.total ?? 0,
    ensurePageInRange,
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={listQuery.isLoading}
      isFetching={listQuery.isFetching}
      emptyTitle={t('No support tickets found')}
      emptyDescription={t('Try adjusting the ticket filters.')}
      skeletonKeyPrefix='support-tickets-admin-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Search ticket ID, subject, user, or email...'),
        searchDebounceMs: 300,
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: statusOptions,
            singleSelect: true,
          },
        ],
        additionalSearch: (
          <Input
            inputMode='numeric'
            aria-label={t('Filter by user ID')}
            placeholder={t('User ID')}
            value={userIdFilter}
            className='w-full sm:w-32'
            onChange={(event) => {
              const value = event.currentTarget.value
                .replaceAll(/\D/g, '')
                .replace(/^0+/, '')
              table.getColumn('user_id')?.setFilterValue(value)
            }}
          />
        ),
        preActions: (
          <Button
            type='button'
            variant='outline'
            size='icon-sm'
            disabled={listQuery.isFetching}
            aria-label={t('Refresh')}
            title={t('Refresh')}
            onClick={() => void listQuery.refetch()}
          >
            <HugeiconsIcon icon={Refresh01Icon} />
          </Button>
        ),
      }}
      afterTable={
        listQuery.isError ? (
          <Alert variant='destructive'>
            <AlertDescription>
              {getTicketErrorMessage(
                listQuery.error,
                t,
                'Failed to load support tickets.'
              )}
            </AlertDescription>
          </Alert>
        ) : undefined
      }
    />
  )
}
