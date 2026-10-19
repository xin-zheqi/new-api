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
import React from 'react';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { API } from '../../../../helpers/api';
import { StatusContext } from '../../../../context/Status';
import SettingsMonitoring from '../SettingsMonitoring';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
});
test('classic site reminder switch saves false and refreshes the personal-settings status', async () => {
  const dispatch = vi.fn();
  const refresh = vi.fn();
  vi.spyOn(API, 'put').mockResolvedValue({ data: { success: true } });
  vi.spyOn(API, 'get').mockResolvedValue({
    data: { success: true, data: { quota_remind_enabled: false } },
  });
  render(
    <StatusContext.Provider value={[{ status: {} }, dispatch]}>
      <SettingsMonitoring
        refresh={refresh}
        options={{
          QuotaRemindEnabled: true,
          QuotaRemindThreshold: '100',
          AutomaticDisableStatusCodes: '401',
          AutomaticRetryStatusCodes: '500',
          'monitor_setting.auto_test_channel_time_range': '00:00-23:59',
        }}
      />
    </StatusContext.Provider>,
  );
  fireEvent.click(
    await screen.findByRole('switch', {
      name: 'Site-wide low balance reminders',
    }),
  );
  fireEvent.click(screen.getByRole('button'));
  await waitFor(() =>
    expect(API.put).toHaveBeenCalledWith('/api/option/', {
      key: 'QuotaRemindEnabled',
      value: 'false',
    }),
  );
  await waitFor(() =>
    expect(dispatch).toHaveBeenCalledWith({
      type: 'set',
      payload: { quota_remind_enabled: false },
    }),
  );
  expect(refresh).toHaveBeenCalled();
});
