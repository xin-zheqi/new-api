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
import {
  CancelCircleIcon,
  CheckmarkCircle02Icon,
  Delete02Icon,
  Download01Icon,
  Upload01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'

import {
  completeInvoiceApplication,
  deleteInvoicePDF,
  downloadInvoicePDF,
  rejectInvoiceApplication,
  uploadInvoicePDF,
} from '../api'
import { INVOICE_PDF_MAX_SIZE, invoiceQueryKeys } from '../constants'
import { getInvoiceErrorMessage } from '../lib/invoice-error'
import {
  downloadInvoiceBlob,
  formatInvoiceMoney,
  formatInvoiceTime,
} from '../lib/invoice-form'
import type { InvoiceApplication } from '../types'
import {
  AdminInvoiceConfirmDialog,
  type InvoiceConfirmAction,
} from './admin-invoice-confirm-dialog'
import { InvoiceRejectDialog } from './invoice-reject-dialog'
import { InvoiceStatusBadge } from './invoice-status-badge'

export function AdminInvoiceDetailSheet(props: {
  application: InvoiceApplication | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onResolved: () => void
}) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [rejectOpen, setRejectOpen] = useState(false)
  const [confirmAction, setConfirmAction] =
    useState<InvoiceConfirmAction | null>(null)
  const [downloading, setDownloading] = useState(false)
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: invoiceQueryKeys.all })
  const showError = (error: unknown, fallback: string) => {
    toast.error(getInvoiceErrorMessage(error, t, fallback))
    void refresh()
  }

  const uploadMutation = useMutation({
    mutationFn: (file: File) =>
      uploadInvoicePDF(props.application?.id ?? 0, file),
    onSuccess: () => {
      toast.success(t('Invoice PDF uploaded'))
      void refresh()
    },
    onError: (error) => showError(error, 'Failed to upload invoice PDF.'),
  })
  const deleteMutation = useMutation({
    mutationFn: () => deleteInvoicePDF(props.application?.id ?? 0),
    onSuccess: () => {
      setConfirmAction(null)
      toast.success(t('Invoice PDF deleted'))
      void refresh()
    },
    onError: (error) => showError(error, 'Failed to delete invoice PDF.'),
  })
  const completeMutation = useMutation({
    mutationFn: () => completeInvoiceApplication(props.application?.id ?? 0),
    onSuccess: () => {
      setConfirmAction(null)
      toast.success(t('Invoice completed'))
      void refresh()
      props.onResolved()
    },
    onError: (error) =>
      showError(error, 'Failed to complete invoice application.'),
  })
  const rejectMutation = useMutation({
    mutationFn: (reason: string) =>
      rejectInvoiceApplication(props.application?.id ?? 0, reason),
    onSuccess: () => {
      setRejectOpen(false)
      toast.success(t('Invoice application rejected'))
      void refresh()
      props.onResolved()
    },
    onError: (error) =>
      showError(error, 'Failed to reject invoice application.'),
  })
  const isBusy =
    uploadMutation.isPending ||
    deleteMutation.isPending ||
    completeMutation.isPending ||
    rejectMutation.isPending ||
    downloading

  const selectPDF = (file: File | undefined) => {
    if (!file) return
    if (
      !file.name.toLowerCase().endsWith('.pdf') ||
      file.type.toLowerCase() !== 'application/pdf'
    ) {
      toast.error(t('Select a valid PDF file.'))
      return
    }
    if (file.size <= 0 || file.size > INVOICE_PDF_MAX_SIZE) {
      toast.error(t('PDF must be between 1 byte and 20 MB.'))
      return
    }
    uploadMutation.mutate(file)
  }

  const download = async () => {
    if (!props.application) return
    setDownloading(true)
    try {
      const blob = await downloadInvoicePDF(props.application.id, 'admin')
      downloadInvoiceBlob(
        blob,
        props.application.pdf_name || `invoice-${props.application.id}.pdf`
      )
    } catch (error) {
      showError(error, 'Failed to download invoice PDF.')
    } finally {
      setDownloading(false)
    }
  }

  const application = props.application
  const isPending = application?.status === 'pending'

  return (
    <>
      <Sheet
        open={props.open}
        onOpenChange={(open) => {
          if (isBusy && !open) return
          if (!open) {
            setRejectOpen(false)
            setConfirmAction(null)
          }
          props.onOpenChange(open)
        }}
      >
        <SheetContent className='w-full gap-0 sm:max-w-3xl'>
          <SheetHeader className='border-b'>
            <div className='flex flex-wrap items-center gap-2'>
              <SheetTitle>{t('Invoice application details')}</SheetTitle>
              {application && (
                <InvoiceStatusBadge status={application.status} />
              )}
            </div>
            <SheetDescription>
              {application ? `#${application.id}` : t('Loading...')}
            </SheetDescription>
          </SheetHeader>

          {application && (
            <div className='min-h-0 flex-1 overflow-y-auto'>
              <div className='space-y-6 p-4'>
                <section className='space-y-3'>
                  <h3 className='text-sm font-semibold'>{t('Applicant')}</h3>
                  <dl className='grid gap-3 text-sm sm:grid-cols-2'>
                    <div>
                      <dt className='text-muted-foreground'>{t('User')}</dt>
                      <dd className='[overflow-wrap:anywhere] break-words'>
                        {application.user?.display_name ||
                          application.user?.username ||
                          t('User #{{id}}', { id: application.user_id })}
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground'>{t('Email')}</dt>
                      <dd className='[overflow-wrap:anywhere] break-words'>
                        {application.user?.email || '-'}
                      </dd>
                    </div>
                  </dl>
                </section>

                <section className='space-y-3 border-t pt-5'>
                  <h3 className='text-sm font-semibold'>
                    {t('Invoice details')}
                  </h3>
                  <dl className='grid gap-3 text-sm sm:grid-cols-2'>
                    <div className='sm:col-span-2'>
                      <dt className='text-muted-foreground'>
                        {t('Invoice title')}
                      </dt>
                      <dd className='[overflow-wrap:anywhere] break-words whitespace-pre-wrap'>
                        {application.invoice_title}
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground'>
                        {t('Taxpayer ID')}
                      </dt>
                      <dd className='[overflow-wrap:anywhere] break-words'>
                        {application.taxpayer_id || '-'}
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground'>
                        {t('Bank name')}
                      </dt>
                      <dd className='[overflow-wrap:anywhere] break-words'>
                        {application.bank_name || '-'}
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground'>{t('Amount')}</dt>
                      <dd className='font-medium tabular-nums'>
                        {formatInvoiceMoney(
                          application.total_amount_micros,
                          application.currency,
                          i18n.resolvedLanguage
                        )}
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground'>
                        {t('Submitted at')}
                      </dt>
                      <dd>{formatInvoiceTime(application.created_at)}</dd>
                    </div>
                    <div className='sm:col-span-2'>
                      <dt className='text-muted-foreground'>
                        {t('Invoice remark')}
                      </dt>
                      <dd className='[overflow-wrap:anywhere] break-words whitespace-pre-wrap'>
                        {application.remark || '-'}
                      </dd>
                    </div>
                  </dl>
                </section>

                <section className='space-y-3 border-t pt-5'>
                  <h3 className='text-sm font-semibold'>
                    {t('Subscriptions')}
                  </h3>
                  <div className='divide-y rounded-md border'>
                    {application.items.map((item) => (
                      <div
                        key={item.id}
                        className='flex items-start justify-between gap-3 p-3 text-sm'
                      >
                        <div className='min-w-0'>
                          <p className='font-medium [overflow-wrap:anywhere] break-words'>
                            {item.item_type === 'redemption_recharge'
                                ? t('Redemption code balance recharge')
                                : item.item_type === 'top_up'
                                  ? item.plan_title || t('Balance recharge')
                                  : item.plan_title || t('Subscription')}
                          </p>
                          <p className='text-muted-foreground text-xs'>
                            #{item.top_up_id || item.redemption_id || item.user_subscription_id}
                          </p>
                        </div>
                        <span className='shrink-0 tabular-nums'>
                          {formatInvoiceMoney(
                            item.paid_amount_micros,
                            item.currency,
                            i18n.resolvedLanguage
                          )}
                        </span>
                      </div>
                    ))}
                  </div>
                </section>

                {application.status === 'rejected' &&
                  application.rejection_reason && (
                    <Alert variant='destructive'>
                      <AlertTitle>{t('Rejection reason')}</AlertTitle>
                      <AlertDescription className='[overflow-wrap:anywhere] break-words whitespace-pre-wrap'>
                        {application.rejection_reason}
                      </AlertDescription>
                    </Alert>
                  )}

                <section className='space-y-3 border-t pt-5'>
                  <div>
                    <h3 className='text-sm font-semibold'>
                      {t('Invoice PDF')}
                    </h3>
                    <p className='text-muted-foreground mt-1 text-sm [overflow-wrap:anywhere] break-words'>
                      {application.pdf_name || t('No PDF uploaded')}
                    </p>
                  </div>
                  <input
                    ref={fileInputRef}
                    type='file'
                    accept='application/pdf,.pdf'
                    className='hidden'
                    onChange={(event) => {
                      selectPDF(event.currentTarget.files?.[0])
                      event.currentTarget.value = ''
                    }}
                  />
                  <div className='flex flex-wrap gap-2'>
                    {isPending && (
                      <Button
                        type='button'
                        variant='outline'
                        disabled={isBusy}
                        onClick={() => fileInputRef.current?.click()}
                      >
                        {uploadMutation.isPending ? (
                          <Spinner data-icon='inline-start' />
                        ) : (
                          <HugeiconsIcon
                            icon={Upload01Icon}
                            data-icon='inline-start'
                          />
                        )}
                        {application.pdf_name
                          ? t('Replace PDF')
                          : t('Upload PDF')}
                      </Button>
                    )}
                    {application.pdf_name && (
                      <Button
                        type='button'
                        variant='outline'
                        disabled={isBusy}
                        onClick={() => void download()}
                      >
                        {downloading ? (
                          <Spinner data-icon='inline-start' />
                        ) : (
                          <HugeiconsIcon
                            icon={Download01Icon}
                            data-icon='inline-start'
                          />
                        )}
                        {t('Download PDF')}
                      </Button>
                    )}
                    {isPending && application.pdf_name && (
                      <Button
                        type='button'
                        variant='destructive'
                        disabled={isBusy}
                        onClick={() => setConfirmAction('delete')}
                      >
                        <HugeiconsIcon
                          icon={Delete02Icon}
                          data-icon='inline-start'
                        />
                        {t('Delete PDF')}
                      </Button>
                    )}
                  </div>
                </section>
              </div>

              {isPending && (
                <div className='bg-background sticky bottom-0 flex flex-wrap justify-end gap-2 border-t p-4'>
                  <Button
                    type='button'
                    variant='destructive'
                    disabled={isBusy}
                    onClick={() => setRejectOpen(true)}
                  >
                    <HugeiconsIcon
                      icon={CancelCircleIcon}
                      data-icon='inline-start'
                    />
                    {t('Reject application')}
                  </Button>
                  <Button
                    type='button'
                    disabled={isBusy || !application.pdf_name}
                    onClick={() => setConfirmAction('complete')}
                  >
                    <HugeiconsIcon
                      icon={CheckmarkCircle02Icon}
                      data-icon='inline-start'
                    />
                    {t('Complete invoice')}
                  </Button>
                </div>
              )}
            </div>
          )}
        </SheetContent>
      </Sheet>

      <InvoiceRejectDialog
        open={rejectOpen}
        isSubmitting={rejectMutation.isPending}
        onOpenChange={setRejectOpen}
        onConfirm={async (reason) => {
          await rejectMutation.mutateAsync(reason)
        }}
      />

      <AdminInvoiceConfirmDialog
        action={confirmAction}
        isSubmitting={deleteMutation.isPending || completeMutation.isPending}
        onOpenChange={(open) => {
          if (!open) setConfirmAction(null)
        }}
        onConfirm={() => {
          if (confirmAction === 'delete') deleteMutation.mutate()
          if (confirmAction === 'complete') completeMutation.mutate()
        }}
      />
    </>
  )
}
