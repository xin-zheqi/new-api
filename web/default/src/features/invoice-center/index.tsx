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
import { Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuthStore } from '@/stores/auth-store'

import {
  createInvoiceApplication,
  downloadInvoicePDF,
  getInvoiceCenter,
} from './api'
import { InvoiceApplicationForm } from './components/invoice-application-form'
import { InvoiceHistory } from './components/invoice-history'
import { INVOICE_HISTORY_PAGE_SIZE, invoiceQueryKeys } from './constants'
import { getInvoiceErrorMessage } from './lib/invoice-error'
import { downloadInvoiceBlob } from './lib/invoice-form'
import type { InvoiceApplication, InvoiceApplicationPayload } from './types'

export function InvoiceCenter() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const subjectId = useAuthStore((state) => state.auth.user?.id ?? 0)
  const [historyPage, setHistoryPage] = useState(1)
  const [downloadingId, setDownloadingId] = useState<number | null>(null)
  const invoiceQuery = useQuery({
    queryKey: invoiceQueryKeys.center(
      subjectId,
      historyPage,
      INVOICE_HISTORY_PAGE_SIZE
    ),
    queryFn: () =>
      getInvoiceCenter({
        page: historyPage,
        pageSize: INVOICE_HISTORY_PAGE_SIZE,
      }),
    enabled: subjectId > 0,
    placeholderData: (previousData) => previousData,
  })
  const applyMutation = useMutation({
    mutationFn: createInvoiceApplication,
    onSuccess: () => {
      setHistoryPage(1)
      void queryClient.invalidateQueries({
        queryKey: invoiceQueryKeys.subject(subjectId),
      })
      toast.success(t('Invoice application submitted'))
    },
    onError: (error) => {
      toast.error(
        getInvoiceErrorMessage(
          error,
          t,
          'Failed to submit invoice application.'
        )
      )
    },
  })

  const applicationDay = invoiceQuery.data?.application_day ?? 0
  const remainingApplications = invoiceQuery.data?.remaining_applications ?? 0
  let disabledReason: string | undefined
  if (invoiceQuery.data && !invoiceQuery.data.identity_eligible) {
    disabledReason = t(
      'Invoice center is only available for university or enterprise users.'
    )
  } else if (invoiceQuery.data && !invoiceQuery.data.application_open) {
    disabledReason = t('Applications can only be submitted on day {{day}}.', {
      day: applicationDay,
    })
  } else if (remainingApplications === 0) {
    disabledReason = t(
      'You have reached this month’s invoice application limit.'
    )
  }

  const submitApplication = async (payload: InvoiceApplicationPayload) => {
    await applyMutation.mutateAsync(payload)
  }

  const downloadApplication = async (application: InvoiceApplication) => {
    setDownloadingId(application.id)
    try {
      const blob = await downloadInvoicePDF(application.id, 'user')
      downloadInvoiceBlob(
        blob,
        application.pdf_name || `invoice-${application.id}.pdf`
      )
    } catch (error) {
      toast.error(
        getInvoiceErrorMessage(error, t, 'Failed to download invoice PDF.')
      )
    } finally {
      setDownloadingId(null)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invoice Center')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto w-full max-w-6xl space-y-6'>
          {invoiceQuery.isLoading && (
            <div className='space-y-4'>
              <Skeleton className='h-20 w-full' />
              <Skeleton className='h-96 w-full' />
              <Skeleton className='h-48 w-full' />
            </div>
          )}

          {invoiceQuery.isError && (
            <Alert variant='destructive'>
              <AlertDescription className='flex flex-wrap items-center justify-between gap-3'>
                <span>
                  {getInvoiceErrorMessage(
                    invoiceQuery.error,
                    t,
                    'Failed to load invoice center.'
                  )}
                </span>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => void invoiceQuery.refetch()}
                >
                  <HugeiconsIcon
                    icon={Refresh01Icon}
                    data-icon='inline-start'
                  />
                  {t('Retry')}
                </Button>
              </AlertDescription>
            </Alert>
          )}

          {invoiceQuery.data && (
            <>
              <div className='space-y-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Applications open on day {{day}} of each month for eligible subscriptions from the past {{days}} days.',
                    {
                      day: invoiceQuery.data.application_day,
                      days: invoiceQuery.data.lookback_days,
                    }
                  )}
                </p>
                <dl className='grid divide-y rounded-md border sm:grid-cols-3 sm:divide-x sm:divide-y-0'>
                  <div className='p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('Eligible subscriptions')}
                    </dt>
                    <dd className='mt-1 text-lg font-semibold tabular-nums'>
                      {invoiceQuery.data.subscriptions.length}
                    </dd>
                  </div>
                  <div className='p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('Applications remaining this month')}
                    </dt>
                    <dd className='mt-1 text-lg font-semibold tabular-nums'>
                      {remainingApplications}
                    </dd>
                  </div>
                  <div className='p-3'>
                    <dt className='text-muted-foreground text-xs'>
                      {t('Eligible subscription lookback')}
                    </dt>
                    <dd className='mt-1 text-lg font-semibold tabular-nums'>
                      {t('{{days}} days', {
                        days: invoiceQuery.data.lookback_days,
                      })}
                    </dd>
                  </div>
                </dl>
              </div>

              <InvoiceApplicationForm
                subscriptions={invoiceQuery.data.subscriptions}
                disabledReason={disabledReason}
                isSubmitting={applyMutation.isPending}
                onSubmit={submitApplication}
              />

              <InvoiceHistory
                applications={invoiceQuery.data.applications}
                total={invoiceQuery.data.applications_total}
                page={invoiceQuery.data.page}
                pageSize={invoiceQuery.data.size}
                isFetching={invoiceQuery.isFetching}
                downloadingId={downloadingId}
                onPageChange={setHistoryPage}
                onDownload={(application) =>
                  void downloadApplication(application)
                }
              />
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
