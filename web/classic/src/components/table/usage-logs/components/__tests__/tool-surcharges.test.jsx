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
import { cleanup, render, screen } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, expect, test } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import ToolSurchargeDetails from '../ToolSurchargeDetails';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(cleanup);
test('DeepSeek and grounding usage show explicit per-thousand-call prices and omit uncharged items', () => {
  render(
    <ToolSurchargeDetails
      items={[
        { name: 'deepseek_search', count: 2, price: 0.75 },
        { name: 'google_search_grounded_prompt', count: 1, price: 35 },
        { name: 'free', count: 1, price: 0 },
      ]}
    />,
  );
  expect(
    screen.getByRole('columnheader', { name: 'Price ($/1K calls)' }),
  ).toBeVisible();
  expect(
    screen.getByRole('row', { name: 'deepseek_search 2 0.75' }),
  ).toBeVisible();
  expect(
    screen.getByRole('row', { name: 'google_search_grounded_prompt 1 35' }),
  ).toBeVisible();
  expect(screen.queryByText('free')).not.toBeInTheDocument();
});
