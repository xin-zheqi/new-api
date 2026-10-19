/*
Copyright (C) 2025 QuantumNous

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
import React, { useState } from 'react';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, beforeEach, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { API } from '../../../../../helpers/api';
import { StatusContext } from '../../../../../context/Status';
import { UserContext } from '../../../../../context/User';
import NotificationSettings from '../NotificationSettings';

function NotificationForm({ role = 100, siteEnabled = true, onSave }) {
  const [settings, setSettings] = useState({
    warningType: 'email',
    warningThreshold: 0,
    quotaWarningEnabled: true,
    channelNotifyEnabled: role === 100,
    upstreamModelUpdateNotifyEnabled: false,
  });
  return (
    <StatusContext.Provider
      value={[{ status: { quota_remind_enabled: siteEnabled } }, vi.fn()]}
    >
      <UserContext.Provider
        value={[{ user: { role, setting: '{}' } }, vi.fn()]}
      >
        <NotificationSettings
          t={i18next.t.bind(i18next)}
          notificationSettings={settings}
          handleNotificationSettingChange={(key, value) =>
            setSettings((current) => ({ ...current, [key]: value }))
          }
          saveNotificationSettings={() => onSave(settings)}
        />
      </UserContext.Provider>
    </StatusContext.Provider>
  );
}

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
beforeEach(() => {
  localStorage.clear();
  vi.spyOn(API, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { setting: '{}', permissions: { sidebar_settings: false } },
    },
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
});

test('classic category switches submit explicit opt-outs and accept the site-default threshold', async () => {
  const save = vi.fn();
  render(<NotificationForm onSave={save} />);
  fireEvent.click(
    await screen.findByRole('switch', { name: 'Low balance reminder' }),
  );
  fireEvent.click(
    screen.getByRole('switch', { name: 'Channel notifications' }),
  );
  fireEvent.click(screen.getByRole('button', { name: '保存设置' }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        quotaWarningEnabled: false,
        channelNotifyEnabled: false,
        warningThreshold: 0,
      }),
    ),
  );
});

test('classic ordinary users cannot select channel notices and site disable keeps their reminder preference', async () => {
  render(<NotificationForm role={1} siteEnabled={false} onSave={vi.fn()} />);
  const quota = await screen.findByRole('switch', {
    name: 'Low balance reminder',
  });
  expect(quota).toBeDisabled();
  expect(quota).toBeChecked();
  expect(
    screen.queryByRole('switch', { name: 'Channel notifications' }),
  ).not.toBeInTheDocument();
});
