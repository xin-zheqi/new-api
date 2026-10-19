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
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { screen, within } from '@testing-library/dom';
import '@testing-library/jest-dom/vitest';
import dayjs from 'dayjs';
import { afterEach, expect, test } from 'vitest';
import { RecentRequests } from '../index';

let root;
let container;

function renderRequests(requests) {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root.render(<RecentRequests requests={requests} />));
}

afterEach(() => {
  if (root) act(() => root.unmount());
  container?.remove();
  root = null;
  container = null;
});

test('request time sits below the model and available token counts are shown', () => {
  const timestamp = 1790985478;
  renderRequests([
    {
      request_id: 'request-1',
      created_at: timestamp,
      model_name: 'deepseek-v4-flash',
      prompt_tokens: 8,
      completion_tokens: 1,
      cache_read_tokens: 3,
      cache_write_tokens: 7,
      cache_write_5m_tokens: 2,
      cache_write_1h_tokens: 5,
      quota: 338,
    },
  ]);

  const model = screen.getByText('deepseek-v4-flash');
  const time = screen.getByText(
    dayjs.unix(timestamp).format('YYYY-MM-DD HH:mm:ss'),
  );
  expect(time.closest('time')).toHaveStyle({ display: 'block' });
  expect(time.closest('time').previousElementSibling).toContainElement(model);

  const record = model.closest('[data-testid="recent-request"]');
  expect(within(record).getByText(/Input tokens:/)).toHaveTextContent('8');
  expect(within(record).getByText(/Output tokens:/)).toHaveTextContent('1');
  expect(within(record).getByText(/Cache read:/)).toHaveTextContent('3');
  expect(within(record).getByText(/Cache write:/)).toHaveTextContent('7');
  expect(within(record).getByText(/Cache write 5m:/)).toHaveTextContent('2');
  expect(within(record).getByText(/Cache write 1h:/)).toHaveTextContent('5');
});

test('a request without a token count does not show an empty token section', () => {
  renderRequests([
    {
      request_id: 'request-2',
      created_at: 1790985478,
      model_name: 'model-without-usage',
      prompt_tokens: 0,
      completion_tokens: 0,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
      quota: 0,
    },
  ]);

  const record = screen
    .getByText('model-without-usage')
    .closest('[data-testid="recent-request"]');
  expect(within(record).queryByText(/Input tokens:/)).not.toBeInTheDocument();
  expect(within(record).queryByText(/Cache write:/)).not.toBeInTheDocument();
  expect(within(record).queryByText(/Total tokens:/)).not.toBeInTheDocument();
});
