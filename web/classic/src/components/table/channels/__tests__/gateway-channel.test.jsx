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
import { afterEach, beforeAll, beforeEach, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { API } from '../../../../helpers/api';
import { UserContext } from '../../../../context/User';
import EditChannelModal from '../modals/EditChannelModal';

beforeAll(() =>
  i18next.use(initReactI18next).init({
    lng: 'zh-CN',
    resources: { 'zh-CN': { translation: {} } },
  }),
);
const originalIntersectionObserver = globalThis.IntersectionObserver;
beforeEach(() => {
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.spyOn(API, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/models')
      return { data: { success: true, data: [{ id: 'gpt-4o-mini' }] } };
    if (url === '/api/group/')
      return { data: { success: true, data: ['default'] } };
    if (url === '/api/prefill_group?type=model')
      return { data: { success: true, data: [] } };
    if (url === '/api/user/2fa/status' || url === '/api/user/passkey')
      return { data: { success: true, data: { enabled: false } } };
    throw new Error('Unexpected request: ' + url);
  });
  vi.spyOn(API, 'post').mockResolvedValue({
    data: { success: true, data: ['gpt-4o-mini'] },
  });
});
afterEach(() => {
  cleanup();
  globalThis.IntersectionObserver = originalIntersectionObserver;
});

test.each([
  { type: 60, name: 'New API' },
  { type: 59, name: 'Sub2API' },
])(
  '$name can be selected in the real classic channel form and fetch models with its type',
  async ({ type, name }) => {
    render(
      <UserContext.Provider value={[{ user: { role: 100 } }, vi.fn()]}>
        <EditChannelModal
          visible
          editingChannel={{}}
          handleClose={vi.fn()}
          refresh={vi.fn()}
        />
      </UserContext.Provider>,
    );
    fireEvent.click(await screen.findByRole('combobox', { name: '类型' }));
    const option = await screen.findByRole('option', { name });
    const dropdown = option.closest('[role="dialog"]');
    fireEvent.click(option);
    // Semi commits controlled selections when its popup close animation ends.
    // jsdom has no animations, so dispatch the browser's completion events.
    fireEvent.animationEnd(dropdown);
    fireEvent(dropdown, new Event('webkitAnimationEnd', { bubbles: true }));
    await waitFor(() =>
      expect(screen.getByLabelText('API地址', { exact: false })).toBeRequired(),
    );
    fireEvent.change(screen.getByLabelText('名称'), {
      target: { value: name + ' fixture' },
    });
    fireEvent.change(screen.getByLabelText('密钥'), {
      target: { value: 'sk-fixture' },
    });
    const base = screen.getByLabelText('API地址', { exact: false });
    expect(base).toBeRequired();
    fireEvent.change(base, { target: { value: 'https://gateway.example' } });
    fireEvent.click(
      screen.getByRole('button', { name: '获取模型列表', exact: true }),
    );
    await waitFor(() =>
      expect(API.post).toHaveBeenCalledWith(
        '/api/channel/fetch_models',
        { base_url: 'https://gateway.example', type, key: 'sk-fixture' },
        { skipErrorHandler: true },
      ),
    );
  },
);
