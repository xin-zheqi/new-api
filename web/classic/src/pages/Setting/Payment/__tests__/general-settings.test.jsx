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
import { MemoryRouter } from 'react-router-dom';
import { API } from '../../../../helpers/api';
import SettingsGeneralPayment from '../SettingsGeneralPayment';

beforeAll(() =>
  i18next.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  }),
);
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
});

const options = {
  ServerAddress: 'https://old-site.example.com',
  CustomCallbackAddress: 'https://payments.example.com',
  PaymentMethodSettings: '[]',
  'payment_setting.balance_subscription_enabled': false,
};

test('payment settings show the global address with a link to its only editor', () => {
  render(
    <MemoryRouter>
      <SettingsGeneralPayment options={options} />
    </MemoryRouter>,
  );
  expect(
    screen.queryByRole('textbox', { name: '服务器地址' }),
  ).not.toBeInTheDocument();
  expect(screen.getByText(options.ServerAddress)).toBeVisible();
  expect(screen.getByRole('link', { name: '系统设置' })).toHaveAttribute(
    'href',
    '/console/setting?tab=system',
  );
});

test('saving a payment switch never resubmits stale global or unchanged callback addresses', async () => {
  const refresh = vi.fn();
  vi.spyOn(API, 'put').mockResolvedValue({ data: { success: true } });
  render(
    <MemoryRouter>
      <SettingsGeneralPayment options={options} refresh={refresh} />
    </MemoryRouter>,
  );
  fireEvent.click(
    screen.getByRole('switch', {
      name: 'Enable balance subscription purchases',
    }),
  );
  fireEvent.click(screen.getByRole('button', { name: '保存通用设置' }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  expect(API.put).toHaveBeenCalledTimes(1);
  expect(API.put).toHaveBeenCalledWith('/api/option/', {
    key: 'payment_setting.balance_subscription_enabled',
    value: true,
  });
});

test.each([
  ['', ''],
  ['https://new-payments.example.com/', 'https://new-payments.example.com'],
])(
  'saving callback %j preserves clearing and URL normalization',
  async (input, expected) => {
    const refresh = vi.fn();
    vi.spyOn(API, 'put').mockResolvedValue({ data: { success: true } });
    render(
      <MemoryRouter>
        <SettingsGeneralPayment options={options} refresh={refresh} />
      </MemoryRouter>,
    );
    const callbackAddress = screen.getByRole('textbox', { name: '回调地址' });
    await waitFor(() =>
      expect(callbackAddress).toHaveValue(options.CustomCallbackAddress),
    );
    fireEvent.change(callbackAddress, {
      target: { value: input },
    });
    fireEvent.click(screen.getByRole('button', { name: '保存通用设置' }));
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
    expect(API.put).toHaveBeenCalledTimes(1);
    expect(API.put).toHaveBeenCalledWith('/api/option/', {
      key: 'CustomCallbackAddress',
      value: expected,
    });
  },
);
