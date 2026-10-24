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
import userEvent from '@testing-library/user-event';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, expect, test } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import ResponseModelMismatch from '../ResponseModelMismatch';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(cleanup);
const mismatch = {
  requested_model: 'public-model',
  response_models: ['provider-v2', '<img src=x onerror=alert(1)>'],
  truncated: true,
};

test('ordinary users cannot see provider identifiers even if admin metadata is present', () => {
  const { container } = render(
    <ResponseModelMismatch isAdmin={false} mismatch={mismatch} />,
  );
  expect(container).toBeEmptyDOMElement();
});

test('old or matching logs do not add a mismatch indicator', () => {
  const { container } = render(<ResponseModelMismatch isAdmin />);
  expect(container).toBeEmptyDOMElement();
});

test('admins open model details by keyboard and provider values remain escaped text', async () => {
  const user = userEvent.setup();
  render(<ResponseModelMismatch isAdmin mismatch={mismatch} />);
  await user.tab();
  expect(
    screen.getByRole('button', { name: 'Response model mismatch' }),
  ).toHaveFocus();
  await user.keyboard('{Enter}');
  expect(await screen.findByText('public-model')).toBeVisible();
  expect(screen.getByText('provider-v2')).toBeVisible();
  expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeVisible();
  expect(document.querySelector('img')).toBeNull();
  expect(
    screen.getByText('Additional model names were omitted.'),
  ).toBeVisible();
});

test('expanded log details retain long model names without another popup', () => {
  const longModel = 'provider-' + 'long-version-'.repeat(15);
  render(
    <ResponseModelMismatch
      isAdmin
      expanded
      mismatch={{ ...mismatch, response_models: [longModel] }}
    />,
  );
  expect(screen.getByText(longModel)).toBeVisible();
  expect(screen.queryByText('Response model mismatch')).not.toBeInTheDocument();
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
});
