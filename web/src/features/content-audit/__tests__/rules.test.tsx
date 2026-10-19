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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { AuditMatches } from '../matches'
import { AuditRules } from '../rules'
import { normalizeAuditPolicy } from '../types'

const data = {
  policy: { enabled: false, rules: [] },
  retention_days: 7,
  max_rules: 20,
}

function rules(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    id: `rule-${index}`,
    name: `Rule ${index + 1}`,
    enabled: true,
    pattern: 'secret',
    flags: '',
    mode: 'first' as const,
    action: 'warn' as const,
    replacement: '',
  }))
}

describe('private audit rules', () => {
  test('shows the full-width custom rule button at the bottom and disables it at twenty rules', () => {
    const client = new QueryClient()
    const view = render(
      <QueryClientProvider client={client}>
        <AuditRules
          data={{ ...data, policy: { enabled: false, rules: rules(20) } }}
        />
      </QueryClientProvider>
    )

    expect(screen.getByRole('button', { name: 'Add rule' })).toBeDisabled()
    view.unmount()
    client.clear()
  })

  test('preset starts as warning, replacement enables its input and saving preserves it across tab remounts', async () => {
    const network = vi
      .spyOn(api, 'request')
      .mockResolvedValue({ data: { success: true, data: null } })
    const client = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    })
    const view = render(
      <QueryClientProvider client={client}>
        <AuditRules data={data} />
      </QueryClientProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Email address' }))
    expect(screen.getByLabelText('Rule name')).toHaveValue('Email address')
    expect(screen.getByLabelText('Replacement')).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Action'), {
      target: { value: 'replace' },
    })
    expect(screen.getByLabelText('Replacement')).toBeEnabled()
    expect(screen.getByLabelText('Match mode')).toBeVisible()
    fireEvent.click(
      screen.getByRole('switch', { name: 'Enable my content audit' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save rules' }))
    await waitFor(() => expect(network).toHaveBeenCalled())
    expect(network.mock.calls[0][0]).toMatchObject({
      method: 'PUT',
      data: {
        enabled: true,
        rules: [
          { name: 'Email address', action: 'replace', replacement: '[EMAIL]' },
        ],
      },
    })
    await waitFor(() =>
      expect(
        client.getQueryData(['content-audit-policy', undefined])
      ).toMatchObject({ policy: { enabled: true } })
    )
    view.unmount()
    client.clear()
  })
  test('warning and block actions always normalize to first match', () => {
    const normalized = normalizeAuditPolicy({
      enabled: true,
      rules: [
        {
          id: 'warn',
          name: 'Warn',
          enabled: true,
          pattern: 'secret',
          flags: 'miim',
          mode: 'all',
          action: 'warn',
          replacement: '',
        },
        {
          id: 'block',
          name: 'Block',
          enabled: true,
          pattern: 'secret',
          flags: 's',
          mode: 'all',
          action: 'block',
          replacement: '',
        },
      ],
    })
    expect(normalized.rules.map((rule) => rule.mode)).toEqual([
      'first',
      'first',
    ])
    expect(normalized.rules[0].flags).toBe('im')
  })
  test('multiple matches render escaped content, overlaps and truncation notices', () => {
    const original = '<img src=x onerror=alert(1)>'
    render(
      <AuditMatches
        result={{
          action: 'replace',
          count: 49,
          truncated: true,
          matches: [
            {
              number: 1,
              rule_id: 'r1',
              rule_name: 'First',
              path: 'input',
              original,
              replacement: '[safe]',
              action: 'replace',
              applied: true,
              truncated: false,
            },
            {
              number: 2,
              rule_id: 'r2',
              rule_name: 'Second',
              path: 'input',
              original: 'secret',
              action: 'replace',
              applied: false,
              truncated: true,
              overlap: true,
            },
          ],
        }}
      />
    )
    expect(screen.getByText(original)).toBeVisible()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.getByText(/Only the first 48/)).toBeVisible()
    expect(screen.getByText(/Overlapping replacement skipped/)).toBeVisible()
  })
})
