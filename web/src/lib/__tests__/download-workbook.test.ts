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
import { describe, expect, test, vi } from 'vitest'

import { downloadWorkbook } from '../download-workbook'

describe('usage workbook download', () => {
  test('uses a temporary object URL for XLSX and releases it after download', async () => {
    vi.useFakeTimers()
    const create = vi.fn(() => 'blob:usage-test')
    const revoke = vi.fn()
    vi.stubGlobal(
      'URL',
      Object.assign(URL, { createObjectURL: create, revokeObjectURL: revoke })
    )
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {})
    const blob = new Blob(['zip'], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    })
    await downloadWorkbook(blob)
    expect(create).toHaveBeenCalledWith(blob)
    expect(click).toHaveBeenCalledOnce()
    expect(document.querySelector('a[download]')).toBeNull()
    vi.runAllTimers()
    expect(revoke).toHaveBeenCalledWith('blob:usage-test')
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })
  test('a JSON error is never downloaded with an XLSX extension', async () => {
    const blob = {
      type: 'application/json',
      text: async () => '{"message":"Too many rows"}',
    } as Blob
    await expect(downloadWorkbook(blob)).rejects.toThrow('Too many rows')
  })
})
