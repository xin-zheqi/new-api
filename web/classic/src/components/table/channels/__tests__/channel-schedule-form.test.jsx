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
import { afterEach, beforeAll, beforeEach, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { API } from '../../../../helpers/api';
import { UserContext } from '../../../../context/User';
import EditChannelModal from '../modals/EditChannelModal';
import ChannelScheduleEditor from '../modals/ChannelScheduleEditor';
import { getChannelsColumns } from '../ChannelsColumnDefs';

// Provider artwork loads browser-only dependencies and is unrelated to scheduling.
vi.mock('@lobehub/icons', () => {
  const Icon = () => null;
  Icon.Color = Icon;
  Icon.Combine = Icon;
  return Object.fromEntries(
    'OpenAI Claude Gemini Moonshot Zhipu Qwen DeepSeek Minimax Wenxin Spark Midjourney Hunyuan Cohere Cloudflare Ai360 Yi Jina Mistral XAI Ollama Doubao Suno Xinference OpenRouter Dify Coze SiliconCloud FastGPT Kling Jimeng Perplexity Replicate'
      .split(' ')
      .map((name) => [name, Icon]),
  );
});

const existingSettings = {
  reasoning_content_repair: true,
  tool_output_repair: true,
  reasoning_effort_mapping: { xhigh: 'max' },
};
const schedule = {
  timezone: 'Asia/Shanghai',
  periods: [{ start: '09:00', end: '18:00' }],
};
const originalIntersectionObserver = globalThis.IntersectionObserver;

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'zh-CN', resources: { 'zh-CN': { translation: {} } } }),
);

beforeEach(() => {
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.spyOn(API, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/models')
      return { data: { success: true, data: [{ id: 'gpt-4o-mini' }] } };
    if (url === '/api/group/')
      return { data: { success: true, data: ['default'] } };
    if (url === '/api/prefill_group?type=model')
      return { data: { success: true, data: [] } };
    if (url === '/api/user/2fa/status' || url === '/api/user/passkey')
      return { data: { success: true, data: { enabled: false } } };
    throw new Error('Unexpected request: ' + url);
  });
  vi.spyOn(API, 'put').mockResolvedValue({ data: { success: true } });
  vi.spyOn(API, 'post').mockResolvedValue({ data: { success: true } });
});

afterEach(() => {
  cleanup();
  globalThis.IntersectionObserver = originalIntersectionObserver;
});

function renderChannelForm(activeHours = schedule) {
  const baseGet = API.get.getMockImplementation();
  const channel = {
    id: 123,
    type: 43,
    name: 'Scheduled fixture',
    key: '',
    status: 1,
    base_url: 'https://example.test',
    models: 'gpt-4o-mini',
    group: 'default',
    model_mapping: '',
    param_override: '',
    header_override: '',
    status_code_mapping: '',
    error_message_mapping: '',
    priority: 0,
    weight: 0,
    setting: JSON.stringify({
      ...existingSettings,
      channel_schedule: activeHours,
    }),
  };
  API.get.mockImplementation(async (url, ...args) =>
    url === '/api/channel/123'
      ? { data: { success: true, data: structuredClone(channel) } }
      : baseGet(url, ...args),
  );
  return render(
    <UserContext.Provider value={[{ user: { role: 100 } }, vi.fn()]}>
      <EditChannelModal
        visible
        editingChannel={{ id: 123 }}
        handleClose={vi.fn()}
        refresh={vi.fn()}
      />
    </UserContext.Provider>,
  );
}

test('an existing schedule loads, adds an overnight range and preserves compatibility settings on save', async () => {
  renderChannelForm();
  expect(await screen.findByLabelText('Start time')).toHaveValue('09:00');
  expect(screen.getByLabelText('End time')).toHaveValue('18:00');
  expect(screen.getByRole('textbox', { name: 'Time zone' })).toHaveValue(
    'Asia/Shanghai',
  );
  fireEvent.click(screen.getByRole('button', { name: 'Add time range' }));
  fireEvent.change(screen.getAllByLabelText('Start time')[1], {
    target: { value: '22:00' },
  });
  fireEvent.change(screen.getAllByLabelText('End time')[1], {
    target: { value: '04:00' },
  });
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.put).toHaveBeenCalled());
  const payload = API.put.mock.calls[0][1];
  expect(JSON.parse(payload.setting)).toMatchObject({
    ...existingSettings,
    channel_schedule: {
      ...schedule,
      periods: [...schedule.periods, { start: '22:00', end: '04:00' }],
    },
  });
  expect(payload).not.toHaveProperty('channel_schedule_periods');
  expect(payload).not.toHaveProperty('channel_schedule_timezone');
  expect(payload).not.toHaveProperty('status');
});

test('removing all time ranges omits the schedule and preserves the remaining channel settings', async () => {
  renderChannelForm();
  fireEvent.click(
    await screen.findByRole('button', { name: 'Remove time range' }),
  );
  expect(screen.queryByLabelText('Start time')).not.toBeInTheDocument();
  expect(
    screen.queryByRole('textbox', { name: 'Time zone' }),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.put).toHaveBeenCalled());
  expect(JSON.parse(API.put.mock.calls[0][1].setting)).toMatchObject(
    existingSettings,
  );
  expect(JSON.parse(API.put.mock.calls[0][1].setting)).not.toHaveProperty(
    'channel_schedule',
  );
});

