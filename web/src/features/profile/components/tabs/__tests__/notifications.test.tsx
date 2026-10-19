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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { UserProfile } from '../../../types'
import { NotificationTab } from '../notification-tab'

const profile: UserProfile = {
  id: 1,
  username: 'root',
  display_name: 'Root',
  role: 100,
  group: 'default',
  quota: 1000,
  used_quota: 0,
  request_count: 0,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 1,
  setting: '{}',
}

beforeEach(() => {
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

test('root defaults remain enabled and explicit category choices are saved independently', async () => {
  const updated = vi.fn()
  render(<NotificationTab profile={profile} onUpdate={updated} />)
  const quota = screen.getByRole('switch', { name: 'Low balance reminder' })
  const channel = screen.getByRole('switch', { name: 'Channel notifications' })
  expect(quota).toBeChecked()
  expect(channel).toBeChecked()
  fireEvent.click(quota)
  fireEvent.click(channel)
  fireEvent.click(
    screen.getByRole('switch', {
      name: 'Receive Upstream Model Update Notifications',
    })
  )
  fireEvent.click(screen.getByRole('button', { name: 'Save Settings' }))
  await waitFor(() => expect(updated).toHaveBeenCalledOnce())
  expect(api.put).toHaveBeenCalledWith(
    '/api/user/setting',
    expect.objectContaining({
      quota_warning_enabled: false,
      channel_notify_enabled: false,
      upstream_model_update_notify_enabled: true,
      quota_warning_threshold: 0,
    })
  )
})

test('ordinary users retain an explicit opt-out and cannot see administrator notification switches', () => {
  render(
    <NotificationTab
      profile={{
        ...profile,
        role: 1,
        setting: '{"quota_warning_enabled":false,"quota_warning_threshold":0}',
      }}
      onUpdate={vi.fn()}
    />
  )
  expect(
    screen.getByRole('switch', { name: 'Low balance reminder' })
  ).not.toBeChecked()
  expect(
    screen.queryByRole('switch', { name: 'Channel notifications' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('switch', {
      name: 'Receive Upstream Model Update Notifications',
    })
  ).not.toBeInTheDocument()
})

test('site-wide disable leaves channel settings usable without clearing the personal reminder choice', async () => {
  render(
    <NotificationTab
      profile={profile}
      onUpdate={vi.fn()}
      quotaRemindEnabled={false}
    />
  )
  expect(
    screen.getByRole('switch', { name: 'Low balance reminder' })
  ).toHaveAttribute('aria-disabled', 'true')
  fireEvent.click(screen.getByRole('switch', { name: 'Channel notifications' }))
  fireEvent.click(screen.getByRole('button', { name: 'Save Settings' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith(
      '/api/user/setting',
      expect.objectContaining({
        quota_warning_enabled: true,
        channel_notify_enabled: false,
      })
    )
  )
})
