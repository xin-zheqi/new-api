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
import type { StatusVariant } from '@/components/status-badge'

import type { AdminInvoiceListParams, InvoiceStatus } from './types'

export const INVOICE_PDF_MAX_SIZE = 20 * 1024 * 1024
export const INVOICE_HISTORY_PAGE_SIZE = 10

export const INVOICE_STATUS_VARIANTS: Record<InvoiceStatus, StatusVariant> = {
  pending: 'warning',
  completed: 'success',
  rejected: 'danger',
}

export const ADMIN_INVOICE_STATUS_OPTIONS: Array<{
  label: string
  value: InvoiceStatus
}> = [
  { label: 'Pending', value: 'pending' },
  { label: 'Completed', value: 'completed' },
  { label: 'Rejected', value: 'rejected' },
]

export const invoiceQueryKeys = {
  all: ['invoice-center'] as const,
  subject: (subjectId: number) =>
    [...invoiceQueryKeys.all, 'subject', subjectId] as const,
  center: (subjectId: number, page: number, pageSize: number) =>
    [...invoiceQueryKeys.subject(subjectId), 'self', page, pageSize] as const,
  adminLists: (subjectId: number) =>
    [...invoiceQueryKeys.subject(subjectId), 'admin-list'] as const,
  adminList: (subjectId: number, params: AdminInvoiceListParams) =>
    [...invoiceQueryKeys.adminLists(subjectId), params] as const,
}
