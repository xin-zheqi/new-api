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
import { getRouteApi } from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { Download, Loader2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { getCurrencyDisplay } from '@/lib/currency'
import { formatDateTimeStr, formatTimestamp } from '@/lib/format'

import { exportUsageLogsCsv } from '../../../../shared/usage-log-export'
import { getUsageLogExport } from '../api'
import { buildApiParams } from '../lib/utils'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

export function UsageLogsExportButton<TData>(props: {
  table: Table<TData>
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const searchParams = route.useSearch()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const [exporting, setExporting] = useState(false)
  const controller = useRef<AbortController | null>(null)

  useEffect(() => () => controller.current?.abort(), [])

  const handleExport = async () => {
    if (controller.current || props.disabled) return
    const activeController = new AbortController()
    controller.current = activeController
    setExporting(true)
    try {
      // Snapshot applied filters, scope, and currency once for the entire file.
      const params = buildApiParams({
        page: 1,
        pageSize: 100,
        searchParams,
        columnFilters: props.table.getState().columnFilters,
        isAdmin,
      })
      const now = new Date()
      params.end_timestamp = Math.min(
        params.end_timestamp ?? Infinity,
        Math.floor(now.getTime() / 1000)
      )
      const { config, meta } = getCurrencyDisplay()
      let unit = t('Tokens')
      let scale = 1
      if (meta.kind !== 'tokens') {
        unit = meta.kind === 'currency' ? meta.currencyCode : meta.symbol
        scale = meta.exchangeRate / config.quotaPerUnit
      }
      const count = await exportUsageLogsCsv({
        headers: [
          t('Username'),
          t('Token Name'),
          t('Model Name'),
          t('Input tokens'),
          t('Output tokens'),
          t('Cache read tokens'),
          t('Cache write tokens'),
          t('Amount ({{unit}})', { unit }),
          t('Time'),
        ],
        fetchExport: () =>
          getUsageLogExport(params, isAdmin, activeController.signal),
        formatAmount: (quota) => quota * scale,
        formatTime: formatTimestamp,
        filename: `usage-logs-${formatDateTimeStr(now).replaceAll(/[: ]/g, '-')}.csv`,
        signal: activeController.signal,
      })
      if (count === 0) toast.info(t('No logs to export'))
    } catch {
      if (!activeController.signal.aborted) {
        toast.error(t('Failed to export logs'))
      }
    } finally {
      controller.current = null
      if (!activeController.signal.aborted) setExporting(false)
    }
  }

  return (
    <Button
      type='button'
      variant='outline'
      disabled={exporting || props.disabled}
      aria-busy={exporting}
      onClick={handleExport}
    >
      {exporting ? <Loader2 className='animate-spin' /> : <Download />}
      {exporting ? t('Exporting...') : t('Export table')}
    </Button>
  )
}
