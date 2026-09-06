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

import type { PaymentMethodSetting } from '../types'

export type { PaymentMethodSetting } from '../types'

const MAX_PAYMENT_METHOD_SETTINGS_BYTES = 64 * 1024

/**
 * Parse the optional server-side payment method overlay. Invalid entries are
 * ignored so an older or hand-written response cannot break the wallet.
 */
export function parsePaymentMethodSettings(
  value: unknown
): PaymentMethodSetting[] {
  let parsed: unknown = value
  if (typeof value === 'string') {
    if (
      new TextEncoder().encode(value).length > MAX_PAYMENT_METHOD_SETTINGS_BYTES
    ) {
      return []
    }
    try {
      parsed = JSON.parse(value || '[]')
    } catch {
      return []
    }
  }
  if (!Array.isArray(parsed)) return []

  const settings: PaymentMethodSetting[] = []
  for (const item of parsed) {
    if (!item || typeof item !== 'object') continue
    const record = item as Record<string, unknown>
    const type = typeof record.type === 'string' ? record.type.trim() : ''
    const name = typeof record.name === 'string' ? record.name.trim() : ''
    const enabled = record.enabled
    if (
      !type ||
      !name ||
      type.length > 64 ||
      [...name].length > 128 ||
      (enabled !== undefined &&
        enabled !== null &&
        typeof enabled !== 'boolean')
    ) {
      continue
    }
    if (
      [...type].some((character) => {
        const codePoint = character.codePointAt(0) ?? 0
        return codePoint < 0x21 || codePoint > 0x7e
      }) ||
      [...name].some((character) => /\p{Cc}/u.test(character))
    ) {
      continue
    }
    if (
      settings.some(
        (entry) =>
          normalizePaymentMethodType(entry.type) ===
          normalizePaymentMethodType(type)
      )
    ) {
      continue
    }
    settings.push({
      type: normalizePaymentMethodType(type),
      name,
      enabled: enabled !== false,
    })
    if (settings.length >= 64) break
  }
  return settings
}

export function getPaymentMethodSetting(
  type: string,
  settings: PaymentMethodSetting[] | undefined
): PaymentMethodSetting | undefined {
  const normalizedType = normalizePaymentMethodType(type)
  if (!normalizedType) return undefined
  if (!Array.isArray(settings)) return undefined
  return settings.find(
    (setting) => normalizePaymentMethodType(setting.type) === normalizedType
  )
}

/**
 * Return the stable gateway identifier used by both checkout routes and the
 * settings overlay. Waffo exposes indexed child methods in the UI, but the
 * availability switch applies to the gateway as a whole.
 */
export function normalizePaymentMethodType(type: string): string {
  const normalizedType = typeof type === 'string' ? type.trim() : ''
  return normalizedType.startsWith('waffo:') ? 'waffo' : normalizedType
}

export function isPaymentMethodEnabled(
  type: string,
  settings: PaymentMethodSetting[] | undefined
): boolean {
  return getPaymentMethodSetting(type, settings)?.enabled !== false
}

/**
 * Built-in labels are translation keys. Administrator-provided names are
 * returned verbatim and are never treated as translation keys.
 */
export const PAYMENT_METHOD_DEFAULT_NAMES: Record<string, string> = {
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
  stripe: 'Stripe',
  creem: 'Creem',
  wechat_native_v3: 'WeChat Pay',
  waffo: 'Waffo Payment',
  waffo_pancake: 'Waffo Pancake',
}

// Older PayMethods entries commonly store localized labels (for example
// "支付宝" or "微信") instead of the stable names above. Treat only these
// known aliases as defaults; any other value is an administrator custom name.
const PAYMENT_METHOD_DEFAULT_ALIASES: Record<string, string[]> = {
  alipay: ['支付宝', 'Alipay'],
  wxpay: ['微信', '微信支付', 'WeChat'],
  wechat_native_v3: ['微信', '微信支付', 'WeChat'],
  waffo: ['Waffo', 'Waffo (Global Payment)'],
}

export function isPaymentMethodDefaultName(
  type: string,
  name: string | undefined,
  translate?: (key: string) => string
): boolean {
  const gatewayType = normalizePaymentMethodType(type)
  const defaultName = PAYMENT_METHOD_DEFAULT_NAMES[gatewayType]
  const normalizedName = name?.trim()
  if (!defaultName || !normalizedName) return false
  const translatedName = translate ? translate(defaultName) : defaultName
  return [
    defaultName,
    translatedName,
    ...(PAYMENT_METHOD_DEFAULT_ALIASES[gatewayType] ?? []),
  ].includes(normalizedName)
}

export function getPaymentMethodDisplayName(
  type: string,
  fallbackName: string | undefined,
  settings: PaymentMethodSetting[] | undefined,
  translate?: (key: string) => string
): string {
  const normalizedType = typeof type === 'string' ? type.trim() : ''
  const gatewayType = normalizePaymentMethodType(normalizedType)
  const override = getPaymentMethodSetting(gatewayType, settings)
  const defaultName = PAYMENT_METHOD_DEFAULT_NAMES[gatewayType]
  const fallback = fallbackName?.trim()
  const displayFallback =
    fallback && fallback !== normalizedType ? fallback : ''

  // Waffo exposes several provider-specific buttons (Card, Apple Pay, and
  // Google Pay) while the global setting controls the gateway as a whole.
  // Keep those child labels intact; applying the gateway label to every child
  // would make the buttons indistinguishable.
  const isWaffoChild = normalizedType.startsWith('waffo:')
  if (isWaffoChild && displayFallback) return displayFallback

  // The editor stores the canonical built-in name when only the enabled
  // state is overridden. Translate that value, while preserving an
  // administrator-provided Epay name returned by the server.
  if (override?.name) {
    if (isPaymentMethodDefaultName(gatewayType, override.name, translate)) {
      return translate ? translate(defaultName) : defaultName
    }
    return override.name
  }

  if (
    displayFallback &&
    (!defaultName ||
      !isPaymentMethodDefaultName(gatewayType, displayFallback, translate))
  ) {
    return displayFallback
  }
  if (defaultName) return translate ? translate(defaultName) : defaultName
  return fallback || normalizedType
}
