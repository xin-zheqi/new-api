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
  formatPaymentAmount,
  getSafeHttpCheckoutUrl,
  openHttpCheckoutUrl,
  submitHttpCheckoutForm,
} from './payment.js';

describe('classic payment amount formatting', () => {
  test('uses the currency recorded with the payment', () => {
    const value = formatPaymentAmount(12.345678, 'eur', 'en-US');

    assert.match(value, /EUR/);
    assert.match(value, /12\.345678/);
  });

  test('does not guess a currency for legacy records', () => {
    assert.equal(formatPaymentAmount(12.34, '', 'en-US'), '-');
    assert.equal(formatPaymentAmount(Number.NaN, 'USD', 'en-US'), '-');
    assert.equal(formatPaymentAmount(12.34, 'USDT', 'en-US'), '-');
    assert.equal(formatPaymentAmount(12.34, 'XXX', 'en-US'), '-');
    assert.equal(formatPaymentAmount(12.34, 'XTS', 'en-US'), '-');
    assert.equal(formatPaymentAmount(12.34, 'ZZZ', 'en-US'), '-');
  });
});

describe('classic payment redirect URL safety', () => {
  test('accepts and normalizes absolute HTTP(S) checkout URLs', () => {
    assert.equal(
      getSafeHttpCheckoutUrl(' https://checkout.example.com/pay?id=1 '),
      'https://checkout.example.com/pay?id=1',
    );
    assert.equal(
      getSafeHttpCheckoutUrl('http://localhost:8080/checkout'),
      'http://localhost:8080/checkout',
    );
  });

  test('rejects executable, non-HTTP, relative, and malformed URLs', () => {
    const unsafeValues = [
      'javascript:alert(document.domain)',
      'data:text/html,<script>alert(1)</script>',
      'file:///tmp/checkout',
      '//checkout.example.com/pay',
      '/pay',
      'checkout.example.com/pay',
      '',
      '   ',
      null,
      undefined,
      {},
    ];

    for (const value of unsafeValues) {
      assert.equal(getSafeHttpCheckoutUrl(value), null);
    }
  });

  test('rejects unsafe targets before opening a window or creating a form', () => {
    assert.equal(openHttpCheckoutUrl('javascript:alert(1)'), false);
    assert.equal(
      submitHttpCheckoutForm('data:text/html,<h1>unsafe</h1>', {}),
      false,
    );
  });

  test('opens valid checkout URLs in an isolated new tab', () => {
    const originalWindow = Object.getOwnPropertyDescriptor(
      globalThis,
      'window',
    );
    const calls = [];
    Object.defineProperty(globalThis, 'window', {
      configurable: true,
      value: {
        open: (...args) => calls.push(args),
      },
    });

    try {
      assert.equal(
        openHttpCheckoutUrl('https://checkout.example.com/pay'),
        true,
      );
      assert.deepEqual(calls, [
        ['https://checkout.example.com/pay', '_blank', 'noopener,noreferrer'],
      ]);
    } finally {
      if (originalWindow) {
        Object.defineProperty(globalThis, 'window', originalWindow);
      } else {
        Reflect.deleteProperty(globalThis, 'window');
      }
    }
  });

  test('isolates checkout forms that open in a new tab', () => {
    const originalDocument = Object.getOwnPropertyDescriptor(
      globalThis,
      'document',
    );
    const originalNavigator = Object.getOwnPropertyDescriptor(
      globalThis,
      'navigator',
    );
    const form = {
      action: '',
      method: '',
      target: '',
      rel: '',
      appendChild: () => undefined,
      submit: () => undefined,
    };
    Object.defineProperty(globalThis, 'document', {
      configurable: true,
      value: {
        createElement: (tag) =>
          tag === 'form' ? form : { type: '', name: '', value: '' },
        body: {
          appendChild: () => undefined,
          removeChild: () => undefined,
        },
      },
    });
    Object.defineProperty(globalThis, 'navigator', {
      configurable: true,
      value: { userAgent: 'Mozilla/5.0 Chrome/140.0.0.0' },
    });

    try {
      assert.equal(
        submitHttpCheckoutForm('https://checkout.example.com/pay', {
          order_id: 'test-order',
        }),
        true,
      );
      assert.equal(form.action, 'https://checkout.example.com/pay');
      assert.equal(form.target, '_blank');
      assert.equal(form.rel, 'noopener noreferrer');
    } finally {
      if (originalDocument) {
        Object.defineProperty(globalThis, 'document', originalDocument);
      } else {
        Reflect.deleteProperty(globalThis, 'document');
      }
      if (originalNavigator) {
        Object.defineProperty(globalThis, 'navigator', originalNavigator);
      } else {
        Reflect.deleteProperty(globalThis, 'navigator');
      }
    }
  });
});
