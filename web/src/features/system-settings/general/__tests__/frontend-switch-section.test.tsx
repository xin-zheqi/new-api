/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { FrontendSwitchSection } from '../frontend-switch-section'

const { mutateMock } = vi.hoisted(() => ({
  mutateMock: vi.fn(),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({
    isPending: false,
    mutate: mutateMock,
  }),
}))

describe('frontend switch section', () => {
  test('asks for confirmation and submits the classic frontend option', () => {
    render(<FrontendSwitchSection />)

    fireEvent.click(
      screen.getByRole('button', { name: 'Switch to classic frontend' })
    )

    const dialog = screen.getByRole('alertdialog')
    expect(
      within(dialog).getByRole('heading', {
        name: 'Switch to classic frontend?',
      })
    ).toBeInTheDocument()

    fireEvent.click(
      within(dialog).getByRole('button', {
        name: 'Switch to classic frontend',
      })
    )

    expect(mutateMock).toHaveBeenCalledTimes(1)
    expect(mutateMock).toHaveBeenCalledWith(
      { key: 'theme.frontend', value: 'classic' },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  test('disables switching while the system information form has unsaved changes', () => {
    render(<FrontendSwitchSection disabled />)

    expect(
      screen.getByRole('button', { name: 'Switch to classic frontend' })
    ).toBeDisabled()
  })
})
