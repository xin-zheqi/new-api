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

import { isSupportedCurrencyCode } from '../../helpers/payment.js';

export const INVOICE_TITLE_MAX_LENGTH = 255;
export const TAXPAYER_ID_MAX_LENGTH = 32;
export const BANK_NAME_MAX_LENGTH = 255;
export const INVOICE_REMARK_MAX_LENGTH = 1000;
export const REJECTION_REASON_MAX_LENGTH = 1000;
export const INVOICE_SEARCH_MAX_LENGTH = 100;
export const DEFAULT_PAGE_SIZE = 20;

const INVOICE_PDF_MAX_BYTES = 20 * 1024 * 1024;
export const clampInvoiceText = (value, maxLength) =>
  Array.from(value || '')
    .slice(0, maxLength)
    .join('');

export const formatInvoiceTime = (timestamp) => {
  const value = Number(timestamp || 0);
  return value > 0 ? new Date(value * 1000).toLocaleString() : '-';
};

export const formatInvoiceMoney = (amountMicros, currency, locale) => {
  const amount = Number(amountMicros);
  const normalizedCurrency = String(currency || '')
    .trim()
    .toUpperCase();
  if (
    !Number.isSafeInteger(amount) ||
    amount <= 0 ||
    !isSupportedCurrencyCode(normalizedCurrency)
  ) {
    return '-';
  }

  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: normalizedCurrency,
      currencyDisplay: 'code',
      maximumFractionDigits: 6,
    }).format(amount / 1000000);
  } catch {
    return '-';
  }
};

export const getInvoiceStatusMeta = (status, t) => {
  if (status === 'completed') {
    return { color: 'green', label: t('Completed') };
  }
  if (status === 'rejected') {
    return { color: 'red', label: t('Rejected') };
  }
  return { color: 'orange', label: t('Pending') };
};

export const validateInvoicePdf = async (file, t) => {
  if (!file || file.size <= 0) return t('Select a non-empty PDF file.');
  if (file.size > INVOICE_PDF_MAX_BYTES) {
    return t('The PDF file must not exceed 20 MiB.');
  }
  if (!file.name.toLowerCase().endsWith('.pdf')) {
    return t('Only PDF files are supported.');
  }
  if (file.type !== 'application/pdf') {
    return t('PDF content type must be application/pdf.');
  }
  try {
    const signature = new Uint8Array(await file.slice(0, 4).arrayBuffer());
    if (String.fromCharCode(...signature) !== '%PDF') {
      return t('The selected file is not a valid PDF.');
    }
  } catch {
    return t('The selected file could not be read.');
  }
  return '';
};

const INVOICE_ERROR_TRANSLATIONS = {
  'invoice center is disabled': (t) => t('Invoice center is disabled.'),
  'invoice application not found': (t) => t('Invoice application not found.'),
  'invoice application is no longer pending': (t) =>
    t(
      'This invoice application is no longer pending. Refresh the list and try again.',
    ),
  'upload an invoice pdf before completing the application': (t) =>
    t('The administrator must upload a PDF again before completion.'),
  'invoice application already submitted this month': (t) =>
    t("You have reached this month's invoice application limit."),
  'one or more subscriptions are not eligible for invoicing': (t) =>
    t('One or more subscriptions are no longer eligible for invoicing.'),
  'invalid subscription id': (t) =>
    t('One or more subscriptions are no longer eligible for invoicing.'),
  'duplicate subscription id': (t) =>
    t('One or more subscriptions are no longer eligible for invoicing.'),
  'subscription amount is invalid': (t) =>
    t('One or more subscriptions are no longer eligible for invoicing.'),
  'subscriptions in one invoice must use the same currency': (t) =>
    t('Each invoice application can include only one currency.'),
  'subscriptions in one invoice application must use the same currency': (t) =>
    t('Each invoice application can include only one currency.'),
  'invoice center is only available for university or enterprise users': (t) =>
    t('Invoice center is only available for university or enterprise users.'),
  'pdf upload must not exceed 20 mb': (t) =>
    t('The PDF file must not exceed 20 MiB.'),
  'pdf must be between 1 byte and 20 mb': (t) =>
    t('The PDF file must not exceed 20 MiB.'),
  'pdf content type is invalid': (t) =>
    t('PDF content type must be application/pdf.'),
  'invalid pdf upload': (t) => t('The selected file is not a valid PDF.'),
  'upload exactly one pdf file': (t) => t('Select a non-empty PDF file.'),
  'uploaded file is not a valid pdf': (t) =>
    t('The selected file is not a valid PDF.'),
  'uploaded pdf is incomplete': (t) => t('The uploaded PDF is incomplete.'),
  'uploaded pdf contains active or embedded content': (t) =>
    t('PDF files containing active or embedded content are not allowed.'),
  'failed to read invoice pdf': (t) =>
    t('The selected file could not be read.'),
  'invalid invoice pdf file name': (t) => t('Only PDF files are supported.'),
  'invalid invoice user id': (t) => t('Enter a valid user ID.'),
  'invalid invoice id': (t) => t('Invoice application not found.'),
  'invalid invoice application filter': (t) => t('Failed to load invoice data'),
  'invoice application is too large': (t) =>
    t('Invoice application is too large.'),
  'invalid invoice application': (t) => t('Invoice application is invalid.'),
  'invoice title and subscriptions are required': (t) =>
    t('Enter the invoice title and select at least one subscription.'),
  'too many subscriptions in one invoice application': (t) =>
    t('Too many subscriptions were selected for one application.'),
  'invalid invoice rejection': (t) =>
    t('The rejection reason is invalid. Check it and try again.'),
  'invalid invoice settings': (t) => t('Invoice settings are invalid.'),
  'all invoice settings are required': (t) =>
    t('All invoice settings are required.'),
  'invoice settings are outside the allowed range': (t) =>
    t('Invoice settings are outside the allowed range.'),
  'failed to update invoice settings': (t) =>
    t('Failed to save invoice settings'),
};

export const getInvoiceErrorMessage = (source, t, fallback) => {
  const payload = source?.response?.data ?? source?.data ?? source;
  const message =
    typeof payload === 'string'
      ? payload
      : typeof payload?.message === 'string'
        ? payload.message
        : '';
  const normalized = message.trim().toLowerCase();

  const applicationDay = normalized.match(
    /^invoice applications are accepted on day (\d{1,2}) of each month$/,
  );
  if (applicationDay) {
    return t(
      'Invoice applications are accepted only on day {{day}} of each month.',
      { day: applicationDay[1] },
    );
  }
  if (normalized.startsWith('invoice title ')) {
    return t('Enter the full invoice title');
  }
  if (normalized.startsWith('taxpayer id ')) {
    return t('Optional, letters and numbers only');
  }
  if (normalized.startsWith('rejection reason ')) {
    return t('The rejection reason is invalid. Check it and try again.');
  }

  const translate = INVOICE_ERROR_TRANSLATIONS[normalized];
  return translate ? translate(t) : fallback;
};
