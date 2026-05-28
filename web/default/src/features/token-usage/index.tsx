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
import { Fragment, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type ColumnDef,
  type ExpandedState,
  type PaginationState,
  flexRender,
  getCoreRowModel,
  getExpandedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { VChart } from '@visactor/react-vchart'
import {
  CalendarDays,
  ChevronDown,
  CircleDollarSign,
  Database,
  Search,
  Shapes,
  Sigma,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import dayjs from '@/lib/dayjs'
import {
  formatNumber,
  formatQuota,
  formatTimestampToDate,
  formatTokens,
} from '@/lib/format'
import { cn } from '@/lib/utils'
import { useDebounce } from '@/hooks'
import { SectionPageLayout } from '@/components/layout'
import {
  DataTablePage,
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getTokenModelUsage } from './api'
import type {
  TokenModelUsageItem,
  TokenModelUsageModel,
  TokenModelUsageSummary,
} from './types'

const route = getRouteApi('/_authenticated/token-usage/')

type RangePreset = 'today' | 'yesterday' | '7d' | '30d' | 'month' | 'all'

function getRange(preset: RangePreset) {
  const now = dayjs()
  switch (preset) {
    case 'today':
      return {
        start: now.startOf('day').unix(),
        end: now.endOf('day').unix(),
      }
    case 'yesterday':
      return {
        start: now.subtract(1, 'day').startOf('day').unix(),
        end: now.subtract(1, 'day').endOf('day').unix(),
      }
    case '7d':
      return {
        start: now.subtract(6, 'day').startOf('day').unix(),
        end: now.endOf('day').unix(),
      }
    case '30d':
      return {
        start: now.subtract(29, 'day').startOf('day').unix(),
        end: now.endOf('day').unix(),
      }
    case 'month':
      return {
        start: now.startOf('month').unix(),
        end: now.endOf('month').unix(),
      }
    case 'all':
      return { start: undefined, end: undefined }
  }
}

function toInputValue(timestamp?: number) {
  return timestamp ? dayjs(timestamp * 1000).format('YYYY-MM-DDTHH:mm') : ''
}

function fromInputValue(value: string) {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return Math.floor(date.getTime() / 1000)
}

function getStatusLabel(status: number, t: (key: string) => string) {
  if (status === 1) return t('Enabled')
  if (status === 2) return t('Disabled')
  if (status === 3) return t('Expired')
  if (status === 4) return t('Exhausted')
  return t('Unknown')
}

function getStatusVariant(status: number) {
  return status === 1 ? 'secondary' : 'outline'
}

function formatDisplayKey(key?: string) {
  if (!key) return 'sk-...'
  return key.startsWith('sk-') ? key : `sk-${key}`
}

function SummaryCard({
  title,
  value,
  description,
  icon: Icon,
}: {
  title: string
  value: string
  description: string
  icon: LucideIcon
}) {
  return (
    <Card size='sm'>
      <CardHeader className='pb-1'>
        <div className='flex items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardDescription className='truncate'>{title}</CardDescription>
            <CardTitle className='truncate text-xl tabular-nums'>
              {value}
            </CardTitle>
          </div>
          <Icon className='text-muted-foreground size-4 shrink-0' />
        </div>
      </CardHeader>
      <CardContent>
        <div className='text-muted-foreground truncate text-xs'>
          {description}
        </div>
      </CardContent>
    </Card>
  )
}

function TokenUsagePie({ models }: { models: TokenModelUsageModel[] }) {
  const { t } = useTranslation()
  const data = models
    .filter((model) => model.quota > 0)
    .slice(0, 12)
    .map((model) => ({
      model: model.model_name || t('Unknown Model'),
      quota: model.quota,
      quotaText: formatQuota(model.quota),
    }))

  if (!data.length) {
    return (
      <div className='border-border/60 bg-muted/20 flex aspect-[4/3] min-h-44 items-center justify-center rounded-lg border text-sm text-muted-foreground'>
        {t('No usage data')}
      </div>
    )
  }

  const spec = {
    type: 'pie',
    data: [{ id: 'usage', values: data }],
    outerRadius: 0.78,
    innerRadius: 0.45,
    valueField: 'quota',
    categoryField: 'model',
    legends: {
      visible: true,
      orient: 'bottom',
      item: { visible: true },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum: { model: string }) => datum.model,
            value: (datum: { quotaText: string }) => datum.quotaText,
          },
        ],
      },
    },
  }

  return (
    <div className='h-56 min-h-56'>
      <VChart spec={spec} />
    </div>
  )
}