test('an existing unrestricted channel retains the no-schedule default when saved', async () => {
  renderChannelForm(null);
  await screen.findByRole('button', { name: 'Add time range' });
  expect(screen.queryByLabelText('Start time')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.put).toHaveBeenCalled());
  expect(JSON.parse(API.put.mock.calls[0][1].setting)).not.toHaveProperty(
    'channel_schedule',
  );
});

test('incomplete ranges show validation and do not write the channel', async () => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  renderChannelForm();
  fireEvent.change(await screen.findByLabelText('End time'), {
    target: { value: '' },
  });
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  expect(await screen.findByText(/Time ranges must use HH:mm/)).toBeVisible();
  expect(API.put).not.toHaveBeenCalled();
});

test('editing the timezone preserves the daily boundaries when saving', async () => {
  renderChannelForm();
  const zone = await screen.findByRole('textbox', { name: 'Time zone' });
  fireEvent.change(zone, { target: { value: 'America/New_York' } });
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.put).toHaveBeenCalled());
  expect(JSON.parse(API.put.mock.calls[0][1].setting).channel_schedule).toEqual(
    {
      timezone: 'America/New_York',
      periods: schedule.periods,
    },
  );
});

test('a new channel can configure its first schedule in advanced channel behavior', async () => {
  render(
    <UserContext.Provider value={[{ user: { role: 100 } }, vi.fn()]}>
      <EditChannelModal
        visible
        editingChannel={{}}
        handleClose={vi.fn()}
        refresh={vi.fn()}
      />
    </UserContext.Provider>,
  );
  fireEvent.change(await screen.findByLabelText('名称'), {
    target: { value: 'New scheduled fixture' },
  });
  fireEvent.change(screen.getByLabelText('密钥'), {
    target: { value: 'sk-fixture' },
  });
  fireEvent.change(screen.getByLabelText('自定义模型名称'), {
    target: { value: 'gpt-4o-mini' },
  });
  fireEvent.click(screen.getByRole('button', { name: '填入', exact: true }));
  fireEvent.click(screen.getByText('高级设置'));
  fireEvent.click(
    await screen.findByRole('button', { name: 'Add time range' }),
  );
  expect(screen.getByRole('textbox', { name: 'Time zone' })).toHaveValue(
    'Asia/Shanghai',
  );
  fireEvent.change(screen.getByLabelText('Start time'), {
    target: { value: '00:00' },
  });
  fireEvent.change(screen.getByLabelText('End time'), {
    target: { value: '24:00' },
  });
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.post).toHaveBeenCalled());
  expect(
    JSON.parse(API.post.mock.calls[0][1].channel.setting).channel_schedule,
  ).toEqual({
    timezone: 'Asia/Shanghai',
    periods: [{ start: '00:00', end: '24:00' }],
  });
});

test('the 32-range limit disables adding while keeping current ranges editable and removable', () => {
  const onPeriodsChange = vi.fn();
  const periods = Array.from({ length: 32 }, () => ({
    start: '09:00',
    end: '18:00',
  }));
  render(
    <ChannelScheduleEditor
      timezone='UTC'
      periods={periods}
      onTimezoneChange={vi.fn()}
      onPeriodsChange={onPeriodsChange}
    />,
  );
  expect(screen.getByRole('button', { name: 'Add time range' })).toBeDisabled();
  fireEvent.change(screen.getAllByLabelText('End time')[0], {
    target: { value: '19:00' },
  });
  expect(onPeriodsChange.mock.lastCall[0][0]).toEqual({
    start: '09:00',
    end: '19:00',
  });
  fireEvent.click(
    screen.getAllByRole('button', { name: 'Remove time range' })[0],
  );
  expect(onPeriodsChange).toHaveBeenLastCalledWith(periods.slice(1));
});

test.each([
  [1, false, 'Scheduled pause'],
  [1, true, '已启用'],
  [1, undefined, '已启用'],
  [2, false, '已禁用'],
  [3, false, '自动禁用'],
])(
  'status %s and schedule_active %s show %s without confusing manual or health disables',
  (status, active, expected) => {
    const columns = getChannelsColumns({ t: (key) => key, COLUMN_KEYS: {} });
    const column = columns.find((item) => item.dataIndex === 'status');
    render(
      column.render(status, {
        schedule_active: active,
        other_info: '{}',
        channel_info: {},
      }),
    );
    expect(screen.getByText(expected)).toBeVisible();
  },
);

test('a scheduled pause keeps the manual disable action tied to base status', () => {
  const manageChannel = vi.fn();
  const columns = getChannelsColumns({
    t: (key) => key,
    COLUMN_KEYS: {},
    manageChannel,
  });
  const record = { id: 123, status: 1, type: 43, schedule_active: false };
  const actions = columns[columns.length - 1];
  render(actions.render(null, record));
  fireEvent.click(screen.getByRole('button', { name: '禁用', exact: true }));
  expect(manageChannel).toHaveBeenCalledWith(123, 'disable', record);
  expect(
    screen.queryByRole('button', { name: '启用', exact: true }),
  ).not.toBeInTheDocument();
});
