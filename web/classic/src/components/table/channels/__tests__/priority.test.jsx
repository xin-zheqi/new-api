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
  within,
} from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, beforeEach, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { API } from '../../../../helpers/api';
import { ChannelPriorityEditor } from '../ChannelPriorityOrder';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(cleanup);
const original = {
  revision: 'v1',
  items: [
    { id: 1, priority: 100 },
    { id: 2, priority: 0 },
  ],
  changes: [],
};
const changed = {
  revision: 'v2',
  items: [
    { id: 2, priority: 110 },
    { id: 1, priority: 100 },
  ],
  changes: [{ id: 2, previous: 0, priority: 110 }],
};
beforeEach(() => {
  vi.spyOn(API, 'get').mockResolvedValue({
    data: { success: true, data: original },
  });
  vi.spyOn(API, 'post').mockImplementation(async (_url, body) => ({
    data: { success: true, data: body.restore ? original : changed },
  }));
});

async function chooseOption(label, optionName) {
  fireEvent.click(
    await screen.findByRole('combobox', { name: label, exact: true }),
  );
  const option = await screen.findByRole('option', { name: optionName });
  const popup = option.closest('[role="dialog"]');
  fireEvent.click(option);
  if (popup) {
    fireEvent.animationEnd(popup);
    fireEvent(popup, new Event('webkitAnimationEnd', { bubbles: true }));
  }
}

test('dropdown controls preview the full order, commit with the original revision and undo with the saved revision', async () => {
  render(<ChannelPriorityEditor channels={[{ id: 1, name: 'Primary' }]} />);
  await chooseOption('Channel', /#2/);
  await chooseOption('Target channel', /#1 Primary/);
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
  expect(await screen.findByRole('status')).toHaveTextContent('#2: 0 → 110');
  for (const control of screen.getAllByRole('combobox')) {
    expect(control).toHaveAttribute('aria-disabled', 'true');
  }
  expect(API.post).toHaveBeenLastCalledWith('/api/channel/priority/reorder', {
    id: 2,
    target_id: 1,
    position: 'before',
    revision: 'v1',
    preview: true,
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
  const undo = await screen.findByRole('button', { name: 'Undo last move' });
  expect(API.post).toHaveBeenLastCalledWith('/api/channel/priority/reorder', {
    id: 2,
    target_id: 1,
    position: 'before',
    revision: 'v1',
    preview: false,
  });
  fireEvent.click(undo);
  await waitFor(() =>
    expect(API.post).toHaveBeenLastCalledWith('/api/channel/priority/reorder', {
      revision: 'v2',
      restore: [{ id: 2, previous: 110, priority: 0 }],
      preview: false,
    }),
  );
});

test('a drag previews a same-tier move and cancellation sends no commit', async () => {
  render(<ChannelPriorityEditor />);
  const handle = await screen.findByRole('button', { name: 'Move channel #2' });
  const target = screen
    .getByRole('button', { name: 'Move channel #1' })
    .closest('tr');
  vi.spyOn(target, 'getBoundingClientRect').mockReturnValue({
    top: 0,
    height: 100,
  });
  fireEvent.dragStart(handle, { dataTransfer: { setData: vi.fn() } });
  fireEvent.drop(target, { clientY: 50 });
  await screen.findByRole('status');
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(screen.queryByRole('status')).not.toBeInTheDocument();
  expect(API.post).toHaveBeenCalledTimes(1);
  expect(API.post.mock.calls[0][1]).toMatchObject({
    preview: true,
    position: 'same',
  });
});

test('a rejected revision is shown and refreshed without silently retrying the mutation', async () => {
  API.post.mockResolvedValue({
    data: {
      success: false,
      message: 'Channel priorities changed; refresh and retry',
    },
  });
  render(<ChannelPriorityEditor />);
  await chooseOption('Channel', /#2/);
  await chooseOption('Target channel', /#1/);
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Channel priorities changed',
  );
  expect(
    screen.queryByRole('button', { name: 'Save Changes' }),
  ).not.toBeInTheDocument();
  expect(API.post).toHaveBeenCalledTimes(1);
  expect(API.get).toHaveBeenCalledTimes(2);
});

test('channel search filters by name and keyboard selection prepares a move without committing', async () => {
  render(
    <ChannelPriorityEditor
      channels={[
        { id: 1, name: 'Primary' },
        { id: 2, name: 'Backup' },
      ]}
    />,
  );
  const channel = await screen.findByRole('combobox', {
    name: 'Channel',
    exact: true,
  });
  fireEvent.click(channel);
  const search = within(channel).getByRole('textbox');
  fireEvent.change(search, { target: { value: 'Backup' } });
  await screen.findByRole('option', { name: /#2 Backup/ });
  expect(
    screen.queryByRole('option', { name: /#1 Primary/ }),
  ).not.toBeInTheDocument();
  fireEvent.keyDown(search, { key: 'ArrowDown', keyCode: 40 });
  fireEvent.keyDown(search, { key: 'Enter', keyCode: 13 });
  await waitFor(() => expect(channel).toHaveTextContent('#2 Backup'));
  expect(API.post).not.toHaveBeenCalled();
});
