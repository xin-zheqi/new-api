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
import { fireEvent, render, screen } from '@testing-library/react'
import * as React from 'react'
import { describe, expect, test } from 'vitest'

import { PaymentMethodSettingsVisualEditor } from '../payment-method-settings-visual-editor'

function PaymentMethodEditorHarness({ initialValue = '[]' }) {
  const [value, setValue] = React.useState(initialValue)

  return (
    <>
      <PaymentMethodSettingsVisualEditor
        value={value}
        epayMethodsValue='[{"name":"Bank transfer","type":"bank_transfer"}]'
        onChange={setValue}
      />
      <output aria-label='Serialized payment method settings'>{value}</output>
    </>
  )
}

function readSerializedSettings() {
  return JSON.parse(
    screen.getByLabelText('Serialized payment method settings').textContent ||
      '[]'
  )
}

describe('payment method settings editor', () => {
  test('controls built-in and custom gateways through one editor', () => {
    render(<PaymentMethodEditorHarness />)

    expect(screen.getByText('bank_transfer')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('switch', { name: 'Enable Stripe' }))
    expect(readSerializedSettings()).toEqual([
      { type: 'stripe', name: 'Stripe', enabled: false },
    ])
    expect(screen.getByText('Disabled')).toBeInTheDocument()

    const stripeName = screen.getByRole('textbox', {
      name: 'Display name for this payment method. Stripe',
    })
    fireEvent.change(stripeName, { target: { value: 'Company card' } })
    expect(readSerializedSettings()).toEqual([
      { type: 'stripe', name: 'Company card', enabled: false },
    ])

    fireEvent.click(screen.getByRole('switch', { name: 'Enable Company card' }))
    expect(readSerializedSettings()).toEqual([
      { type: 'stripe', name: 'Company card', enabled: true },
    ])
    expect(screen.queryByText('Disabled')).not.toBeInTheDocument()
  })

  test('clearing a custom name restores the translated default', () => {
    render(
      <PaymentMethodEditorHarness initialValue='[{"type":"stripe","name":"Company card","enabled":true}]' />
    )

    fireEvent.change(
      screen.getByRole('textbox', {
        name: 'Display name for this payment method. Company card',
      }),
      { target: { value: '' } }
    )

    expect(readSerializedSettings()).toEqual([])
  })
})
