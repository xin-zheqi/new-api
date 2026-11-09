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
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import '@testing-library/jest-dom/vitest';
import { afterEach, expect, test, vi } from 'vitest';
import LogsFilters from '../UsageLogsFilters';

afterEach(cleanup);

const props = () => ({
  formInitValues: { logType: '0' },
  setFormApi: vi.fn(),
  refresh: vi.fn(),
  setShowColumnSelector: vi.fn(),
  setLogType: vi.fn(),
  loading: false,
  isAdminUser: false,
  t: (key) => key,
  exportLogs: vi.fn(),
  exportingLogs: false,
  canExportLogs: true,
});

test('export appears after column settings and starts a download without submitting the search', async () => {
  const values = props();
  render(<LogsFilters {...values} />);
  const buttons = screen.getAllByRole('button');
  const columns = screen.getByRole('button', { name: '列设置' });
  const exportButton = screen.getByRole('button', { name: 'Export table' });
  expect(buttons.indexOf(exportButton)).toBe(buttons.indexOf(columns) + 1);
  await userEvent.click(exportButton);
  expect(values.exportLogs).toHaveBeenCalledTimes(1);
  expect(values.refresh).not.toHaveBeenCalled();
});

test.each([
  { loading: true },
  { exportingLogs: true },
  { canExportLogs: false },
])(
  'export is disabled while a query/download is pending or no query has loaded: %j',
  (state) => {
    render(<LogsFilters {...props()} {...state} />);
    expect(
      screen.getByRole('button', {
        name: state.exportingLogs ? 'Exporting...' : 'Export table',
      }),
    ).toBeDisabled();
  },
);
