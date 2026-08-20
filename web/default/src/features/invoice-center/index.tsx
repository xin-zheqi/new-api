import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  Download,
  FileText,
  Receipt,
  WalletCards,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Main } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getUserId } from '@/features/auth/lib/storage'
import { api } from '@/lib/api'
import { formatQuota } from '@/lib/format'

import type { InvoiceApplication, InvoiceCenterData } from './types'

export function InvoiceCenter() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<number[]>([])
  const [invoiceTitle, setInvoiceTitle] = useState('')
  const query = useQuery({
    queryKey: ['invoice-center'],
    queryFn: async () =>
      (await api.get('/api/user/invoice')).data.data as InvoiceCenterData,
  })
  const applyMutation = useMutation({
    mutationFn: async () =>
      api.post('/api/user/invoice/apply', {
        invoice_title: invoiceTitle,
        subscription_ids: selected,
      }),
    onSuccess: (response) => {
      if (!response.data?.success) {
        toast.error(response.data?.message)
        return
      }
      toast.success(t('Invoice application submitted'))
      setSelected([])
      setInvoiceTitle('')
      queryClient.invalidateQueries({ queryKey: ['invoice-center'] })
    },
  })
  const total = useMemo(
    () =>
      query.data?.subscriptions
        .filter((item) => selected.includes(item.id))
        .reduce((sum, item) => sum + item.amount_total, 0) ?? 0,
    [query.data?.subscriptions, selected]
  )
  const now = new Date()
  const appliedThisMonth = query.data?.applications.some((item) => {
    const created = new Date(item.created_at * 1000)
    return (
      created.getFullYear() === now.getFullYear() &&
      created.getMonth() === now.getMonth()
    )
  })

  const downloadInvoice = async (id: number) => {
    const response = await api.get(`/api/user/invoice/${id}/download`, {
      responseType: 'blob',
      headers: { 'New-Api-User': getUserId() ?? '' },
    })
    const url = URL.createObjectURL(response.data)
    const link = document.createElement('a')
    link.href = url
    link.download = 'invoice.pdf'
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  }

  return (
    <Main>
      <div className='mx-auto flex w-full max-w-6xl flex-col gap-6 overflow-auto p-4 sm:p-6'>
        <header className='flex flex-col gap-4 border-b pb-5 sm:flex-row sm:items-end sm:justify-between'>
          <div>
            <div className='text-muted-foreground mb-2 flex items-center gap-2 text-sm'>
              <Receipt className='size-4' />
              {t('Billing and invoices')}
            </div>
            <h1 className='text-2xl font-semibold tracking-tight'>
              {t('Invoice Center')}
            </h1>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                'Applications open on day {{day}} of each month for eligible subscriptions from the past 90 days.',
                { day: query.data?.application_day ?? 25 }
              )}
            </p>
            <p className='text-destructive mt-1 text-sm'>
              {t(
                'Please verify the invoice title carefully. Issued invoices cannot be changed or reissued.'
              )}
            </p>
          </div>
          <div className='text-muted-foreground flex items-center gap-2 text-sm'>
            <CheckCircle2 className='size-4 text-emerald-600' />
            {appliedThisMonth
              ? t('Application submitted this month')
              : t('Available this month')}
          </div>
        </header>
        <div className='grid gap-4 sm:grid-cols-3'>
          <div className='bg-card flex items-center gap-3 rounded-lg border p-4'>
            <WalletCards className='text-primary size-5' />
            <div>
              <p className='text-muted-foreground text-xs'>
                {t('Eligible subscriptions')}
              </p>
              <p className='text-xl font-semibold'>
                {query.data?.subscriptions.length ?? 0}
              </p>
            </div>
          </div>
          <div className='bg-card flex items-center gap-3 rounded-lg border p-4'>
            <Receipt className='text-primary size-5' />
            <div>
              <p className='text-muted-foreground text-xs'>
                {t('Selected amount')}
              </p>
              <p className='text-xl font-semibold'>{formatQuota(total)}</p>
            </div>
          </div>
          <div className='bg-card flex items-center gap-3 rounded-lg border p-4'>
            <CheckCircle2 className='text-primary size-5' />
            <div>
              <p className='text-muted-foreground text-xs'>
                {t('Monthly applications')}
              </p>
              <p className='text-xl font-semibold'>
                {query.data?.monthly_limit ?? 1}
              </p>
            </div>
          </div>
        </div>
        <Card>
          <CardHeader>
            <CardTitle className='text-base'>
              {t('Eligible subscriptions')}
            </CardTitle>
          </CardHeader>
          <CardContent className='space-y-3'>
            {query.data?.subscriptions.length ? (
              query.data.subscriptions.map((subscription) => (
                <label
                  key={subscription.id}
                  className='hover:bg-muted/50 flex cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors'
                >
                  <Checkbox
                    checked={selected.includes(subscription.id)}
                    onCheckedChange={(checked) =>
                      setSelected((current) =>
                        checked
                          ? [...current, subscription.id]
                          : current.filter((id) => id !== subscription.id)
                      )
                    }
                  />
                  <span className='min-w-0 flex-1'>
                    <span className='block font-medium'>
                      {subscription.plan_title}
                    </span>
                    <span className='text-muted-foreground text-xs'>
                      {new Date(
                        subscription.created_at * 1000
                      ).toLocaleDateString()}
                    </span>
                  </span>
                  <span className='font-medium'>
                    {formatQuota(subscription.amount_total)}
                  </span>
                </label>
              ))
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('No eligible subscriptions')}
              </p>
            )}
            <div className='grid gap-3 border-t pt-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end'>
              <div className='space-y-1.5'>
                <Label htmlFor='invoice-title'>{t('Full invoice title')}</Label>
                <Input
                  id='invoice-title'
                  value={invoiceTitle}
                  onChange={(event) => setInvoiceTitle(event.target.value)}
                  placeholder={t('Enter the full invoice title')}
                  maxLength={255}
                />
              </div>
              <Button
                disabled={
                  !selected.length ||
                  !invoiceTitle.trim() ||
                  appliedThisMonth ||
                  applyMutation.isPending
                }
                onClick={() => applyMutation.mutate()}
              >
                {t('Apply for invoice')} | {formatQuota(total)}
              </Button>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className='text-base'>
              {t('Invoice applications')}
            </CardTitle>
          </CardHeader>
          <CardContent className='space-y-3'>
            {query.data?.applications.length ? (
              query.data.applications.map((application: InvoiceApplication) => (
                <div
                  key={application.id}
                  className='flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center'
                >
                  <FileText className='text-muted-foreground size-5' />
                  <div className='min-w-0 flex-1'>
                    <p className='font-medium'>{application.invoice_title}</p>
                    <p className='text-muted-foreground text-xs'>
                      {new Date(application.created_at * 1000).toLocaleString()}{' '}
                      | {formatQuota(application.total_amount)}
                    </p>
                  </div>
                  <Badge
                    variant={
                      application.status === 'completed'
                        ? 'default'
                        : 'secondary'
                    }
                  >
                    {t(
                      application.status === 'completed'
                        ? 'Completed'
                        : 'Pending'
                    )}
                  </Badge>
                  {application.status === 'completed' &&
                    application.pdf_name && (
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => downloadInvoice(application.id)}
                      >
                        <Download />
                        {t('Download PDF')}
                      </Button>
                    )}
                </div>
              ))
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('No invoice applications')}
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </Main>
  )
}
