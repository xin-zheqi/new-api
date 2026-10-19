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
import PolicyDecisionRecord from '../PolicyDecisionRecord';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(cleanup);
test('a started stream displays the non-retry decision and channel without rendering embedded markup', () => {
  render(
    <PolicyDecisionRecord
      events={[
        {
          attempt: 1,
          channel_id: 3,
          group: '<script>bad()</script>',
          elapsed_ms: 20,
          decision: {
            reason: 'response_started',
            source: 'system',
            action: 'stop',
          },
        },
      ]}
    />,
  );
  expect(
    screen.getByRole('list', { name: 'Request decision flow' }),
  ).toHaveTextContent('Response already started; retry is unsafe · #3');
  expect(screen.getByText(/bad/)).toHaveTextContent('<script>bad()</script>');
  expect(document.querySelector('script')).toBeNull();
});
