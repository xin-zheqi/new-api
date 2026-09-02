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
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type SortingState,
  type VisibilityState,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
} from '@/components/data-table'
import { getAdminPlans } from '@/features/subscriptions/api'
import { getAdminUserSubscriptions } from '../api'
import {
  getSubscriptionSourceOptions,
  getSubscriptionStatusOptions,
} from '../constants'
import type { AdminUserSubscription } from '../types'
import { useUserSubscriptionsColumns } from './user-subscriptions-columns'
import { useUserSubscriptions } from './user-subscriptions-provider'

const route = getRouteApi('/_authenticated/subscriptions/users')

function isReadonlySubscriptionRow(subscription: AdminUserSubscription) {
  return (
    subscription.effective_status === 'expired' ||
    subscription.effective_status === 'cancelled'
  )
}

export function UserSubscriptionsTable() {
  const { t } = useTranslation()
  const columns = useUserSubscriptionsColumns()
  const { refreshTrigger } = useUserSubscriptions()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

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
      { columnId: 'source', searchKey: 'source', type: 'array' },
      { columnId: 'plan_id', searchKey: 'plan_id', type: 'array' },
    ],
  })

  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const sourceFilter =
    (columnFilters.find((filter) => filter.id === 'source')?.value as
      | string[]
      | undefined) ?? []
  const planFilter =
    (columnFilters.find((filter) => filter.id === 'plan_id')?.value as
      | string[]
      | undefined) ?? []

  const { data: plansData } = useQuery({
    queryKey: ['admin-subscription-plans-for-user-subscriptions'],
    queryFn: async () => {
      const result = await getAdminPlans()
      return result.data || []
    },
    placeholderData: (prev) => prev,
  })

  const planOptions = useMemo(
    () =>
      (plansData || []).map((item) => ({
        value: String(item.plan.id),
        label: item.plan.title || `#${item.plan.id}`,
      })),
    [plansData]
  )

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'admin-user-subscriptions',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilter,
      sourceFilter,
      planFilter,
      refreshTrigger,
    ],
    queryFn: async () => {
      const result = await getAdminUserSubscriptions({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: globalFilter,
        status: statusFilter[0] ?? '',
        source: sourceFilter[0] ?? '',
        plan_id: Number(planFilter[0] || 0),
      })
      if (!result.success) {
        toast.error(result.message || t('Failed to load'))
        return { items: [], total: 0 }
      }
      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const subscriptions = data?.items || []

  const table = useReactTable({
    data: subscriptions,
    columns,
    state: {
      sorting,
      columnVisibility,
      columnFilters,
      globalFilter,
      pagination,
    },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    globalFilterFn: (row, _columnId, filterValue) => {
      const searchValue = String(filterValue).toLowerCase()
      const fields = [
        row.original.id,
        row.original.user_id,
        row.original.username,
        row.original.display_name,
        row.original.email,
        row.original.plan_id,
        row.original.plan_title,
      ]
      return fields.some((field) =>
        String(field || '')
          .toLowerCase()
          .includes(searchValue)
      )
    },
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    manualPagination: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No user subscriptions found')}
      emptyDescription={t(
        'Try adjusting filters or open a subscription for a user.'
      )}
      skeletonKeyPrefix='user-subscriptions-skeleton'
      toolbarProps={{
        searchPlaceholder: t('Filter by user, email, plan or ID...'),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: getSubscriptionStatusOptions(t),
            singleSelect: true,
          },
          {
            columnId: 'source',
            title: t('Source'),
            options: getSubscriptionSourceOptions(t),
            singleSelect: true,
          },
          {
            columnId: 'plan_id',
            title: t('Plan'),
            options: planOptions,
            singleSelect: true,
          },
        ],
      }}
      getRowClassName={(row, { isMobile }) =>
        isReadonlySubscriptionRow(row.original)
          ? isMobile
            ? DISABLED_ROW_MOBILE
            : DISABLED_ROW_DESKTOP
          : undefined
      }
    />
  )
}
