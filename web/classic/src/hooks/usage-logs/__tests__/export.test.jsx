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

import { act, cleanup, renderHook } from '@testing-library/react';
import { Toast } from '@douyinfe/semi-ui';
import { ReadableStream } from 'node:stream/web';
import { TextEncoder } from 'node:util';
import { afterEach, beforeAll, beforeEach, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { API } from '../../../helpers/api';
import { useUsageLogsExport } from '../useUsageLogsExport';

beforeAll(() =>
  i18next.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  }),
);

const originalCreateObjectURL = URL.createObjectURL;
const originalRevokeObjectURL = URL.revokeObjectURL;

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  localStorage.clear();
  vi.spyOn(Toast, 'error').mockImplementation(() => {});
  vi.spyOn(Toast, 'info').mockImplementation(() => {});
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  URL.createObjectURL = vi.fn(() => 'blob:usage-log-test');
  URL.revokeObjectURL = vi.fn();
});
afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.clearAllTimers();
  vi.useRealTimers();
  URL.createObjectURL = originalCreateObjectURL;
  URL.revokeObjectURL = originalRevokeObjectURL;
  vi.restoreAllMocks();
});

const log = {
  id: 1,
  created_at: 1600000000,
  username: 'sample-user',
  token_name: 'sample-token',
  model_name: 'sample-model',
  prompt_tokens: 123,
  completion_tokens: 5,
  quota: 500000,
  cache_read_tokens: 20,
  cache_write_tokens: 50,
  is_stream: false,
  request_path: '/v1/chat/completions',
  first_response_time_ms: 145.5,
  use_time: 8,
};
const appliedQuery = (path = '/api/log/self/') => ({
  current: {
    path,
    params: new URLSearchParams({
      type: '2',
      token_name: 'token & one',
      model_name: 'sample-model',
      group: 'group+A',
      request_id: 'req/one',
      start_timestamp: '1',
      end_timestamp: '4102444800',
      p: '5',
      page_size: '10',
    }),
  },
});
const response = (rows, count = rows.length) => ({
  data: new ReadableStream({
    start(controller) {
      const encoder = new TextEncoder();
      for (const row of rows) {
        controller.enqueue(encoder.encode(`${JSON.stringify(row)}\n`));
      }
      if (count !== null) {
        controller.enqueue(
          encoder.encode(`${JSON.stringify({ complete: true, count })}\n`),
        );
      }
      controller.close();
    },
  }),
});
const readBlob = (blob) =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = reject;
    reader.readAsText(blob);
  });

test('self export uses one stream for all filtered rows and captures query and currency once', async () => {
  localStorage.setItem('quota_per_unit', '500000');
  localStorage.setItem('quota_display_type', 'CNY');
  localStorage.setItem('status', '{"usd_exchange_rate":7}');
  const query = appliedQuery();
  const requests = [];
  vi.spyOn(API, 'get').mockImplementation(async (url, options) => {
    requests.push({ url: new URL(url, 'https://test.invalid'), options });
    // Changes after the click must not alter this file's query or amounts.
    query.current.params.set('token_name', 'changed');
    localStorage.setItem('status', '{"usd_exchange_rate":9}');
    return response(Array.from({ length: 101 }, () => log));
  });
  const { result } = renderHook(() => useUsageLogsExport(query));
  await act(() => result.current.exportLogs());

  expect(requests).toHaveLength(1);
  for (const request of requests) {
    expect(request.url.pathname).toBe('/api/log/self/export');
    expect(request.url.searchParams.get('token_name')).toBe('token & one');
    expect(request.url.searchParams.get('group')).toBe('group+A');
    expect(request.url.searchParams.get('request_id')).toBe('req/one');
    expect(request.url.searchParams.get('type')).toBe('2');
    expect(request.url.searchParams.has('page_size')).toBe(false);
    expect(request.url.searchParams.has('p')).toBe(false);
    expect(Number(request.url.searchParams.get('end_timestamp'))).toBeLessThan(
      4102444800,
    );
    expect(request.options.disableDuplicate).toBe(true);
    expect(request.options.signal.aborted).toBe(false);
    expect(request.options.adapter).toBe('fetch');
    expect(request.options.responseType).toBe('stream');
  }
  expect(URL.createObjectURL).toHaveBeenCalledTimes(1);
  const csv = await readBlob(URL.createObjectURL.mock.calls[0][0]);
  expect(csv).toContain('"Amount (CNY)"');
  expect(csv.split('\r\n')[0]).toBe(
    '"Username","Token Name","Model Name","Request type","Request interface","Input tokens","Output tokens","Cache read tokens","Cache write tokens","Amount (CNY)","Time","Time to first token (s)","Total request time (s)"',
  );
  expect(csv).toContain(
    '"sample-user","sample-token","sample-model","Non-streaming","chat",123,5,20,50,7,',
  );
  expect(csv.split('\r\n')[1]).toMatch(/,0\.1455,8$/);
  expect(csv.trim().split('\r\n')).toHaveLength(102);
  expect(result.current.exportingLogs).toBe(false);
});

