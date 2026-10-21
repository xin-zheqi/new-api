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
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, beforeEach, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { MemoryRouter } from 'react-router-dom';
import { API } from '../../../helpers/api';
import { StatusContext } from '../../../context/Status';
import { UserContext } from '../../../context/User';
import Setting from '../index';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
const originalIntersectionObserver = globalThis.IntersectionObserver;
const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;
beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
  globalThis.IntersectionObserver = originalIntersectionObserver;
  HTMLElement.prototype.scrollIntoView = originalScrollIntoView;
});

test.each([
  ['retry', '运营设置', '失败重试次数'],
  ['routing', '模型相关设置', 'Session behavior'],
])(
  'saved %s policy links open the original editor without a duplicate policy tab',
  async (section, tabName, label) => {
    const user = { id: 1, role: 100 };
    localStorage.setItem('user', JSON.stringify(user));
    vi.spyOn(API, 'get').mockImplementation(async (url) => ({
      data: {
        success: true,
        data:
          url === '/api/option/'
            ? [
                { key: 'RetryTimes', value: '2' },
                { key: 'SensitiveWords', value: 'example' },
                { key: 'channel_affinity_setting.rules', value: '[]' },
                {
                  key: 'channel_affinity_setting.session_mode',
                  value: 'prefer',
                },
              ]
            : {},
      },
    }));
    render(
      <MemoryRouter
        initialEntries={[
          `/console/setting?tab=request-policies&policy=${section}`,
        ]}
      >
        <UserContext.Provider value={[{ user }, vi.fn()]}>
          <StatusContext.Provider value={[{ status: {} }, vi.fn()]}>
            <Setting />
          </StatusContext.Provider>
        </UserContext.Provider>
      </MemoryRouter>,
    );
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: tabName })).toHaveAttribute(
        'aria-selected',
        'true',
      ),
    );
    expect(await screen.findByLabelText(label)).toBeVisible();
    expect(
      screen.queryByRole('tab', { name: 'Request policies' }),
    ).not.toBeInTheDocument();
    if (section === 'retry')
      expect(screen.getByLabelText('屏蔽词列表')).toBeVisible();
  },
);
