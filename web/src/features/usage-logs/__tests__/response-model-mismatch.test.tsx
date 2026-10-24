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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { ResponseModelMismatch } from '../components/response-model-mismatch'

const mismatch = {
  requested_model: 'public-model',
  response_models: ['provider-v2', '<img src=x onerror=alert(1)>'],
  truncated: true,
}

test('ordinary users never receive an upstream model indicator even with admin metadata', () => {
  const { container } = render(
    <ResponseModelMismatch isAdmin={false} mismatch={mismatch} />
  )
  expect(container).toBeEmptyDOMElement()
})

test('old logs and matching responses do not add a model indicator', () => {
  const { container } = render(<ResponseModelMismatch isAdmin />)
  expect(container).toBeEmptyDOMElement()
})

test('admins can open mismatch details with the keyboard and model values remain text', async () => {
  const user = userEvent.setup()
  render(<ResponseModelMismatch isAdmin mismatch={mismatch} />)
  await user.tab()
  expect(
    screen.getByRole('button', { name: 'Response model mismatch' })
  ).toHaveFocus()
  await user.keyboard('{Enter}')
  expect(await screen.findByText('public-model')).toBeVisible()
  expect(screen.getByText('provider-v2')).toBeVisible()
  expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeVisible()
  expect(document.querySelector('img')).toBeNull()
  expect(screen.getByText('Additional model names were omitted.')).toBeVisible()
})

test('expanded log details show the full long model identifier without a nested popover', () => {
  const longModel = 'provider-' + 'long-version-'.repeat(15)
  render(
    <ResponseModelMismatch
      isAdmin
      expanded
      mismatch={{ ...mismatch, response_models: [longModel] }}
    />
  )
  expect(screen.getByText(longModel)).toBeVisible()
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})
