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
import React, { useState } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, expect, test } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import AdvancedCustomRoutes from '../AdvancedCustomRoutes';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(cleanup);
function Fixture({ initial }) {
  const [value, setValue] = useState(initial);
  return <AdvancedCustomRoutes value={value} onChange={setValue} />;
}
test('adding a Gemini Responses converter preserves existing routes and extension fields', () => {
  const existing = {
    incoming_path: '/v1/alpha/search',
    upstream_path: '/v1/alpha/search',
    converter: 'none',
    models: ['deepseek-search'],
  };
  render(
    <Fixture
      initial={JSON.stringify({ advanced_routes: [existing], extension: true })}
    />,
  );
  fireEvent.click(
    screen.getByRole('button', {
      name: 'Gemini Generate Content to OpenAI Responses',
    }),
  );
  expect(JSON.parse(screen.getByLabelText('Advanced routes').value)).toEqual({
    extension: true,
    advanced_routes: [
      existing,
      {
        incoming_path: '/v1beta/models/{model}:generateContent',
        upstream_path: '/v1/responses',
        converter: 'gemini_generate_content_to_openai_responses',
        auth: {
          type: 'header',
          name: 'Authorization',
          value: 'Bearer {api_key}',
        },
      },
    ],
  });
});
test('invalid JSON remains editable and prevents template insertion', () => {
  render(<Fixture initial='{' />);
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Advanced routes JSON is invalid',
  );
  expect(
    screen.getByRole('button', {
      name: 'Anthropic Messages to OpenAI Responses',
    }),
  ).toBeDisabled();
  fireEvent.change(screen.getByLabelText('Advanced routes'), {
    target: { value: '{"advanced_routes":[]}' },
  });
  expect(
    screen.getByRole('button', {
      name: 'Anthropic Messages to OpenAI Responses',
    }),
  ).toBeEnabled();
});
