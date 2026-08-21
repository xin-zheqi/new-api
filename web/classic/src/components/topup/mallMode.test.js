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

import assert from 'node:assert/strict';
import { describe, test } from 'node:test';

import {
  canPurchaseBuiltInSubscriptions,
  isSafeEmbeddedMallUrl,
} from './mallMode.js';

describe('classic wallet mall mode', () => {
  test('only allows built-in subscription purchases after mall mode is known to be disabled', () => {
    assert.equal(canPurchaseBuiltInSubscriptions(false, false), false);
    assert.equal(canPurchaseBuiltInSubscriptions(false, true), false);
    assert.equal(canPurchaseBuiltInSubscriptions(true, true), false);
    assert.equal(canPurchaseBuiltInSubscriptions(true, false), true);
  });

  test('rejects same-origin and unsafe embedded mall URLs', () => {
    assert.equal(
      isSafeEmbeddedMallUrl('https://EXAMPLE.com./store', 'example.com'),
      false,
    );
    assert.equal(
      isSafeEmbeddedMallUrl('https://example.com../store', 'EXAMPLE.COM.'),
      false,
    );
    assert.equal(
      isSafeEmbeddedMallUrl('https://user@example.net/store', 'example.com'),
      false,
    );
    assert.equal(
      isSafeEmbeddedMallUrl('http://example.net/store', 'example.com'),
      false,
    );
    assert.equal(
      isSafeEmbeddedMallUrl('https://shop.example.net/store', 'example.com'),
      true,
    );
  });
});
