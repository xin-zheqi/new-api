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
import userEvent from '@testing-library/user-event';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { API } from '../../../../helpers/api';
import SettingsChannelAffinity from '../../Operation/SettingsChannelAffinity';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(cleanup);

test('visually editing a session rule preserves advanced JSON fields and saves all changes atomically', async () => {
  const user = userEvent.setup();
  vi.spyOn(API, 'get').mockResolvedValue({
    data: { success: true, data: { total: 0, by_rule_name: {} } },
  });
  vi.spyOn(API, 'patch').mockResolvedValue({ data: { success: true } });
  vi.spyOn(API, 'put');
  const rule = {
    name: 'Original',
    model_regex: ['.*'],
    key_sources: [{ type: 'request_header', key: 'X-Session' }],
    session_mode: 'strict',
    extension: { preserve: false },
    param_override_template: { temperature: 0 },
  };
  render(
    <SettingsChannelAffinity
      options={{
        'channel_affinity_setting.enabled': 'true',
        'channel_affinity_setting.rules': JSON.stringify([rule]),
        'channel_affinity_setting.session_mode': 'prefer',
      }}
      refresh={vi.fn()}
    />,
  );
  fireEvent.click(await screen.findByRole('button', { name: '编辑规则' }));
  const dialog = await screen.findByRole('dialog');
  await user.clear(within(dialog).getByLabelText('名称'));
  await user.type(within(dialog).getByLabelText('名称'), 'Edited');
  await user.click(within(dialog).getByRole('button', { name: 'confirm' }));
  expect(await screen.findByText('Edited')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }));
  await waitFor(() => expect(API.patch).toHaveBeenCalled());
  const payload = API.patch.mock.calls[0][1].options;
  expect(
    JSON.parse(payload['channel_affinity_setting.rules'])[0],
  ).toMatchObject({ ...rule, name: 'Edited' });
  expect(payload).not.toHaveProperty('channel_affinity_setting.session_mode');
  expect(API.put).not.toHaveBeenCalled();
});
