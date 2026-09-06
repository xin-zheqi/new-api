import { describe, expect, test } from 'vitest'

import {
  getPaymentMethodDisplayName,
  isPaymentMethodEnabled,
  normalizePaymentMethodType,
  parsePaymentMethodSettings,
} from '../payment-methods'

describe('payment method settings', () => {
  test('parses valid entries and defaults omitted enabled to true', () => {
    expect(
      parsePaymentMethodSettings([
        { type: ' alipay ', name: ' 支付宝扫码 ' },
        { type: 'stripe', name: 'Stripe', enabled: false },
      ])
    ).toEqual([
      { type: 'alipay', name: '支付宝扫码', enabled: true },
      { type: 'stripe', name: 'Stripe', enabled: false },
    ])
  })

  test('ignores malformed, duplicate, and non-printable entries', () => {
    expect(
      parsePaymentMethodSettings([
        { type: 'stripe', name: 'first' },
        { type: 'stripe', name: 'duplicate' },
        { type: '带中文', name: 'invalid type' },
        { type: 'custom', name: 'bad\nname' },
        { type: 'custom-2', name: 'invalid enabled', enabled: 'false' },
        null,
      ])
    ).toEqual([{ type: 'stripe', name: 'first', enabled: true }])
  })

  test('accepts JSON strings and safely handles invalid input', () => {
    expect(
      parsePaymentMethodSettings('[{"type":"creem","name":"Creem"}]')
    ).toEqual([{ type: 'creem', name: 'Creem', enabled: true }])
    expect(parsePaymentMethodSettings('{"type":"stripe"}')).toEqual([])
    expect(parsePaymentMethodSettings('{bad json')).toEqual([])
  })

  test('canonicalizes Waffo child types so one gateway has one override', () => {
    expect(normalizePaymentMethodType(' waffo:0 ')).toBe('waffo')
    expect(
      parsePaymentMethodSettings([
        { type: 'waffo:0', name: '第一项', enabled: false },
        { type: ' waffo ', name: '重复项', enabled: true },
      ])
    ).toEqual([{ type: 'waffo', name: '第一项', enabled: false }])
  })

  test('keeps custom Epay labels ahead of built-in translated defaults', () => {
    const translate = (key: string) =>
      ({ Alipay: '支付宝', 'WeChat Pay': '微信支付' })[key] || key

    expect(
      getPaymentMethodDisplayName('alipay', '支付宝扫码', [], translate)
    ).toBe('支付宝扫码')
    expect(
      getPaymentMethodDisplayName(
        'alipay',
        '支付宝扫码',
        [{ type: 'alipay', name: '企业支付宝', enabled: true }],
        translate
      )
    ).toBe('企业支付宝')
    expect(
      getPaymentMethodDisplayName(
        'alipay',
        'Alipay',
        [{ type: 'alipay', name: 'Alipay', enabled: false }],
        translate
      )
    ).toBe('支付宝')
    expect(getPaymentMethodDisplayName('alipay', '支付宝', [], translate)).toBe(
      '支付宝'
    )
    expect(getPaymentMethodDisplayName('wxpay', '微信', [], translate)).toBe(
      '微信支付'
    )
  })

  test('does not expose a stable identifier used as a legacy fallback label', () => {
    const translate = (key: string) =>
      ({ 'WeChat Pay': '微信支付' })[key] || key

    expect(
      getPaymentMethodDisplayName(
        'wechat_native_v3',
        'wechat_native_v3',
        [],
        translate
      )
    ).toBe('微信支付')
  })

  test('only an explicit false disables a payment method', () => {
    expect(isPaymentMethodEnabled('stripe', undefined)).toBe(true)
    expect(
      isPaymentMethodEnabled('stripe', [
        { type: 'stripe', name: 'Stripe', enabled: false },
      ])
    ).toBe(false)
    expect(
      isPaymentMethodEnabled('stripe', [
        { type: 'stripe', name: 'Stripe', enabled: true },
      ])
    ).toBe(true)
  })

  test('preserves Waffo child labels when the gateway has a custom name', () => {
    expect(
      getPaymentMethodDisplayName(
        'waffo:0',
        'Card',
        [{ type: 'waffo', name: 'International checkout', enabled: true }],
        (key: string) => key
      )
    ).toBe('Card')
  })
})
