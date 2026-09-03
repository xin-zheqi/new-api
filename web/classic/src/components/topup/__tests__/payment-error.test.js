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
import test from 'node:test';
import { getPaymentAmountError } from '../paymentError.js';

test('uses the legacy data error returned by amount endpoints', () => {
  assert.equal(
    getPaymentAmountError('error', '充值金额过低', '获取金额失败'),
    '充值金额过低',
  );
});

test('uses the standard API message when data is absent', () => {
  assert.equal(
    getPaymentAmountError('请先确认支付合规声明', undefined, '获取金额失败'),
    '请先确认支付合规声明',
  );
});

test('falls back instead of exposing the legacy error sentinel', () => {
  assert.equal(
    getPaymentAmountError('error', undefined, '获取金额失败'),
    '获取金额失败',
  );
});
