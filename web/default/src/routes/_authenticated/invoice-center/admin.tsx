import { createFileRoute } from '@tanstack/react-router'

import { AdminInvoiceCenter } from '@/features/invoice-center/admin'

export const Route = createFileRoute('/_authenticated/invoice-center/admin')({
  component: AdminInvoiceCenter,
})
