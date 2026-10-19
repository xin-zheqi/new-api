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
import ToolPriceSettings from '../ToolPriceSettings';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(cleanup);

test('new DeepSeek and grounding prices are added while explicit zero and custom prices are retained', async () => {
  vi.spyOn(API, 'put').mockResolvedValue({ data: { success: true } });
  render(
    <ToolPriceSettings
      options={{
        'tool_price_setting.prices': '{"deepseek_search":0,"search_std":11}',
      }}
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: '保存' }));
  await waitFor(() => expect(API.put).toHaveBeenCalled());
  expect(JSON.parse(API.put.mock.calls[0][1].value)).toMatchObject({
    deepseek_search: 0,
    deepseek_fetch: 0.75,
    search_std: 11,
    'google_search:gemini-2.5*': 0,
    'google_search_grounded_prompt:gemini-2.5*': 35,
  });
});

test('negative JSON tool prices cannot be saved as a previous valid draft', () => {
  vi.spyOn(API, 'put');
  render(<ToolPriceSettings options={{}} />);
  fireEvent.click(screen.getByRole('radio', { name: 'JSON' }));
  fireEvent.change(screen.getByRole('textbox'), {
    target: { value: '{"deepseek_search":-1}' },
  });
  expect(screen.getByRole('button', { name: '保存' })).toBeDisabled();
  expect(API.put).not.toHaveBeenCalled();
});
