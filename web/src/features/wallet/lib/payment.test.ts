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

import { PAYMENT_TYPES } from '../constants'
import type { TopupInfo } from '../types'
import {
  canPurchaseBuiltInSubscriptions,
  dispatchSelectedPayment,
  getDefaultPaymentType,
  getPaymentUnitPrice,
  getPaymentMinTopupAmount,
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
  isWechatNativePayment,
} from './payment'

function topupInfo(overrides: Partial<TopupInfo> = {}): TopupInfo {
  return {
    enable_online_topup: false,
    enable_stripe_topup: false,
    enable_creem_topup: false,
    enable_waffo_topup: false,
    enable_waffo_pancake_topup: false,
    enable_wechat_topup: false,
    enable_redemption: true,
    pay_methods: [],
    min_topup: 1,
    stripe_min_topup: 1,
    amount_options: [],
    discount: {},
    creem_products: [],
    waffo_pay_methods: [],
    ...overrides,
  }
}

describe('default wallet mall mode', () => {
  test('only allows built-in subscriptions after mall mode is known to be disabled', () => {
    expect(canPurchaseBuiltInSubscriptions(false, false)).toBe(false)
    expect(canPurchaseBuiltInSubscriptions(false, true)).toBe(false)
    expect(canPurchaseBuiltInSubscriptions(true, true)).toBe(false)
    expect(canPurchaseBuiltInSubscriptions(true, false)).toBe(true)
  })

  test('keeps legacy Epay method names out of the official WeChat flow', () => {
    expect(isWechatNativePayment('wechat_native')).toBe(false)
    expect(isWechatNativePayment('wxpay')).toBe(false)
    expect(isWechatNativePayment(PAYMENT_TYPES.WECHAT_NATIVE)).toBe(true)
  })
})

describe('payment type classification', () => {
  test('keeps Waffo and Waffo Pancake on their dedicated flows', () => {
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO)).toBe(true)
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(false)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(true)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO)).toBe(false)
    expect(isStripePayment(PAYMENT_TYPES.STRIPE)).toBe(true)
  })
})

describe('wallet payment defaults and limits', () => {
  test('uses official WeChat when it is the only configured payment method', () => {
    const info = topupInfo({
      enable_wechat_topup: true,
      wechat_min_topup: 3,
      min_topup: 10,
      pay_methods: [
        {
          name: 'WeChat Pay',
          type: PAYMENT_TYPES.WECHAT_NATIVE,
          min_topup: 3,
        },
      ],
    })

    expect(getDefaultPaymentType(info)).toBe(PAYMENT_TYPES.WECHAT_NATIVE)
    expect(getPaymentMinTopupAmount(info)).toBe(3)
  })

  test('keeps WeChat and Epay minimum amounts independent', () => {
    const info = topupInfo({
      enable_online_topup: true,
      enable_wechat_topup: true,
      min_topup: 10,
      wechat_min_topup: 2,
      pay_methods: [
        { name: 'Alipay', type: PAYMENT_TYPES.ALIPAY, min_topup: 10 },
        {
          name: 'WeChat Pay',
          type: PAYMENT_TYPES.WECHAT_NATIVE,
          min_topup: 2,
        },
      ],
    })

    expect(getPaymentMinTopupAmount(info, PAYMENT_TYPES.ALIPAY)).toBe(10)
    expect(getPaymentMinTopupAmount(info, PAYMENT_TYPES.WECHAT_NATIVE)).toBe(2)
  })

  test('honors the server-selected default when multiple gateways are available', () => {
    const info = topupInfo({
      enable_online_topup: true,
      enable_wechat_topup: true,
      default_payment_method: PAYMENT_TYPES.WECHAT_NATIVE,
      pay_methods: [
        { name: 'Alipay', type: PAYMENT_TYPES.ALIPAY, min_topup: 1 },
        { name: 'WeChat Pay', type: PAYMENT_TYPES.WECHAT_NATIVE, min_topup: 2 },
      ],
    })

    expect(getDefaultPaymentType(info)).toBe(PAYMENT_TYPES.WECHAT_NATIVE)
  })

  test('uses the dedicated WeChat unit price without changing legacy pricing', () => {
    const info = topupInfo({ wechat_unit_price: 7.45 })

    expect(getPaymentUnitPrice(info, PAYMENT_TYPES.WECHAT_NATIVE, 6.8)).toBe(
      7.45
    )
    expect(getPaymentUnitPrice(info, PAYMENT_TYPES.ALIPAY, 6.8)).toBe(6.8)
  })
})

describe('payment dispatch', () => {
  test('keeps the selected Waffo method index through confirmation', async () => {
    const calls: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      3,
      {
        regular: async () => {
          calls.push('regular')
          return false
        },
        waffo: async (amount, index) => {
          calls.push(`waffo:${amount}:${index}`)
          return true
        },
        waffoPancake: async () => {
          calls.push('pancake')
          return false
        },
      }
    )

    expect(success).toBe(true)
    expect(calls).toEqual(['waffo:120:3'])
  })

  test('does not create a Waffo order without a selected method index', async () => {
    let called = false
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      null,
      {
        regular: async () => false,
        waffo: async () => {
          called = true
          return true
        },
        waffoPancake: async () => false,
      }
    )

    expect(success).toBe(false)
    expect(called).toBe(false)
  })
})
