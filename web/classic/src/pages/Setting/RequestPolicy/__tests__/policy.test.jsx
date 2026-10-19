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
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll, expect, test, vi } from 'vitest';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import PolicyFields from '../PolicyFields';

beforeAll(() =>
  i18next
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } }),
);
afterEach(cleanup);

test('changing recovery mode atomically saves the legacy alias and keeps unrelated options untouched', async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  render(
    <PolicyFields
      options={{
        'monitor_setting.channel_test_mode': 'scheduled_all',
        RetryTimes: '2',
      }}
      fields={[
        [
          'monitor_setting.channel_test_mode',
          'Channel test mode',
          'select',
          [
            ['scheduled_all', 'Scheduled testing'],
            ['passive_recovery', 'Passive recovery'],
          ],
        ],
      ]}
      onSave={save}
    />,
  );
  fireEvent.change(screen.getByLabelText('Channel test mode'), {
    target: { value: 'passive_recovery' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith({
      'monitor_setting.channel_test_mode': 'passive_recovery',
      'monitor_setting.auto_test_only_auto_disabled': 'true',
    }),
  );
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled(),
  );
});

test('a failed save leaves the edited retry value available for correction and retry', async () => {
  const save = vi.fn().mockRejectedValue(new Error('Policy rejected'));
  render(
    <PolicyFields
      options={{ RetryTimes: '2' }}
      fields={[['RetryTimes', 'Retry times', 'number']]}
      onSave={save}
    />,
  );
  fireEvent.change(screen.getByLabelText('Retry times'), {
    target: { value: '3' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Policy rejected');
  expect(screen.getByLabelText('Retry times')).toHaveValue(3);
  expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled();
});