test('admin export preserves username and channel scope from the applied query', async () => {
  const query = appliedQuery('/api/log/');
  query.current.params.set('username', 'admin-filter');
  query.current.params.set('channel', '7');
  const get = vi.spyOn(API, 'get').mockResolvedValue(response([log]));
  const { result } = renderHook(() => useUsageLogsExport(query));
  await act(() => result.current.exportLogs());
  const url = new URL(get.mock.calls[0][0], 'https://test.invalid');
  expect(url.pathname).toBe('/api/log/export');
  expect(url.searchParams.get('username')).toBe('admin-filter');
  expect(url.searchParams.get('channel')).toBe('7');
  expect(URL.createObjectURL).toHaveBeenCalledTimes(1);
});

test('a truncated stream reports an error and does not download a partial file', async () => {
  vi.spyOn(API, 'get').mockResolvedValue(response([log], null));
  const { result } = renderHook(() => useUsageLogsExport(appliedQuery()));
  await act(() => result.current.exportLogs());
  expect(URL.createObjectURL).not.toHaveBeenCalled();
  expect(Toast.error).toHaveBeenCalledWith('错误：Failed to export logs');
  expect(result.current.exportingLogs).toBe(false);
});

test('an empty filtered export shows an empty notice without downloading', async () => {
  const query = appliedQuery();
  query.current.params.set('type', '5');
  const get = vi.spyOn(API, 'get').mockResolvedValue(response([]));
  const { result } = renderHook(() => useUsageLogsExport(query));
  await act(() => result.current.exportLogs());
  expect(URL.createObjectURL).not.toHaveBeenCalled();
  expect(Toast.info).toHaveBeenCalledWith('No logs to export');
  expect(
    new URL(get.mock.calls[0][0], 'https://test.invalid').searchParams.get(
      'type',
    ),
  ).toBe('5');
});

test('leaving the page cancels the active request and suppresses download and error notices', async () => {
  let resolveRequest;
  const pending = new Promise((resolve) => {
    resolveRequest = resolve;
  });
  const get = vi.spyOn(API, 'get').mockReturnValue(pending);
  const { result, unmount } = renderHook(() =>
    useUsageLogsExport(appliedQuery()),
  );
  let exporting;
  act(() => {
    exporting = result.current.exportLogs();
  });
  expect(result.current.exportingLogs).toBe(true);
  unmount();
  expect(get.mock.calls[0][1].signal.aborted).toBe(true);
  resolveRequest(response([log]));
  await exporting;
  expect(URL.createObjectURL).not.toHaveBeenCalled();
  expect(Toast.error).not.toHaveBeenCalled();
});

test('a failed request restores the export control without creating a file', async () => {
  vi.spyOn(API, 'get').mockRejectedValue(new Error('network unavailable'));
  const { result } = renderHook(() => useUsageLogsExport(appliedQuery()));
  await act(() => result.current.exportLogs());
  expect(URL.createObjectURL).not.toHaveBeenCalled();
  expect(Toast.error).toHaveBeenCalledWith('错误：Failed to export logs');
  expect(result.current.exportingLogs).toBe(false);
});

test('a successful HTTP response without a readable export is rejected', async () => {
  vi.spyOn(API, 'get').mockResolvedValue({ data: { success: true, data: [] } });
  const { result } = renderHook(() => useUsageLogsExport(appliedQuery()));
  await act(() => result.current.exportLogs());
  expect(URL.createObjectURL).not.toHaveBeenCalled();
  expect(Toast.error).toHaveBeenCalledWith('错误：Failed to export logs');
  expect(result.current.exportingLogs).toBe(false);
});
