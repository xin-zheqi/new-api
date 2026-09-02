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
import { describe, expect, test } from 'vitest'

import { getSafeEmbeddedMallUrl } from './safe-mall-url'

describe('safe embedded mall URL', () => {
  test('rejects equivalent same-origin hostnames', () => {
    expect(
      getSafeEmbeddedMallUrl('https://EXAMPLE.com../store', 'example.com.')
    ).toBeUndefined()
    expect(
      getSafeEmbeddedMallUrl(
        'https://例子.测试/store',
        'xn--fsqu00a.xn--0zwm56d'
      )
    ).toBeUndefined()
  })

  test('accepts only external HTTPS URLs without credentials', () => {
    expect(
      getSafeEmbeddedMallUrl('https://shop.example.net/store', 'example.com')
    ).toBe('https://shop.example.net/store')
    expect(
      getSafeEmbeddedMallUrl('https://user@example.net/store', 'example.com')
    ).toBeUndefined()
    expect(
      getSafeEmbeddedMallUrl('http://shop.example.net/store', 'example.com')
    ).toBeUndefined()
  })
})
