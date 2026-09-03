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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'

import { ManualTopupDialog } from '../manual-topup-dialog'

type ApiMethod = (...args: unknown[]) => Promise<{ data: unknown }>
type MockableApi = {
  get: ApiMethod
  post: ApiMethod
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPost = apiClient.post
const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (originalGetAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalGetAnimations
    )
    return
  }
  Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
})

afterEach(() => {
  apiClient.get = originalGet
  apiClient.post = originalPost
})

describe('ManualTopupDialog', () => {
  test('submits payment currency and independent invoice details', async () => {
    const user = userEvent.setup()
    const requests: Array<{ url: string; data: Record<string, unknown> }> = []
    apiClient.get = async () => ({
      data: {
        success: true,
        data: {
          items: [{ id: 7, username: 'billing-user' }],
        },
      },
    })
    apiClient.post = async (url, data) => {
      requests.push({ url: String(url), data: data as Record<string, unknown> })
      return { data: { success: true } }
    }

    render(
      <ManualTopupDialog
        open
        onOpenChange={() => undefined}
        onCreated={() => undefined}
      />
    )

    const invoiceAmountField = screen
      .getByLabelText('Invoiceable amount')
      .closest('[data-slot="form-item"]')
    const invoiceCurrencyField = screen
      .getByLabelText('Invoice currency')
      .closest('[data-slot="form-item"]')
    expect(invoiceAmountField?.parentElement).toBe(
      invoiceCurrencyField?.parentElement
    )
    expect(invoiceAmountField?.parentElement).toHaveClass('items-start')

    await user.click(screen.getByRole('combobox', { name: 'User' }))
    await user.click(
      await screen.findByRole('option', { name: /billing-user/ })
    )
    fireEvent.change(screen.getByLabelText('Payment Method'), {
      target: { value: 'wire_transfer' },
    })
    fireEvent.change(screen.getByLabelText('Recharge Amount'), {
      target: { value: '2' },
    })
    fireEvent.change(screen.getByLabelText('Payment Amount'), {
      target: { value: '3.125' },
    })
    fireEvent.change(screen.getByLabelText('Payment currency'), {
      target: { value: 'kwd' },
    })
    expect(screen.getByLabelText('Payment currency')).toHaveValue('KWD')
    fireEvent.change(screen.getByLabelText('Invoiceable amount'), {
      target: { value: '2.625' },
    })
    fireEvent.change(screen.getByLabelText('Invoice currency'), {
      target: { value: 'kwd' },
    })
    expect(screen.getByLabelText('Invoice currency')).toHaveValue('KWD')
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      url: '/api/user/topup/manual',
      data: {
        user_id: 7,
        payment_method: 'wire_transfer',
        amount: 2,
        money: 3.125,
        currency: 'KWD',
        invoice_amount: 2.625,
        invoice_currency: 'KWD',
        create_time: expect.any(Number),
        credit_balance: false,
      },
    })
  })
})
