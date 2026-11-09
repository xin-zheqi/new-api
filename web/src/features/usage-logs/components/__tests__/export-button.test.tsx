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
  getCoreRowModel,
  useReactTable,
  type ColumnFiltersState,
} from '@tanstack/react-table'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { AxiosResponse } from 'axios'
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
  vi,
} from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
  type CurrencyDisplayType,
} from '@/stores/system-config-store'

import type { UsageLogExportRecord } from '../../../../../shared/usage-log-export'
import { LogsFilterToolbar } from '../logs-filter-toolbar'
import { UsageLogsExportButton } from '../usage-logs-export-button'
import { UsageLogsProvider, useLogsViewScope } from '../usage-logs-provider'

const routeState = vi.hoisted(() => ({ search: {} as Record<string, unknown> }))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  getRouteApi: () => ({ useSearch: () => routeState.search }),
}))

const now = new Date('2026-11-09T04:00:00.000Z')
const columns = [{ accessorKey: 'model_name', header: 'Model' }]
const noRows: UsageLogExportRecord[] = []
const record: UsageLogExportRecord = {
  username: 'alice',
  token_name: 'team&key',
  model_name: 'public+model',
  prompt_tokens: 1234,
  completion_tokens: 56,
  quota: 5000,
  created_at: 1794180000,
  other: '{"cache_tokens":800,"cache_creation_tokens":20}',
}

function pageResponse(
  items: UsageLogExportRecord[],
  total = items.length,
  page = 1
): AxiosResponse {
  return {
    data: { success: true, data: { items, page, page_size: 100, total } },
  } as AxiosResponse
}

function ExportControls(props: {
  filters?: ColumnFiltersState
  disabled?: boolean
  toolbar?: boolean
}) {
  const table = useReactTable({
    columns,
    data: noRows,
    getCoreRowModel: getCoreRowModel(),
    initialState: {
      columnFilters: props.filters || [],
      pagination: { pageIndex: 4, pageSize: 20 },
    },
  })
  const { canManageScope, setViewScope } = useLogsViewScope()
  const exportButton = (
    <UsageLogsExportButton table={table} disabled={props.disabled} />
  )
  return (
    <>
      {canManageScope && (
        <button type='button' onClick={() => setViewScope('self')}>
          Only mine
        </button>
      )}
      <button
        type='button'
        onClick={() =>
          table.setColumnFilters([{ id: 'model_name', value: 'later-model' }])
        }
      >
        Change model filter
      </button>
      {props.toolbar ? (
        <LogsFilterToolbar
          table={table}
          primaryFilters={<span>Applied filters</span>}
          mobilePinnedFilters={<span>Date range</span>}
          hasActiveFilters={false}
          onSearch={() => undefined}
          onReset={() => undefined}
          actionEnd={exportButton}
        />
      ) : (
        exportButton
      )}
    </>
  )
}

function renderExport(props: Parameters<typeof ExportControls>[0] = {}) {
  return render(
    <UsageLogsProvider>
      <ExportControls {...props} />
    </UsageLogsProvider>
  )
}

function readBlob(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.addEventListener('error', () => reject(reader.error), { once: true })
    reader.addEventListener('load', () => resolve(reader.result as string), {
      once: true,
    })
    reader.readAsText(blob)
  })
}

