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
import * as z from 'zod'

import { isSupportedCurrencyCode } from '@/lib/iso-currency'

const unsupportedFormatCharacters = /[<>\p{Cf}\p{Zl}\p{Zp}]/u

function isSupportedInvoiceText(value: string, multiline: boolean): boolean {
  if (unsupportedFormatCharacters.test(value)) return false
  for (const character of value) {
    const codePoint = character.codePointAt(0) ?? 0
    if (
      codePoint <= 8 ||
      codePoint === 11 ||
      codePoint === 12 ||
      (codePoint >= 14 && codePoint <= 31) ||
      codePoint === 127
    ) {
      return false
    }
  }
  return multiline || !/[\r\n]/.test(value)
}

function boundedInvoiceText(
  maxLength: number,
  multiline: boolean,
  tooLongMessage: string,
  unsupportedCharactersMessage: string
) {
  return z
    .string()
    .trim()
    .refine((value) => [...value].length <= maxLength, tooLongMessage)
    .refine(
      (value) => isSupportedInvoiceText(value, multiline),
      unsupportedCharactersMessage
    )
}

type Translate = (key: string) => string

const identityTranslate = ((key: string) => key) as Translate

export function createInvoiceApplicationSchema(
  t: Translate = identityTranslate
) {
  return z.object({
    invoice_title: boundedInvoiceText(
      255,
      false,
      t('Invoice title must not exceed 255 characters'),
      t('This field contains unsupported characters')
    ).refine((value) => value.length > 0, t('Invoice title is required')),
    taxpayer_id: boundedInvoiceText(
      32,
      false,
      t('Taxpayer ID must not exceed 32 characters'),
      t('This field contains unsupported characters')
    )
      .refine(
        (value) => value === '' || /^[A-Za-z0-9]+$/.test(value),
        t('Taxpayer ID may contain only letters and numbers')
      )
      .transform((value) => value.toUpperCase()),
    bank_name: boundedInvoiceText(
      255,
      false,
      t('Bank name must not exceed 255 characters'),
      t('This field contains unsupported characters')
    ),
    remark: boundedInvoiceText(
      1000,
      true,
      t('Invoice remark must not exceed 1000 characters'),
      t('This field contains unsupported characters')
    ),
    subscription_ids: z
      .array(z.number().int().refine((value) => value !== 0))
      .min(0)
      .max(100, t('Select no more than 100 subscriptions')),
  })
}

export function createInvoiceRejectionSchema(t: Translate = identityTranslate) {
  return z.object({
    reason: boundedInvoiceText(
      1000,
      true,
      t('Rejection reason must not exceed 1000 characters'),
      t('This field contains unsupported characters')
    ).refine((value) => value.length > 0, t('Rejection reason is required')),
  })
}

export const invoiceApplicationSchema = createInvoiceApplicationSchema()
export const invoiceRejectionSchema = createInvoiceRejectionSchema()

export type InvoiceApplicationFormValues = z.input<
  ReturnType<typeof createInvoiceApplicationSchema>
>

export type InvoiceRejectionFormValues = z.input<
  ReturnType<typeof createInvoiceRejectionSchema>
>

export function formatInvoiceTime(timestamp: number): string {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

export function formatInvoiceMoney(
  amountMicros: number | null | undefined,
  currency: string | null | undefined,
  locale?: string
): string {
  const normalizedCurrency = currency?.trim().toUpperCase() ?? ''
  if (
    !Number.isSafeInteger(amountMicros) ||
    (amountMicros ?? 0) <= 0 ||
    !isSupportedCurrencyCode(normalizedCurrency)
  ) {
    return '-'
  }

  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: normalizedCurrency,
      currencyDisplay: 'code',
      maximumFractionDigits: 6,
    }).format((amountMicros as number) / 1_000_000)
  } catch {
    return '-'
  }
}

export function downloadInvoiceBlob(blob: Blob, fileName: string): void {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = fileName
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}
