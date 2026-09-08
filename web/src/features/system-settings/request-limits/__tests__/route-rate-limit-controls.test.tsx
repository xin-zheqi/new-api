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

import { RateLimitSection } from '../rate-limit-section'

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({
    isPending: false,
    mutateAsync: vi.fn(),
  }),
}))

const defaultValues = {
  GlobalApiRateLimitEnabled: true,
  GlobalApiRateLimitCount: 360,
  GlobalApiRateLimitDuration: 180,
  GlobalWebRateLimitEnabled: true,
  GlobalWebRateLimitCount: 120,
  GlobalWebRateLimitDuration: 180,
  CriticalRateLimitEnabled: true,
  CriticalRateLimitCount: 20,
  CriticalRateLimitDuration: 1200,
  ModelRequestRateLimitEnabled: false,
  ModelRequestRateLimitDurationMinutes: 1,
  ModelRequestRateLimitCount: 0,
  ModelRequestRateLimitSuccessCount: 1000,
  ModelRequestRateLimitGroup: '{}',
}

describe('route rate limit controls', () => {
  test('shows the active critical limit and disables its inputs when switched off', () => {
    render(<RateLimitSection defaultValues={defaultValues} />)

    const enabledSwitch = screen.getByRole('switch', {
      name: 'Login and sensitive action rate limit',
    })
    const section = enabledSwitch.closest('section')
    expect(section).not.toBeNull()

    const controls = within(section as HTMLElement)
    expect(
      controls.getByRole('spinbutton', { name: 'Max requests per period' })
    ).toHaveValue(20)
    expect(
      controls.getByRole('spinbutton', { name: 'Limit period' })
    ).toHaveValue(1200)

    fireEvent.click(enabledSwitch)

    expect(
      controls.getByRole('spinbutton', { name: 'Max requests per period' })
    ).toBeDisabled()
    expect(
      controls.getByRole('spinbutton', { name: 'Limit period' })
    ).toBeDisabled()
  })
})
