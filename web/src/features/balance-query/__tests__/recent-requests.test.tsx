/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { BalanceQuery } from '../index'

vi.mock('@/components/layout', () => ({
  PublicLayout: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}))
vi.mock('@/stores/system-config-store', () => ({
  useSystemConfigStore: (selector: (state: unknown) => unknown) =>
    selector({
      config: {
        currency: {
          quotaPerUnit: 500000,
          quotaDisplayType: 'USD',
          usdExchangeRate: 1,
          customCurrencySymbol: '¤',
          customCurrencyExchangeRate: 1,
        },
      },
    }),
}))

function request(index: number) {
  return {
    request_id: `request-${index}`,
    created_at: 1760000000 + index,
    model_name: `model-${index}`,
    prompt_tokens: 120,
    completion_tokens: 30,
    cache_read_tokens: 40,
    cache_write_tokens: 0,
    cache_write_5m_tokens: 0,
    cache_write_1h_tokens: 0,
    text_input_tokens: 0,
    text_output_tokens: 0,
    audio_input_tokens: 0,
    audio_output_tokens: 0,
    quota: 1000,
  }
}

function mockUsage(
  recentRequests: ReturnType<typeof request>[],
  error = false
) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        code: true,
        data: {
          name: 'Test key',
          total_granted: 10000,
          total_used: 1000,
          total_available: 9000,
          recent_requests: recentRequests,
          recent_requests_error: error,
        },
      }),
    })
  )
}

async function queryKey() {
  fireEvent.change(screen.getByLabelText('API Key'), {
    target: { value: 'sk-test' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Query' }))
  return screen.findByText('Recent requests')
}

afterEach(() => vi.unstubAllGlobals())

describe('public balance request history', () => {
  test('shows at most 20 billed requests with token breakdown and actual charge', async () => {
    mockUsage(Array.from({ length: 21 }, (_, index) => request(index)))
    render(<BalanceQuery />)

    await queryKey()
    expect(screen.getByText('model-0')).toBeVisible()
    expect(screen.getByText('model-19')).toBeVisible()
    expect(screen.queryByText('model-20')).not.toBeInTheDocument()
    const first = screen.getByText('model-0').closest('li')
    if (!first) throw new Error('First request row was not rendered')
    expect(within(first).getByText('$0.0020')).toBeVisible()
    expect(within(first).getByText('40')).toBeVisible()
    expect(within(first).getByText('120')).toBeVisible()
    expect(screen.queryByText('Private balance lookup')).not.toBeInTheDocument()
    expect(
      screen.queryByText(
        'Check your API key balance, usage, and periodic quota in one place.'
      )
    ).not.toBeInTheDocument()
  })

  test('shows a separate empty or unavailable state after a successful balance query', async () => {
    mockUsage([])
    const view = render(<BalanceQuery />)
    await queryKey()
    expect(screen.getByRole('status')).toHaveTextContent('No recent requests')

    view.unmount()
    mockUsage([], true)
    render(<BalanceQuery />)
    await queryKey()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Request history is temporarily unavailable'
    )
  })

  test('shows cache write totals with optional breakdown and hides absent token counts', async () => {
    const withCache = {
      ...request(1),
      cache_write_tokens: 7,
      cache_write_5m_tokens: 2,
      cache_write_1h_tokens: 5,
    }
    const withoutTokens = {
      ...request(2),
      prompt_tokens: 0,
      completion_tokens: 0,
      cache_read_tokens: 0,
    }
    mockUsage([withCache, withoutTokens])
    render(<BalanceQuery />)

    await queryKey()
    const cacheRow = screen.getByText('model-1').closest('li')
    const emptyRow = screen.getByText('model-2').closest('li')
    if (!cacheRow || !emptyRow) throw new Error('Request rows not rendered')
    expect(within(cacheRow).getByText(/Cache write:/)).toHaveTextContent('7')
    expect(within(cacheRow).getByText(/Cache write 5m:/)).toHaveTextContent('2')
    expect(within(cacheRow).getByText(/Cache write 1h:/)).toHaveTextContent('5')
    expect(
      within(emptyRow).queryByText(/Input tokens:/)
    ).not.toBeInTheDocument()
    expect(
      within(emptyRow).queryByText(/Total tokens:/)
    ).not.toBeInTheDocument()
  })

  test('does not show an earlier key history after the entered key changes', async () => {
    let resolveFetch: ((value: unknown) => void) | undefined
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            resolveFetch = resolve
          })
      )
    )
    render(<BalanceQuery />)
    fireEvent.change(screen.getByLabelText('API Key'), {
      target: { value: 'sk-first' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Query' }))
    fireEvent.change(screen.getByLabelText('API Key'), {
      target: { value: 'sk-second' },
    })

    await act(async () => {
      resolveFetch?.({
        ok: true,
        json: async () => ({
          code: true,
          data: { name: 'First key', recent_requests: [request(1)] },
        }),
      })
    })
    expect(screen.queryByText('First key')).not.toBeInTheDocument()
    expect(screen.queryByText('model-1')).not.toBeInTheDocument()
  })
})
