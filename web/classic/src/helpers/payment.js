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

export function getSafeHttpCheckoutUrl(value) {
  if (typeof value !== 'string') return null;

  const trimmed = value.trim();
  if (!trimmed) return null;

  try {
    const url = new URL(trimmed);
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null;
    return url.href;
  } catch {
    return null;
  }
}

export function openHttpCheckoutUrl(value) {
  const url = getSafeHttpCheckoutUrl(value);
  if (!url) return false;

  window.open(url, '_blank', 'noopener,noreferrer');
  return true;
}

export function redirectToHttpCheckoutUrl(value) {
  const url = getSafeHttpCheckoutUrl(value);
  if (!url) return false;

  window.location.href = url;
  return true;
}

const FALLBACK_UNSUPPORTED_CURRENCIES = new Set(['XXX', 'XTS', 'ZZZ']);

export function isSupportedCurrencyCode(currency) {
  if (
    !/^[A-Z]{3}$/.test(currency) ||
    FALLBACK_UNSUPPORTED_CURRENCIES.has(currency)
  ) {
    return false;
  }

  const supportedValuesOf = Intl.supportedValuesOf;
  return (
    typeof supportedValuesOf !== 'function' ||
    supportedValuesOf('currency').includes(currency)
  );
}

export function formatPaymentAmount(amount, currency, locale) {
  const numeric = Number(amount);
  const normalizedCurrency = String(currency || '')
    .trim()
    .toUpperCase();
  if (
    !Number.isFinite(numeric) ||
    !isSupportedCurrencyCode(normalizedCurrency)
  ) {
    return '-';
  }

  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: normalizedCurrency,
      currencyDisplay: 'code',
      minimumFractionDigits: 2,
      maximumFractionDigits: 6,
    }).format(numeric);
  } catch {
    return '-';
  }
}

export function submitHttpCheckoutForm(value, params = {}) {
  const url = getSafeHttpCheckoutUrl(value);
  if (!url) return false;

  const form = document.createElement('form');
  form.action = url;
  form.method = 'POST';

  const isSafari =
    navigator.userAgent.includes('Safari') &&
    !navigator.userAgent.includes('Chrome');
  if (!isSafari) {
    form.target = '_blank';
    form.rel = 'noopener noreferrer';
  }

  Object.entries(params).forEach(([key, paramValue]) => {
    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = key;
    input.value = String(paramValue);
    form.appendChild(input);
  });

  document.body.appendChild(form);
  form.submit();
  document.body.removeChild(form);
  return true;
}
