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

const BUILT_IN_PAYMENT_METHODS = [
  {
    type: 'alipay',
    defaultName: 'Alipay',
    nameKey: '支付宝',
    gateway: '易支付',
  },
  {
    type: 'wxpay',
    defaultName: 'WeChat Pay',
    nameKey: '微信支付',
    gateway: '易支付',
  },
  { type: 'stripe', defaultName: 'Stripe', gateway: 'Stripe' },
  { type: 'creem', defaultName: 'Creem', gateway: 'Creem' },
  {
    type: 'wechat_native_v3',
    defaultName: 'WeChat Pay',
    nameKey: '微信支付',
    gateway: '微信支付',
  },
  { type: 'waffo', defaultName: 'Waffo Payment', gateway: 'Waffo' },
  {
    type: 'waffo_pancake',
    defaultName: 'Waffo Pancake',
    gateway: 'Waffo Pancake',
  },
];

const MAX_PAYMENT_METHOD_SETTINGS = 64;
const MAX_PAYMENT_METHOD_TYPE_LENGTH = 64;
const MAX_PAYMENT_METHOD_NAME_LENGTH = 128;
const MAX_PAYMENT_METHOD_SETTINGS_BYTES = 64 * 1024;

const DEDICATED_PAYMENT_TYPES = new Set(
  BUILT_IN_PAYMENT_METHODS.filter((method) => method.gateway !== '易支付').map(
    (method) => method.type,
  ),
);

const PAYMENT_METHOD_DEFAULT_ALIASES = {
  alipay: ['支付宝', 'Alipay'],
  wxpay: ['微信', '微信支付', 'WeChat'],
  wechat_native_v3: ['微信', '微信支付', 'WeChat'],
  waffo: ['Waffo', 'Waffo (Global Payment)'],
};

function isControlCharacter(character) {
  const codePoint = character.codePointAt(0);
  return (
    (codePoint >= 0 && codePoint <= 31) ||
    codePoint === 127 ||
    (codePoint >= 128 && codePoint <= 159)
  );
}

function isValidPaymentType(type) {
  if (!type || type.length > MAX_PAYMENT_METHOD_TYPE_LENGTH) return false;
  return Array.from(type).every((character) => {
    const codePoint = character.codePointAt(0);
    return codePoint >= 0x21 && codePoint <= 0x7e;
  });
}

export function parsePaymentMethodSettings(value) {
  let parsed = value;
  if (typeof value === 'string') {
    if (
      new TextEncoder().encode(value).length > MAX_PAYMENT_METHOD_SETTINGS_BYTES
    ) {
      return [];
    }
    try {
      parsed = JSON.parse(value || '[]');
    } catch {
      return [];
    }
  }
  if (!Array.isArray(parsed)) return [];

  return parsed.reduce((result, item) => {
    if (!item || typeof item !== 'object') return result;
    const type = typeof item.type === 'string' ? item.type.trim() : '';
    const name = typeof item.name === 'string' ? item.name.trim() : '';
    if (
      !isValidPaymentType(type) ||
      !name ||
      Array.from(name).length > MAX_PAYMENT_METHOD_NAME_LENGTH ||
      Array.from(name).some(isControlCharacter) ||
      (item.enabled !== undefined &&
        item.enabled !== null &&
        typeof item.enabled !== 'boolean')
    ) {
      return result;
    }
    if (result.length >= MAX_PAYMENT_METHOD_SETTINGS) return result;
    if (
      result.some(
        (entry) =>
          normalizePaymentMethodType(entry.type) ===
          normalizePaymentMethodType(type),
      )
    ) {
      return result;
    }
    result.push({
      type: normalizePaymentMethodType(type),
      name,
      enabled: item.enabled !== false,
    });
    return result;
  }, []);
}

export function getPaymentMethodSetting(type, settings = []) {
  const normalizedType = normalizePaymentMethodType(type);
  if (!normalizedType) return undefined;
  if (!Array.isArray(settings)) return undefined;
  return settings.find(
    (setting) => normalizePaymentMethodType(setting.type) === normalizedType,
  );
}

