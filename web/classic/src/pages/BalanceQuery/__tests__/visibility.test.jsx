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
import { fireEvent, render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import ApiKeyInput from '../ApiKeyInput';

test('password mode shows one visibility control that reveals the API key', () => {
  render(
    <ApiKeyInput
      value='sk-test123'
      onChange={() => {}}
      onEnterPress={() => {}}
    />,
  );

  const keyInput = screen.getByPlaceholderText('sk-...');
  const visibilityButtons = screen.getAllByRole('button', {
    name: /show password|show api key/i,
  });
  expect(visibilityButtons).toHaveLength(1);
  expect(keyInput.getAttribute('type')).toBe('password');

  fireEvent.click(visibilityButtons[0]);
  expect(keyInput.getAttribute('type')).toBe('text');
});
