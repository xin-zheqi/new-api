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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { MonitoringSettingsSection } from '../monitoring-settings-section'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})
test('turning off site reminders saves only that option and invalidates cached status', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  client.setQueryData(['status'], { quota_remind_enabled: true })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  const actions = document.createElement('div')
  document.body.append(actions)
  try {
    render(
      <QueryClientProvider client={client}>
        <SettingsPageProvider actionsContainer={actions}>
          <MonitoringSettingsSection
            defaultValues={{
              QuotaRemindEnabled: true,
              QuotaRemindThreshold: '100',
              'perf_metrics_setting.enabled': false,
              'perf_metrics_setting.flush_interval': 10,
              'perf_metrics_setting.bucket_time': 'minute',
              'perf_metrics_setting.retention_days': 7,
            }}
          />
        </SettingsPageProvider>
      </QueryClientProvider>
    )
    fireEvent.click(
      screen.getByRole('switch', { name: 'Site-wide low balance reminders' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(api.put).toHaveBeenCalledWith(
        '/api/option/',
        { key: 'QuotaRemindEnabled', value: false },
        expect.any(Object)
      )
    )
    expect(api.put).toHaveBeenCalledTimes(1)
    expect(client.getQueryState(['status'])?.isInvalidated).toBe(true)
  } finally {
    actions.remove()
    client.clear()
  }
})
