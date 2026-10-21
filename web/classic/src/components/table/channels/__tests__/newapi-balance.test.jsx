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
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
  vi,
} from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { Modal, Toast } from '@douyinfe/semi-ui';
import { API } from '../../../../helpers/api';
import { useChannelsData } from '../../../../hooks/channels/useChannelsData';
import { getChannelsColumns } from '../ChannelsColumnDefs';
import ChannelsActions from '../ChannelsActions';

// Exercise the existing balance cell, batch controls and data hook together.
// Only HTTP is replaced; New API and Sub2API share the existing channel UI path.
function BalanceControls({ hideBalance = false }) {
  const data = useChannelsData();
  const columns = getChannelsColumns(data);
  const balanceColumn = columns.find(
    (column) => column.key === data.COLUMN_KEYS.BALANCE,
  );
  const typeColumn = columns.find(
    (column) => column.key === data.COLUMN_KEYS.TYPE,
  );
  const actionColumn = columns.find(
    (column) => column.key === data.COLUMN_KEYS.OPERATE,
  );
  const channels = data.channels.flatMap((channel) =>
    channel.children === undefined ? [channel] : channel.children,
  );
  return (
    <>
      <ChannelsActions {...data} />
      {channels.map((channel) => (
        <article key={channel.id} aria-label={channel.name}>
          {typeColumn.render(channel.type, channel)}
          {hideBalance
            ? actionColumn.render(undefined, channel)
            : balanceColumn.render(undefined, channel)}
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

describe.each([
  {
    name: 'New API',
    type: 60,
    error: 'Enable "Allow querying user quota" on the upstream',
  },
  {
    name: 'Sub2API',
    type: 59,
    error:
      'Sub2API /v1/usage reports an unlimited subscription without a finite balance',
  },
])('$name classic balance controls', ({ name, type, error }) => {
  beforeEach(() => {
    localStorage.clear();
    balanceResponse = { success: true, balance: 2.5 };
    vi.spyOn(API, 'get').mockImplementation(async (url) => {
      if (url === `/api/channel/update_balance/${type}/`) {
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
                  id: type,
                  name: `${name} fixture`,
                  type,
                  balance: 19,
                  balance_updated_time: 123,
                  used_quota: 500000,
                  group: 'default',
                  tag: 'gateway',
                  status: 1,
                },
              ],
              total: 1,
              type_counts: { [type]: 1 },
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

  afterEach(async () => {
    cleanup();
    await act(async () => {
      Toast.destroyAll();
      Modal.destroyAll();
    });
    // jsdom has no CSS animations. Complete the close animation so Semi's
    // imperative modal root is unmounted before the next provider fixture.
    await act(async () => {
      for (const dialog of screen.queryAllByRole('dialog')) {
        fireEvent.animationEnd(dialog);
        // React chooses the prefixed event when jsdom lacks AnimationEvent.
        fireEvent(dialog, new Event('webkitAnimationEnd', { bubbles: true }));
      }
    });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    localStorage.clear();
  });

  test.each([
    { tagMode: false, balance: 2.5, display: '$2.50' },
    { tagMode: true, balance: 0, display: '$0.00' },
  ])(
    'clicking a channel balance refreshes its displayed amount (tag mode: $tagMode, balance: $balance)',
    async ({ tagMode, balance, display }) => {
      localStorage.setItem('enable-tag-mode', String(tagMode));
      balanceResponse = { success: true, balance };
      render(<BalanceControls />);
      const channel = within(
        await screen.findByRole('article', { name: `${name} fixture` }),
      );
      fireEvent.click(channel.getByText('$19.00'));
      expect(
        await screen.findByText(`通道 ${name} fixture 余额更新成功！`),
      ).toBeInTheDocument();
      expect(channel.queryByText('$19.00')).not.toBeInTheDocument();
      expect(channel.getByText(display)).toBeInTheDocument();
      expect(API.get).toHaveBeenCalledWith(
        `/api/channel/update_balance/${type}/`,
      );
    },
  );

  test('a rejected query displays the upstream requirement and preserves the previous balance', async () => {
    const message = error;
    balanceResponse = { success: false, message };
    vi.spyOn(console, 'error').mockImplementation(() => {});
    render(<BalanceControls />);
    const channel = within(
      await screen.findByRole('article', { name: `${name} fixture` }),
    );
    fireEvent.click(channel.getByText('$19.00'));
    expect(await screen.findByText(`错误：${message}`)).toBeInTheDocument();
    expect(channel.getByText('$19.00')).toBeInTheDocument();
    expect(
      screen.queryByText(`通道 ${name} fixture 余额更新成功！`),
    ).not.toBeInTheDocument();
  });

  test('the channel type is named and a visible balance query button refreshes it', async () => {
    render(<BalanceControls />);
    const channel = within(
      await screen.findByRole('article', { name: `${name} fixture` }),
    );
    expect(channel.getByText(name)).toBeInTheDocument();
    fireEvent.click(channel.getByRole('button', { name: '余额查询' }));
    expect(await channel.findByText('$2.50')).toBeInTheDocument();
    expect(API.get).toHaveBeenCalledWith(
      `/api/channel/update_balance/${type}/`,
    );
  });

  test('the more menu can query balance when its table column is hidden', async () => {
    render(<BalanceControls hideBalance />);
    const channel = within(
      await screen.findByRole('article', { name: `${name} fixture` }),
    );
    fireEvent.click(channel.getByRole('button', { name: '更多' }));
    fireEvent.click(await screen.findByText('余额查询'));
    expect(
      await screen.findByText(`通道 ${name} fixture 余额更新成功！`),
    ).toBeInTheDocument();
    expect(API.get).toHaveBeenCalledWith(
      `/api/channel/update_balance/${type}/`,
    );
  });

  test('confirming the classic batch balance action calls the shared updater and shows completion', async () => {
    render(<BalanceControls />);
    await screen.findByRole('article', { name: `${name} fixture` });
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
});

test.each([
  { type: 14, channel_info: {} },
  { type: 24, channel_info: {} },
  { type: 3, channel_info: {} },
  { type: 60, channel_info: { is_multi_key: true } },
])(
  'unsupported or multi-key balance controls do not issue a query ($type)',
  (record) => {
    const updateChannelBalance = vi.fn();
    const column = getChannelsColumns({
      t: (key) => key,
      COLUMN_KEYS: { BALANCE: 'balance' },
      updateChannelBalance,
    }).find((column) => column.key === 'balance');
    render(column.render(undefined, { ...record, balance: 0, used_quota: 0 }));
    const query = screen.getByRole('button', { name: '余额查询' });
    expect(query).toBeDisabled();
    expect(query).toHaveTextContent('—');
    fireEvent.click(query);
    expect(updateChannelBalance).not.toHaveBeenCalled();
    cleanup();
  },
);
