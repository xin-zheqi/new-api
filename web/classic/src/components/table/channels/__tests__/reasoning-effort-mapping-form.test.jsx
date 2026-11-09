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
import ReasoningEffortMappingEditor from '../modals/ReasoningEffortMappingEditor';

// Artwork is unrelated to channel settings and loads browser-only dependencies.
vi.mock('@lobehub/icons', () => {
  const Icon = () => null;
  Icon.Color = Icon;
  Icon.Combine = Icon;
  const providers =
    'OpenAI Claude Gemini Moonshot Zhipu Qwen DeepSeek Minimax Wenxin Spark Midjourney Hunyuan Cohere Cloudflare Ai360 Yi Jina Mistral XAI Ollama Doubao Suno Xinference OpenRouter Dify Coze SiliconCloud FastGPT Kling Jimeng Perplexity Replicate';
  return Object.fromEntries(providers.split(' ').map((name) => [name, Icon]));
});

const existingSettings = {
  reasoning_content_repair: true,
  tool_output_repair: true,
  reject_empty_response: true,
  thinking_to_content: false,
};
const originalIntersectionObserver = globalThis.IntersectionObserver;

beforeAll(() =>
  i18next.use(initReactI18next).init({
    lng: 'zh-CN',
    resources: { 'zh-CN': { translation: {} } },
  }),
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

function renderChannelForm(mapping) {
  const baseGet = API.get.getMockImplementation();
  const channel = {
    id: 123,
    type: 43,
    name: 'DeepSeek fixture',
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
      reasoning_effort_mapping: mapping,
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

test('existing channel mappings load, allow adding and editing, and preserve other settings on save', async () => {
  renderChannelForm({ xhigh: 'max' });
  expect(await screen.findByLabelText('Before mapping')).toHaveValue('xhigh');
  expect(screen.getByLabelText('After mapping')).toHaveValue('max');
  fireEvent.change(screen.getByLabelText('After mapping'), {
    target: { value: 'high' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Add mapping' }));
  fireEvent.change(screen.getAllByLabelText('Before mapping')[1], {
    target: { value: ' low ' },
  });
  fireEvent.change(screen.getAllByLabelText('After mapping')[1], {
    target: { value: ' medium ' },
  });
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.put).toHaveBeenCalled());
  const [url, payload] = API.put.mock.calls[0];
  expect(url).toBe('/api/channel/');
  expect(JSON.parse(payload.setting)).toMatchObject({
    ...existingSettings,
    reasoning_effort_mapping: { xhigh: 'high', low: 'medium' },
  });
  expect(payload).not.toHaveProperty('reasoning_effort_mapping');
});

test('removing the last mapping omits it while preserving existing compatibility settings', async () => {
  renderChannelForm({ xhigh: 'max' });
  fireEvent.click(
    await screen.findByRole('button', { name: 'Remove mapping' }),
  );
  expect(screen.queryByLabelText('Before mapping')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.put).toHaveBeenCalled());
  const settings = JSON.parse(API.put.mock.calls[0][1].setting);
  expect(settings).not.toHaveProperty('reasoning_effort_mapping');
  expect(settings).toMatchObject(existingSettings);
});

test('a legacy channel without mappings saves with the mapping field absent', async () => {
  renderChannelForm(undefined);
  await screen.findByRole('button', { name: 'Add mapping' });
  expect(screen.queryByLabelText('Before mapping')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.put).toHaveBeenCalled());
  expect(JSON.parse(API.put.mock.calls[0][1].setting)).not.toHaveProperty(
    'reasoning_effort_mapping',
  );
});

test('a new channel can add a mapping through advanced settings and save it', async () => {
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
    target: { value: 'Mapped effort fixture' },
  });
  fireEvent.change(screen.getByLabelText('密钥'), {
    target: { value: 'sk-fixture' },
  });
  fireEvent.change(screen.getByLabelText('自定义模型名称'), {
    target: { value: 'gpt-4o-mini' },
  });
  fireEvent.click(screen.getByRole('button', { name: '填入', exact: true }));
  fireEvent.click(screen.getByText('高级设置'));
  fireEvent.click(await screen.findByRole('button', { name: 'Add mapping' }));
  fireEvent.change(screen.getByLabelText('Before mapping'), {
    target: { value: 'xhigh' },
  });
  fireEvent.change(screen.getByLabelText('After mapping'), {
    target: { value: 'max' },
  });
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  await waitFor(() => expect(API.post).toHaveBeenCalled());
  const [url, payload] = API.post.mock.calls[0];
  expect(url).toBe('/api/channel/');
  expect(JSON.parse(payload.channel.setting)).toMatchObject({
    reasoning_effort_mapping: { xhigh: 'max' },
  });
  expect(payload.channel).not.toHaveProperty('reasoning_effort_mapping');
});

test('incomplete rows display validation and never write the channel', async () => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  renderChannelForm({ xhigh: 'max' });
  fireEvent.change(await screen.findByLabelText('After mapping'), {
    target: { value: '' },
  });
  fireEvent.click(screen.getByRole('button', { name: /提交/ }));
  expect(
    await screen.findByText(/Reasoning effort mappings require both values\./),
  ).toBeVisible();
  expect(API.put).not.toHaveBeenCalled();
});

test('the add control is disabled at 64 mappings while existing rows stay editable', () => {
  const onChange = vi.fn();
  const rows = Array.from({ length: 64 }, (_, index) => ({
    from: `effort-${index}`,
    to: 'max',
  }));
  render(<ReasoningEffortMappingEditor value={rows} onChange={onChange} />);
  expect(screen.getByRole('button', { name: 'Add mapping' })).toBeDisabled();
  fireEvent.change(screen.getAllByLabelText('After mapping')[0], {
    target: { value: 'high' },
  });
  expect(onChange).toHaveBeenLastCalledWith([
    { from: 'effort-0', to: 'high' },
    ...rows.slice(1),
  ]);
  fireEvent.click(screen.getAllByRole('button', { name: 'Remove mapping' })[0]);
  expect(onChange).toHaveBeenLastCalledWith(rows.slice(1));
});
