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
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { Toast } from '@douyinfe/semi-ui';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { StatusContext } from '../../../../context/Status';
import { API } from '../../../../helpers/api';
import SettingsMonitoring from '../SettingsMonitoring';

beforeAll(() =>
  i18next.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  }),
);
afterEach(async () => {
  cleanup();
  await act(async () => Toast.destroyAll());
  vi.restoreAllMocks();
});

function TestContext(props) {
  return (
    <StatusContext.Provider value={[{ status: {} }, vi.fn()]}>
      {props.children}
    </StatusContext.Provider>
  );
}

test('the old monitoring form saves fractional intervals and concurrency without rewriting the current mode', async () => {
  const refresh = vi.fn();
  vi.spyOn(API, 'patch').mockResolvedValue({ data: { success: true } });
  vi.spyOn(API, 'put');
  render(
    <TestContext>
      <SettingsMonitoring
        refresh={refresh}
        options={{
          'monitor_setting.channel_test_mode': 'auto_ban_only',
          'monitor_setting.auto_test_channel_minutes': 10,
          'monitor_setting.channel_test_concurrency': 1,
        }}
      />
    </TestContext>,
  );
  expect(
    screen.getByText('Test automatically disabled channels'),
  ).toBeVisible();
  fireEvent.change(screen.getByLabelText('自动测试所有通道间隔时间'), {
    target: { value: '0.5' },
  });
  fireEvent.change(screen.getByLabelText('Channel test concurrency'), {
    target: { value: '3' },
  });
  fireEvent.click(screen.getByRole('button', { name: '保存监控设置' }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  expect(API.patch).toHaveBeenCalledExactlyOnceWith(
    '/api/option/request_policy',
    {
      options: {
        'monitor_setting.auto_test_channel_minutes': '0.5',
        'monitor_setting.channel_test_concurrency': '3',
      },
    },
  );
  expect(API.put).not.toHaveBeenCalled();
});

test('selecting passive recovery in the old form atomically saves its legacy alias', async () => {
  const refresh = vi.fn();
  vi.spyOn(API, 'patch').mockResolvedValue({ data: { success: true } });
  render(
    <TestContext>
      <SettingsMonitoring
        refresh={refresh}
        options={{
          'monitor_setting.channel_test_mode': 'scheduled_all',
        }}
      />
    </TestContext>,
  );
  fireEvent.click(screen.getByRole('combobox', { name: 'Channel test mode' }));
  const option = await screen.findByRole('option', {
    name: /Passive recovery/,
  });
  const popup = option.closest('[role="dialog"]');
  fireEvent.click(option);
  if (popup) {
    fireEvent.animationEnd(popup);
    fireEvent(popup, new Event('webkitAnimationEnd', { bubbles: true }));
  }
  await waitFor(() =>
    expect(
      screen.getByRole('combobox', { name: 'Channel test mode' }),
    ).toHaveTextContent('Passive recovery'),
  );
  fireEvent.click(screen.getByRole('button', { name: '保存监控设置' }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  expect(API.patch).toHaveBeenCalledExactlyOnceWith(
    '/api/option/request_policy',
    {
      options: {
        'monitor_setting.channel_test_mode': 'passive_recovery',
        'monitor_setting.auto_test_only_auto_disabled': 'true',
      },
    },
  );
});

test('invalid retry codes cannot be saved and corrected codes are normalized in the original form', async () => {
  const refresh = vi.fn();
  vi.spyOn(API, 'patch').mockResolvedValue({ data: { success: true } });
  render(
    <TestContext>
      <SettingsMonitoring
        refresh={refresh}
        options={{ AutomaticRetryStatusCodes: '500-599' }}
      />
    </TestContext>,
  );
  const codes = screen.getByLabelText('自动重试状态码');
  fireEvent.change(codes, { target: { value: 'broken' } });
  fireEvent.click(screen.getByRole('button', { name: '保存监控设置' }));
  expect(screen.getByText('自动重试状态码格式不正确: broken')).toBeVisible();
  expect(API.patch).not.toHaveBeenCalled();
  fireEvent.change(codes, { target: { value: '429, 500-599' } });
  fireEvent.click(screen.getByRole('button', { name: '保存监控设置' }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  expect(API.patch).toHaveBeenCalledExactlyOnceWith(
    '/api/option/request_policy',
    {
      options: { AutomaticRetryStatusCodes: '429,500-599' },
    },
  );
});
