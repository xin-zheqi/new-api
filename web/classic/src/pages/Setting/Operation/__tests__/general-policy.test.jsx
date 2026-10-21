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
import { API } from '../../../../helpers/api';
import SettingsGeneral from '../SettingsGeneral';
import SettingsSensitiveWords from '../SettingsSensitiveWords';

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

test('retry count remains editable in the original general settings and saves only its own key', async () => {
  const refresh = vi.fn();
  vi.spyOn(API, 'put').mockResolvedValue({ data: { success: true } });
  render(<SettingsGeneral options={{ RetryTimes: '2' }} refresh={refresh} />);
  fireEvent.change(screen.getByLabelText('失败重试次数'), {
    target: { value: '3' },
  });
  fireEvent.click(screen.getByRole('button', { name: '保存通用设置' }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  expect(API.put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
    key: 'RetryTimes',
    value: '3',
  });
});

test('a rejected retry value stays editable and is not reported as saved', async () => {
  const refresh = vi.fn();
  vi.spyOn(API, 'put').mockResolvedValue({
    data: { success: false, message: 'Invalid retry count' },
  });
  render(<SettingsGeneral options={{ RetryTimes: '2' }} refresh={refresh} />);
  fireEvent.change(screen.getByLabelText('失败重试次数'), {
    target: { value: '-1' },
  });
  fireEvent.click(screen.getByRole('button', { name: '保存通用设置' }));
  expect(await screen.findByText('错误：Invalid retry count')).toBeVisible();
  expect(screen.getByLabelText('失败重试次数')).toHaveValue('-1');
  expect(refresh).not.toHaveBeenCalled();
});

test('the original sensitive-word form atomically saves filtering changes including an empty list', async () => {
  const refresh = vi.fn();
  vi.spyOn(API, 'patch').mockResolvedValue({ data: { success: true } });
  vi.spyOn(API, 'put').mockResolvedValue({ data: { success: true } });
  render(
    <SettingsSensitiveWords
      options={{
        CheckSensitiveEnabled: false,
        CheckSensitiveOnPromptEnabled: true,
        SensitiveWords: 'example',
      }}
      refresh={refresh}
    />,
  );
  fireEvent.click(screen.getByRole('switch', { name: '启用屏蔽词过滤功能' }));
  fireEvent.change(screen.getByLabelText('屏蔽词列表'), {
    target: { value: '' },
  });
  fireEvent.click(screen.getByRole('button', { name: '保存屏蔽词过滤设置' }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  expect(API.patch).toHaveBeenCalledExactlyOnceWith(
    '/api/option/request_policy',
    { options: { CheckSensitiveEnabled: 'true', SensitiveWords: '' } },
  );
  expect(API.put).not.toHaveBeenCalled();
});
