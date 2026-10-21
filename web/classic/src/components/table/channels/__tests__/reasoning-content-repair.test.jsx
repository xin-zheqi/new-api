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

// Provider artwork is unrelated to channel settings; avoid loading its emoji
// data and browser-only UI dependencies in this form regression test.
vi.mock('@lobehub/icons', () => {
  const Icon = () => null;
  Icon.Color = Icon;
  Icon.Combine = Icon;
  const providers =
    'OpenAI Claude Gemini Moonshot Zhipu Qwen DeepSeek Minimax Wenxin Spark Midjourney Hunyuan Cohere Cloudflare Ai360 Yi Jina Mistral XAI Ollama Doubao Suno Xinference OpenRouter Dify Coze SiliconCloud FastGPT Kling Jimeng Perplexity Replicate';
  return Object.fromEntries(providers.split(' ').map((name) => [name, Icon]));
});

beforeAll(() =>
  i18next.use(initReactI18next).init({
    lng: 'zh-CN',
    resources: { 'zh-CN': { translation: {} } },
  }),
);
const originalIntersectionObserver = globalThis.IntersectionObserver;
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
  vi.spyOn(API, 'post').mockResolvedValue({
    data: { success: true, data: ['gpt-4o-mini'] },
  });
});
afterEach(() => {
  cleanup();
  globalThis.IntersectionObserver = originalIntersectionObserver;
});

test.each([undefined, false, true])(
  'reasoning repair loads %s, toggles and saves without changing existing settings',
  async (enabled) => {
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
        reasoning_content_repair: enabled,
        reject_empty_response: true,
        thinking_to_content: false,
      }),
    };
    API.get.mockImplementation(async (url, ...args) =>
      url === '/api/channel/123'
        ? { data: { success: true, data: structuredClone(channel) } }
        : baseGet(url, ...args),
    );
    vi.spyOn(API, 'put').mockResolvedValue({ data: { success: true } });
    render(
      <UserContext.Provider value={[{ user: { role: 100 } }, vi.fn()]}>
        <EditChannelModal
          visible
          editingChannel={{ id: 123 }}
          handleClose={vi.fn()}
          refresh={vi.fn()}
        />
      </UserContext.Provider>,
    );
    const toggle = await screen.findByRole('switch', {
      name: 'Restore reasoning content',
    });
    expect(toggle).toHaveAttribute('aria-checked', String(enabled === true));
    expect(toggle.parentElement).toHaveTextContent(
      enabled === true ? '开' : '关',
    );
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-checked', String(enabled !== true));
    expect(toggle.parentElement).toHaveTextContent(
      enabled !== true ? '开' : '关',
    );
    fireEvent.click(screen.getByRole('button', { name: /提交/ }));
    await waitFor(() => expect(API.put).toHaveBeenCalled());
    const [url, payload] = API.put.mock.calls[0];
    expect(url).toBe('/api/channel/');
    expect(JSON.parse(payload.setting)).toMatchObject({
      reasoning_content_repair: enabled !== true,
      reject_empty_response: true,
      thinking_to_content: false,
    });
    expect(payload).not.toHaveProperty('reasoning_content_repair');
  },
);
