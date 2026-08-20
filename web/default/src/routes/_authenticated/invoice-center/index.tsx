import { createFileRoute } from '@tanstack/react-router'

import { InvoiceCenter } from '@/features/invoice-center'

export const Route = createFileRoute('/_authenticated/invoice-center/')({
  component: InvoiceCenter,
})
