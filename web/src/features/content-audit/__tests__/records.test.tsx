/*
Copyright (C) 2023-2026 QuantumNous

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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { AuditRecords } from '../records'

describe('private audit record browser', () => {
  test('loads private details only on demand and confirms deletion with concise copy', async () => {
    const network = vi
      .spyOn(api, 'request')
      .mockImplementation(async (config) => {
        if (config.method === 'DELETE') {
          return { data: { success: true, data: null } }
        }
        if (config.url === '/api/content-audit/records/record-1') {
          return {
            data: {
              success: true,
              data: {
                action: 'warn',
                count: 1,
                truncated: false,
                matches: [
                  {
                    number: 1,
                    rule_id: 'rule-1',
                    rule_name: 'Email rule',
                    path: 'input',
                    action: 'warn',
                    original: 'synthetic@example.invalid',
                    applied: false,
                    truncated: false,
                  },
                ],
              },
            },
          }
        }
        return {
          data: {
            success: true,
            data: {
              items: [
                {
                  id: 'record-1',
                  created_at: 100,
                  token_name: 'Test agent',
                  model: 'model',
                  endpoint: '/v1/responses',
                  request_id: 'request-1',
                  action: 'warn',
                  match_count: 1,
                },
              ],
              total: 1,
            },
          },
        }
      })
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const view = render(
      <QueryClientProvider client={client}>
        <AuditRecords />
      </QueryClientProvider>
    )
    expect(await screen.findByText('Test agent')).toBeVisible()
    expect(
      screen.queryByText('synthetic@example.invalid')
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^Details$/ }))
    expect(await screen.findByText('synthetic@example.invalid')).toBeVisible()
    fireEvent.keyDown(screen.getByRole('dialog'), {
      key: 'Escape',
      code: 'Escape',
    })
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete all my records' })
    )
    const confirmation = await screen.findByRole('alertdialog')
    expect(
      within(confirmation).getByText('Delete', {
        selector: '[data-slot="alert-dialog-title"]',
      })
    ).toBeVisible()
    expect(confirmation).not.toHaveTextContent('Backups')
    fireEvent.click(
      within(confirmation).getByRole('button', { name: /^Delete$/ })
    )
    await waitFor(() =>
      expect(network).toHaveBeenCalledWith(
        expect.objectContaining({
          method: 'DELETE',
          url: '/api/content-audit/records',
        })
      )
    )
    view.unmount()
    client.clear()
  })
  test('submits keyword, action and exact selected timestamps to the server', async () => {
    const network = vi.spyOn(api, 'request').mockResolvedValue({
      data: { success: true, data: { items: [], total: 0 } },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const view = render(
      <QueryClientProvider client={client}>
        <AuditRecords />
      </QueryClientProvider>
    )
    expect(await screen.findByText('No audit records')).toBeVisible()
    fireEvent.change(screen.getByLabelText('Request ID, key name or model'), {
      target: { value: 'Agent' },
    })
    fireEvent.change(screen.getByLabelText('Action'), {
      target: { value: 'block' },
    })
    fireEvent.change(screen.getByLabelText('Start Time'), {
      target: { value: '2026-09-01T00:00' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() =>
      expect(network).toHaveBeenCalledWith(
        expect.objectContaining({
          url: expect.stringContaining('action=block'),
        })
      )
    )
    const request = network.mock.calls.find(([config]) =>
      config.url?.includes('action=block')
    )?.[0]
    const params = new URL(request?.url ?? '', 'http://test').searchParams
    expect(params.get('keyword')).toBe('Agent')
    expect(params.get('start_timestamp')).toBe(
      String(new Date('2026-09-01T00:00').getTime() / 1000)
    )
    expect(params.get('page')).toBe('1')
    view.unmount()
    client.clear()
  })
})