describe('usage logs export button', () => {
  const createObjectURL = vi.fn((_blob: Blob) => 'blob:usage-export')
  const revokeObjectURL = vi.fn()

  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Amount ({{unit}})': 'Amount ({{unit}})',
      'Export table': 'Export table',
      'Exporting...': 'Exporting...',
      'No logs to export': 'No logs to export',
      'Failed to export logs': 'Failed to export logs',
    })
  })

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] })
    vi.setSystemTime(now)
    routeState.search = {
      startTime: now.getTime() - 3600_000,
      endTime: now.getTime() + 3600_000,
      type: ['2'],
      model: 'search-model',
      token: 'search-token',
      group: 'search-group',
      username: 'search-user',
      channel: '8',
      requestId: 'req&1+two',
      upstreamRequestId: 'upstream&id+two',
      page: 5,
    }
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'admin', role: ROLE.ADMIN })
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
      },
    })
    vi.stubGlobal(
      'URL',
      class extends URL {
        static createObjectURL = createObjectURL
        static revokeObjectURL = revokeObjectURL
      }
    )
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(
      () => undefined
    )
    vi.spyOn(toast, 'info').mockImplementation(() => 'info')
    vi.spyOn(toast, 'error').mockImplementation(() => 'error')
  })

  afterEach(() => {
    cleanup()
    if (vi.isFakeTimers()) vi.runOnlyPendingTimers()
    vi.useRealTimers()
    vi.unstubAllGlobals()
    useAuthStore.getState().auth.reset()
    useSystemConfigStore
      .getState()
      .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  })

  test('exports applied admin filters from page one and uses numeric CNY charges', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue(pageResponse([record]))
    renderExport({
      filters: [
        { id: 'model_name', value: 'public&model+v1' },
        { id: 'token_name', value: 'team&key' },
        { id: 'group', value: 'group+1' },
        { id: 'username', value: 'alice&bob' },
        { id: 'channel', value: '12' },
      ],
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Export table' }))
    })

    expect(get).toHaveBeenCalledOnce()
    const url = new URL(get.mock.calls[0][0], 'https://newapi.test')
    expect(url.pathname).toBe('/api/log')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      p: '1',
      page_size: '100',
      type: '2',
      model_name: 'public&model+v1',
      token_name: 'team&key',
      group: 'group+1',
      username: 'alice&bob',
      channel: '12',
      request_id: 'req&1+two',
      upstream_request_id: 'upstream&id+two',
      start_timestamp: String(now.getTime() / 1000 - 3600),
      end_timestamp: String(now.getTime() / 1000),
    })
    expect(createObjectURL).toHaveBeenCalledOnce()
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    const csv = await readBlob(createObjectURL.mock.calls[0][0])
    expect(csv.split('\r\n')[0]).toBe(
      '"Username","Token Name","Model Name","Input tokens","Output tokens","Cache read tokens","Cache write tokens","Amount (CNY)","Time"'
    )
    expect(csv.split('\r\n')[1]).toMatch(
      /^"alice","team&key","public\+model",1234,56,800,20,0.07,"\d{4}-\d\d-\d\d \d\d:\d\d:\d\d"$/
    )
    expect(screen.getByRole('button', { name: 'Export table' })).toBeEnabled()
    expect(toast.error).not.toHaveBeenCalled()
  })

  test.each([
    ['regular user', ROLE.USER, false],
    ['admin viewing only mine', ROLE.ADMIN, true],
  ])(
    'keeps %s on the self endpoint without admin-only filters',
    async (_, role, switchScope) => {
      useAuthStore.getState().auth.setUser({ id: 2, username: 'self', role })
      const get = vi.spyOn(api, 'get').mockResolvedValue(pageResponse([record]))
      renderExport()
      if (switchScope) {
        fireEvent.click(screen.getByRole('button', { name: 'Only mine' }))
      }
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: 'Export table' }))
      })
      const url = new URL(get.mock.calls[0][0], 'https://newapi.test')
      expect(url.pathname).toBe('/api/log/self')
      expect(url.searchParams.has('username')).toBe(false)
      expect(url.searchParams.has('channel')).toBe(false)
      expect(url.searchParams.get('model_name')).toBe('search-model')
      expect(url.searchParams.get('request_id')).toBe('req&1+two')
      expect(createObjectURL).toHaveBeenCalledOnce()
    }
  )

  test('fixes filters, scope, time and currency at click while preventing duplicate exports', async () => {
    let resolveFirst!: (response: AxiosResponse) => void
    const firstPage = new Promise<AxiosResponse>((resolve) => {
      resolveFirst = resolve
    })
    const get = vi
      .spyOn(api, 'get')
      .mockReturnValueOnce(firstPage)
      .mockResolvedValueOnce(pageResponse([record], 101, 2))
    renderExport()
    const exportButton = screen.getByRole('button', { name: 'Export table' })
    act(() => {
      fireEvent.click(exportButton)
      fireEvent.click(exportButton)
    })
    expect(get).toHaveBeenCalledOnce()
    expect(screen.getByRole('button', { name: 'Exporting...' })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Exporting...' })
    ).toHaveAttribute('aria-busy', 'true')

    routeState.search = {
      model: 'later-model',
      endTime: now.getTime() + 7200_000,
    }
    act(() => {
      useSystemConfigStore.getState().setConfig({
        currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
      })
      fireEvent.click(screen.getByRole('button', { name: 'Only mine' }))
      fireEvent.click(
        screen.getByRole('button', { name: 'Change model filter' })
      )
    })
    vi.setSystemTime(new Date(now.getTime() + 60_000))
    await act(async () => {
      resolveFirst(
        pageResponse(
          Array.from({ length: 100 }, () => record),
          101
        )
      )
    })
    expect(get).toHaveBeenCalledTimes(2)
    const first = new URL(get.mock.calls[0][0], 'https://newapi.test')
    const second = new URL(get.mock.calls[1][0], 'https://newapi.test')
    expect(second.pathname).toBe('/api/log')
    expect(second.searchParams.get('p')).toBe('2')
    second.searchParams.set('p', '1')
    expect(second.toString()).toBe(first.toString())
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    const csv = await readBlob(createObjectURL.mock.calls[0][0])
    expect(csv).toContain('"Amount (CNY)"')
    expect(csv.split('\r\n')[101]).toContain(',1234,56,800,20,0.07,')
    expect(screen.getByRole('button', { name: 'Export table' })).toBeEnabled()
  })

  test.each([
    ['USD', 'USD', 0.01],
    ['CUSTOM', '¤', 0.03],
    ['TOKENS', 'Tokens', 5000],
  ] as const)(
    'uses %s display units without applying an unrelated CNY rate',
    async (displayType, unit, amount) => {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: displayType as CurrencyDisplayType,
          usdExchangeRate: 7,
          customCurrencyExchangeRate: 3,
        },
      })
      vi.spyOn(api, 'get').mockResolvedValue(pageResponse([record]))
      renderExport()
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: 'Export table' }))
      })
      vi.runOnlyPendingTimers()
      vi.useRealTimers()
      const csv = await readBlob(createObjectURL.mock.calls[0][0])
      expect(csv).toContain(`"Amount (${unit})"`)
      expect(csv.split('\r\n')[1]).toContain(`,1234,56,800,20,${amount},`)
    }
  )

  test.each(['business error', 'connection error'])(
    'restores the button and reports %s without downloading',
    async (failure) => {
      const get = vi.spyOn(api, 'get')
      if (failure === 'business error') {
        get.mockResolvedValue({
          data: { success: false, message: 'Query failed' },
        } as AxiosResponse)
      } else {
        get.mockRejectedValue(new Error('Connection closed'))
      }
      renderExport()
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: 'Export table' }))
      })
      expect(toast.error).toHaveBeenCalledWith('Failed to export logs')
      expect(createObjectURL).not.toHaveBeenCalled()
      expect(screen.getByRole('button', { name: 'Export table' })).toBeEnabled()
    }
  )

  test('reports no matching rows and leaves the button available', async () => {
    vi.spyOn(api, 'get').mockResolvedValue(pageResponse([]))
    renderExport()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Export table' }))
    })
    expect(toast.info).toHaveBeenCalledWith('No logs to export')
    expect(createObjectURL).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Export table' })).toBeEnabled()
  })

  test('does not export while the surrounding log query has disabled the button', () => {
    const get = vi.spyOn(api, 'get')
    renderExport({ disabled: true })
    fireEvent.click(screen.getByRole('button', { name: 'Export table' }))
    expect(screen.getByRole('button', { name: 'Export table' })).toBeDisabled()
    expect(get).not.toHaveBeenCalled()
  })

  test('cancels the request on unmount and never downloads its late response', async () => {
    let resolvePage!: (response: AxiosResponse) => void
    const get = vi.spyOn(api, 'get').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolvePage = resolve
        })
    )
    const rendered = renderExport()
    fireEvent.click(screen.getByRole('button', { name: 'Export table' }))
    const signal = get.mock.calls[0][1]?.signal as AbortSignal
    expect(signal.aborted).toBe(false)
    rendered.unmount()
    expect(signal.aborted).toBe(true)
    await act(async () => {
      resolvePage(pageResponse([record]))
    })
    expect(createObjectURL).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  })

  test.each([false, true])(
    'places export after column settings at mobile=%s',
    (mobile) => {
      const originalMatchMedia = window.matchMedia
      vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
        ...originalMatchMedia(query),
        matches: mobile && query === '(max-width: 640px)',
      }))
      renderExport({ toolbar: true })
      const view = screen.getByRole('button', { name: 'View' })
      const exportButton = screen.getByRole('button', { name: 'Export table' })
      expect(view).toBeVisible()
      expect(exportButton).toBeVisible()
      expect(
        view.compareDocumentPosition(exportButton) &
          Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
    }
  )
})
