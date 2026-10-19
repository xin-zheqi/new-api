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
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, beforeEach, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { Modal, Toast } from '@douyinfe/semi-ui';
import { API } from '../../../../helpers/api';
import { useChannelsData } from '../../../../hooks/channels/useChannelsData';
import { getChannelsColumns } from '../ChannelsColumnDefs';
import ChannelsActions from '../ChannelsActions';

// Exercise the existing balance cell, batch controls and data hook together.
// Only HTTP is replaced; New API must use the same UI path as other channels.
function BalanceControls() {
  const data = useChannelsData();
  const balanceColumn = getChannelsColumns(data).find(
    (column) => column.key === data.COLUMN_KEYS.BALANCE,
  );
  const channels = data.channels.flatMap((channel) =>
    channel.children === undefined ? [channel] : channel.children,
  );
  return (
    <>
      <ChannelsActions {...data} />
      {channels.map((channel) => (
        <article key={channel.id} aria-label={channel.name}>
          {balanceColumn.render(undefined, channel)}
        </article>
      ))}
    </>
  );
}

let balanceResponse;

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'zh-CN', resources: { 'zh-CN': { translation: {} } } }),
);

beforeEach(() => {
  localStorage.clear();
  balanceResponse = { success: true, balance: 2.5 };
  vi.spyOn(API, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/update_balance/60/') {
      return { data: balanceResponse };
    }
    if (url === '/api/channel/update_balance') {
      return { data: { success: true, message: '' } };
    }
    if (url.startsWith('/api/channel/?')) {
      return {
        data: {
          success: true,
          data: {
            items: [
              {
                id: 60,
                name: 'New API fixture',
                type: 60,
                balance: 19,
                balance_updated_time: 123,
                used_quota: 500000,
                group: 'default',
                tag: 'gateway',
                status: 1,
              },
            ],
            total: 1,
            type_counts: { 60: 1 },
          },
        },
      };
    }
    if (['/api/group/', '/api/models', '/api/option/'].includes(url)) {
      return { data: { success: true, data: [] } };
    }
    throw new Error(`Unexpected request: ${url}`);
  });
});

afterEach(() => {
  cleanup();
  act(() => {
    Toast.destroyAll();
    Modal.destroyAll();
  });
  localStorage.clear();
});

test.each([
  { tagMode: false, balance: 2.5, display: '$2.50' },
  { tagMode: true, balance: 0, display: '$0.00' },
])(
  'clicking a New API balance refreshes its displayed amount (tag mode: $tagMode, balance: $balance)',
  async ({ tagMode, balance, display }) => {
    localStorage.setItem('enable-tag-mode', String(tagMode));
    balanceResponse = { success: true, balance };
    render(<BalanceControls />);
    const channel = within(
      await screen.findByRole('article', { name: 'New API fixture' }),
    );
    fireEvent.click(channel.getByText('$19.00'));
    expect(
      await screen.findByText('通道 New API fixture 余额更新成功！'),
    ).toBeInTheDocument();
    expect(channel.queryByText('$19.00')).not.toBeInTheDocument();
    expect(channel.getByText(display)).toBeInTheDocument();
    expect(API.get).toHaveBeenCalledWith('/api/channel/update_balance/60/');
  },
);

test('a rejected New API query displays the upstream requirement and preserves the previous balance', async () => {
  const message = 'Enable "Allow querying user quota" on the upstream';
  balanceResponse = { success: false, message };
  vi.spyOn(console, 'error').mockImplementation(() => {});
  render(<BalanceControls />);
  const channel = within(
    await screen.findByRole('article', { name: 'New API fixture' }),
  );
  fireEvent.click(channel.getByText('$19.00'));
  expect(await screen.findByText(`错误：${message}`)).toBeInTheDocument();
  expect(channel.getByText('$19.00')).toBeInTheDocument();
  expect(
    screen.queryByText('通道 New API fixture 余额更新成功！'),
  ).not.toBeInTheDocument();
});

test('confirming the classic batch balance action calls the shared updater and shows completion', async () => {
  render(<BalanceControls />);
  await screen.findByRole('article', { name: 'New API fixture' });
  fireEvent.click(screen.getByRole('button', { name: '批量操作' }));
  fireEvent.click(
    await screen.findByRole('button', { name: '更新所有已启用通道余额' }),
  );
  const dialog = within(await screen.findByRole('dialog'));
  expect(
    dialog.getByText('确定要更新所有已启用通道余额吗？'),
  ).toBeInTheDocument();
  expect(API.get).not.toHaveBeenCalledWith('/api/channel/update_balance');
  fireEvent.click(dialog.getByRole('button', { name: 'confirm' }));
  expect(
    await screen.findByText('已更新完毕所有已启用通道余额！'),
  ).toBeInTheDocument();
  expect(API.get).toHaveBeenCalledWith('/api/channel/update_balance');
});