function ModelUsageTable({ models }: { models: TokenModelUsageModel[] }) {
  const { t } = useTranslation()

  if (!models.length) {
    return (
      <div className='rounded-lg border p-6 text-center text-sm text-muted-foreground'>
        {t('No model usage in selected range')}
      </div>
    )
  }

  return (
    <div className='overflow-hidden rounded-lg border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Model')}</TableHead>
            <TableHead className='text-right'>{t('Usage')}</TableHead>
            <TableHead className='text-right'>{t('Total Tokens')}</TableHead>
            <TableHead className='text-right'>{t('Prompt Tokens')}</TableHead>
            <TableHead className='text-right'>
              {t('Completion Tokens')}
            </TableHead>
            <TableHead className='text-right'>{t('Requests')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {models.map((model) => (
            <TableRow key={model.model_name}>
              <TableCell className='font-medium'>
                {model.model_name || t('Unknown Model')}
              </TableCell>
              <TableCell className='text-right'>
                {formatQuota(model.quota)}
              </TableCell>
              <TableCell className='text-right'>
                {formatTokens(model.total_tokens)}
              </TableCell>
              <TableCell className='text-right'>
                {formatTokens(model.prompt_tokens)}
              </TableCell>
              <TableCell className='text-right'>
                {formatTokens(model.completion_tokens)}
              </TableCell>
              <TableCell className='text-right'>
                {formatNumber(model.requests)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function ExpandedUsage({ item }: { item: TokenModelUsageItem }) {
  const { t } = useTranslation()
  return (
    <TableRow className='hover:bg-transparent'>
      <TableCell colSpan={7} className='bg-muted/20 p-4'>
        <div className='grid gap-5 lg:grid-cols-[320px_1fr]'>
          <div className='space-y-3'>
            <div className='text-sm font-medium'>{t('Model Distribution')}</div>
            <TokenUsagePie models={item.models} />
          </div>
          <div className='space-y-3'>
            <div className='text-sm font-medium'>{t('Model Details')}</div>
            <ModelUsageTable models={item.models} />
          </div>
        </div>
      </TableCell>
    </TableRow>
  )
}

function TokenUsageMobileList({
  table,
  isLoading,
}: {
  table: ReturnType<typeof useReactTable<TokenModelUsageItem>>
  isLoading: boolean
}) {
  const { t } = useTranslation()
  if (isLoading) {
    return (
      <div className='divide-border overflow-hidden rounded-lg border'>
        {Array.from({ length: 5 }).map((_, index) => (
          <div key={index} className='space-y-3 border-b p-3 last:border-b-0'>
            <Skeleton className='h-4 w-36' />
            <Skeleton className='h-3 w-full' />
            <Skeleton className='h-3 w-2/3' />
          </div>
        ))}
      </div>
    )
  }

  const rows = table.getRowModel().rows
  if (!rows.length) {
    return (
      <div className='rounded-lg border p-8 text-center text-sm text-muted-foreground'>
        {t('No API key usage found')}
      </div>
    )
  }

  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {rows.map((row) => {
        const item = row.original
        const expanded = row.getIsExpanded()
        return (
          <div
            key={row.id}
            className={cn(
              'bg-card space-y-3 border-b p-3 last:border-b-0',
              item.status !== 1 && DISABLED_ROW_MOBILE
            )}
          >
            <button
              type='button'
              className='flex w-full items-start justify-between gap-3 text-left'
              onClick={() => row.toggleExpanded()}
            >
              <div className='min-w-0'>
                <div className='truncate text-sm font-semibold'>
                  {item.token_name || t('Token Name')}
                </div>
                <div className='text-muted-foreground truncate font-mono text-xs'>
                  {formatDisplayKey(item.key)}
                </div>
              </div>
              <ChevronDown
                className={cn(
                  'text-muted-foreground mt-1 size-4 shrink-0 transition-transform',
                  expanded && 'rotate-180'
                )}
              />
            </button>
            <div className='grid grid-cols-2 gap-2 text-xs'>
              <div>
                <div className='text-muted-foreground'>{t('Usage')}</div>
                <div className='font-medium'>{formatQuota(item.quota)}</div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('Requests')}</div>
                <div className='font-medium'>{formatNumber(item.requests)}</div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('Models')}</div>
                <div className='font-medium'>{item.model_count}</div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('Total Tokens')}</div>
                <div className='font-medium'>
                  {formatTokens(item.total_tokens)}
                </div>
              </div>
            </div>
            {expanded && (
              <div className='space-y-3 pt-1'>
                <TokenUsagePie models={item.models} />
                <ModelUsageTable models={item.models} />
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

function FilterBar({
  keyword,
  onKeywordChange,
  start,
  end,
  onRangeChange,
}: {
  keyword: string
  onKeywordChange: (value: string) => void
  start?: number
  end?: number
  onRangeChange: (range: { start?: number; end?: number }) => void
}) {
  const { t } = useTranslation()
  const presets: Array<{ value: RangePreset; label: string }> = [
    { value: 'today', label: 'Today' },
    { value: 'yesterday', label: 'Yesterday' },
    { value: '7d', label: '7 Days' },
    { value: '30d', label: '30 Days' },
    { value: 'month', label: 'This month' },
    { value: 'all', label: 'All Time' },
  ]

  return (
    <div className='flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:flex-wrap sm:items-center'>
      <div className='relative min-w-0 flex-1 sm:min-w-60'>
        <Search className='text-muted-foreground pointer-events-none absolute top-2 left-2.5 size-4' />
        <Input
          value={keyword}
          onChange={(event) => onKeywordChange(event.target.value)}
          placeholder={t('Filter by API key name...')}
          className='pl-8'
        />
      </div>
      <div className='grid gap-2 sm:grid-cols-[180px_180px]'>
        <Input
          type='datetime-local'
          value={toInputValue(start)}
          onChange={(event) =>
            onRangeChange({ start: fromInputValue(event.target.value), end })
          }
          aria-label={t('Start Time')}
        />
        <Input
          type='datetime-local'
          value={toInputValue(end)}
          onChange={(event) =>
            onRangeChange({ start, end: fromInputValue(event.target.value) })
          }
          aria-label={t('End Time')}
        />
      </div>
      <div className='flex flex-wrap gap-1.5'>
        {presets.map((preset) => (
          <Button
            key={preset.value}
            type='button'
            variant='secondary'
            size='sm'
            className='h-8 px-2'
            onClick={() => onRangeChange(getRange(preset.value))}
          >
            {t(preset.label)}
          </Button>
        ))}
      </div>
    </div>
  )
}

function getColumns(
  t: (key: string) => string
): ColumnDef<TokenModelUsageItem>[] {
  return [
    {
      id: 'expand',
      header: '',
      cell: ({ row }) => (
        <Button
          type='button'
          variant='ghost'
          size='icon'
          className='size-7'
          onClick={() => row.toggleExpanded()}
          aria-label={row.getIsExpanded() ? t('Collapse') : t('Expand')}
        >
          <ChevronDown
            className={cn(
              'size-4 transition-transform',
              row.getIsExpanded() && 'rotate-180'
            )}
          />
        </Button>
      ),
      size: 42,
    },
    {
      accessorKey: 'key',
      header: t('API Key'),
      cell: ({ row }) => (
        <div className='min-w-0'>
          <div className='max-w-56 truncate font-medium'>
            {row.original.token_name || t('Token Name')}
          </div>
          <div className='text-muted-foreground max-w-56 truncate font-mono text-xs'>
            {formatDisplayKey(row.original.key)}
          </div>
        </div>
      ),
    },
    {
      accessorKey: 'status',
      header: t('Status'),
      cell: ({ row }) => (
        <Badge variant={getStatusVariant(row.original.status)}>
          {getStatusLabel(row.original.status, t)}
        </Badge>
      ),
    },
    {
      accessorKey: 'quota',
      header: () => <div className='text-right'>{t('Usage')}</div>,
      cell: ({ row }) => (
        <div className='text-right font-medium'>
          {formatQuota(row.original.quota)}
        </div>
      ),
    },
    {
      accessorKey: 'model_count',
      header: () => <div className='text-right'>{t('Models')}</div>,
      cell: ({ row }) => (
        <div className='text-right'>{row.original.model_count}</div>
      ),
    },
    {
      accessorKey: 'total_tokens',
      header: () => <div className='text-right'>{t('Total Tokens')}</div>,
      cell: ({ row }) => (
        <div className='text-right'>
          {formatTokens(row.original.total_tokens)}
        </div>
      ),
    },
    {
      accessorKey: 'requests',
      header: () => <div className='text-right'>{t('Requests')}</div>,
      cell: ({ row }) => (
        <div className='text-right'>{formatNumber(row.original.requests)}</div>
      ),
    },
  ]
}

function defaultSummary(): TokenModelUsageSummary {
  return {
    total_key_count: 0,
    active_key_count: 0,
    model_count: 0,
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    total_tokens: 0,
    requests: 0,
  }
}

export function TokenUsage() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const [keywordInput, setKeywordInput] = useState(search.keyword || '')
  const debouncedKeyword = useDebounce(keywordInput, 500)
  const [expanded, setExpanded] = useState<ExpandedState>({})

  const pagination = useMemo<PaginationState>(
    () => ({
      pageIndex: Math.max((search.page || 1) - 1, 0),
      pageSize: search.pageSize || 20,
    }),
    [search.page, search.pageSize]
  )

  useEffect(() => {
    setKeywordInput(search.keyword || '')
  }, [search.keyword])

  useEffect(() => {
    if (debouncedKeyword !== (search.keyword || '')) {
      void navigate({
        search: (prev) => ({
          ...prev,
          page: 1,
          keyword: debouncedKeyword || undefined,
        }),
        replace: true,
      })
    }
  }, [debouncedKeyword, navigate, search.keyword])

  const startTimestamp = search.startTime
  const endTimestamp = search.endTime

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'token-model-usage',
      pagination.pageIndex,
      pagination.pageSize,
      search.keyword,
      startTimestamp,
      endTimestamp,
      t,
    ],
    queryFn: async () => {
      const result = await getTokenModelUsage({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: search.keyword,
        start_timestamp: startTimestamp,
        end_timestamp: endTimestamp,
      })
      if (!result.success) {
        toast.error(result.message || t('Failed to load token usage'))
        return { items: [], total: 0, summary: defaultSummary() }
      }
      return {
        items: result.data?.page.items || [],
        total: result.data?.page.total || 0,
        summary: result.data?.summary || defaultSummary(),
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const columns = useMemo(() => getColumns(t), [t])
  const table = useReactTable({
    data: data?.items || [],
    columns,
    state: { pagination, expanded },
    manualPagination: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
    onPaginationChange: (updater) => {
      const next =
        typeof updater === 'function' ? updater(pagination) : updater
      void navigate({
        search: (prev) => ({
          ...prev,
          page: next.pageIndex + 1,
          pageSize: next.pageSize,
        }),
      })
    },
    onExpandedChange: setExpanded,
    getCoreRowModel: getCoreRowModel(),
    getExpandedRowModel: getExpandedRowModel(),
    getRowCanExpand: () => true,
  })

  const summary = data?.summary || defaultSummary()
  const rangeLabel =
    startTimestamp || endTimestamp
      ? `${startTimestamp ? formatTimestampToDate(startTimestamp) : '-'} ~ ${
          endTimestamp ? formatTimestampToDate(endTimestamp) : '-'
        }`
      : t('All Time')

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('API Key Usage')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
            <SummaryCard
              title={t('Total Usage')}
              value={formatQuota(summary.quota)}
              description={rangeLabel}
              icon={CircleDollarSign}
            />
            <SummaryCard
              title={t('Requests')}
              value={formatNumber(summary.requests)}
              description={t('{{count}} active keys', {
                count: summary.active_key_count,
              })}
              icon={Sigma}
            />
            <SummaryCard
              title={t('Total Tokens')}
              value={formatTokens(summary.total_tokens)}
              description={`${t('Prompt Tokens')}: ${formatTokens(summary.prompt_tokens)}`}
              icon={Database}
            />
            <SummaryCard
              title={t('Models')}
              value={formatNumber(summary.model_count)}
              description={`${summary.total_key_count} ${t('API Keys')}`}
              icon={Shapes}
            />
          </div>

          <FilterBar
            keyword={keywordInput}
            onKeywordChange={setKeywordInput}
            start={startTimestamp}
            end={endTimestamp}
            onRangeChange={(range) =>
              void navigate({
                search: (prev) => ({
                  ...prev,
                  page: 1,
                  startTime: range.start,
                  endTime: range.end,
                }),
              })
            }
          />

          <div className='text-muted-foreground flex items-center gap-2 text-xs'>
            <CalendarDays className='size-3.5' />
            <span>{rangeLabel}</span>
          </div>

          <DataTablePage
            table={table}
            columns={columns}
            isLoading={isLoading}
            isFetching={isFetching}
            emptyTitle={t('No API key usage found')}
            emptyDescription={t('Adjust the time range or key filter.')}
            toolbarProps={null}
            skeletonKeyPrefix='token-usage-skeleton'
            mobile={
              <TokenUsageMobileList table={table} isLoading={isLoading} />
            }
            getRowClassName={(row) =>
              row.original.status !== 1 ? DISABLED_ROW_DESKTOP : undefined
            }
            renderRow={(row) => (
              <Fragment key={row.id}>
                <TableRow
                  data-state={row.getIsSelected() && 'selected'}
                  className={
                    row.original.status !== 1
                      ? DISABLED_ROW_DESKTOP
                      : undefined
                  }
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext()
                      )}
                    </TableCell>
                  ))}
                </TableRow>
                {row.getIsExpanded() && (
                  <ExpandedUsage item={row.original} />
                )}
              </Fragment>
            )}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