export function normalizePaymentMethodType(type) {
  const normalizedType = typeof type === 'string' ? type.trim() : '';
  return normalizedType.startsWith('waffo:') ? 'waffo' : normalizedType;
}

export function isPaymentMethodEnabled(type, settings = []) {
  return getPaymentMethodSetting(type, settings)?.enabled !== false;
}

export function isPaymentMethodDefaultName(type, name, t) {
  const normalizedType = normalizePaymentMethodType(type);
  const definition = BUILT_IN_PAYMENT_METHODS.find(
    (method) => method.type === normalizedType,
  );
  const normalizedName = typeof name === 'string' ? name.trim() : '';
  if (!definition || !normalizedName) return false;
  const translatedName = t
    ? t(definition.nameKey || definition.defaultName)
    : definition.defaultName;
  return [
    definition.defaultName,
    translatedName,
    ...(PAYMENT_METHOD_DEFAULT_ALIASES[normalizedType] || []),
  ].includes(normalizedName);
}

export function getPaymentMethodDisplayName(
  type,
  fallbackName,
  settings = [],
  t,
) {
  const normalizedType = typeof type === 'string' ? type.trim() : '';
  const gatewayType = normalizePaymentMethodType(normalizedType);
  const override = getPaymentMethodSetting(gatewayType, settings);
  const definition = BUILT_IN_PAYMENT_METHODS.find(
    (method) => method.type === gatewayType,
  );
  const fallback = typeof fallbackName === 'string' ? fallbackName.trim() : '';
  const displayFallback =
    fallback && fallback !== normalizedType ? fallback : '';
  // Waffo's gateway setting applies to the provider as a whole. Preserve the
  // individual child labels (Card, Apple Pay, Google Pay) in the selector.
  if (normalizedType.startsWith('waffo:') && displayFallback)
    return displayFallback;
  if (definition) {
    const translatedDefault = t
      ? t(definition.nameKey || definition.defaultName)
      : definition.defaultName;
    if (override?.name) {
      if (isPaymentMethodDefaultName(gatewayType, override.name, t)) {
        return translatedDefault;
      }
      return override.name;
    }
    if (
      displayFallback &&
      !isPaymentMethodDefaultName(gatewayType, displayFallback, t)
    ) {
      return displayFallback;
    }
    return translatedDefault;
  }
  if (override?.name) return override.name;
  return fallback || normalizedType || '';
}

function parseEpayMethods(value) {
  let parsed = value;
  if (typeof value === 'string') {
    try {
      parsed = JSON.parse(value || '[]');
    } catch {
      return [];
    }
  }
  if (!Array.isArray(parsed)) return [];

  return parsed.reduce((result, item) => {
    if (!item || typeof item !== 'object') return result;
    const type = typeof item.type === 'string' ? item.type.trim() : '';
    const name = typeof item.name === 'string' ? item.name.trim() : '';
    if (
      !isValidPaymentType(type) ||
      !name ||
      Array.from(name).length > MAX_PAYMENT_METHOD_NAME_LENGTH ||
      Array.from(name).some(isControlCharacter) ||
      DEDICATED_PAYMENT_TYPES.has(type) ||
      type === 'waffo' ||
      type.startsWith('waffo:')
    ) {
      return result;
    }
    if (result.some((entry) => entry.type === type)) return result;
    result.push({
      type,
      defaultName: name,
      configuredName: name,
      gateway: '易支付',
    });
    return result;
  }, []);
}

export function getPaymentMethodDefinitions(epayMethodsValue) {
  const definitions = BUILT_IN_PAYMENT_METHODS.map((definition) => ({
    ...definition,
  }));
  const byType = new Map(
    definitions.map((definition) => [definition.type, definition]),
  );
  for (const definition of parseEpayMethods(epayMethodsValue)) {
    const existing = byType.get(definition.type);
    if (existing) {
      // PayMethods is the authoritative label for legacy Epay entries. Keep
      // dedicated gateway defaults intact if an old config contains a stale
      // entry with the same type.
      if (existing.gateway === '易支付') {
        existing.configuredName = definition.configuredName;
      }
      continue;
    }
    definitions.push(definition);
    byType.set(definition.type, definition);
  }
  return definitions;
}

export { BUILT_IN_PAYMENT_METHODS };
