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
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Download01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

import { formatInvoiceMoney, formatInvoiceTime } from '../lib/invoice-form'
import type { InvoiceApplication } from '../types'
import { InvoiceStatusBadge } from './invoice-status-badge'

export function InvoiceHistory(props: {
  applications: InvoiceApplication[]
  total: number
  page: number
  pageSize: number
  isFetching: boolean
  downloadingId: number | null
  onPageChange: (page: number) => void
  onDownload: (application: InvoiceApplication) => void
}) {
  const { t, i18n } = useTranslation()
  const pageCount = Math.max(1, Math.ceil(props.total / props.pageSize))

  return (
    <section className='space-y-3' aria-labelledby='invoice-history-heading'>
      <div className='flex flex-wrap items-end justify-between gap-2'>
        <div>
          <h3 id='invoice-history-heading' className='text-base font-semibold'>
            {t('Invoice applications')}
          </h3>
          <p className='text-muted-foreground text-sm'>
            {t('{{count}} application(s)', {
              count: props.total,
            })}
          </p>
        </div>
      </div>

      {props.applications.length > 0 ? (
        <div className='space-y-3'>
          {props.applications.map((application) => (
            <article
              key={application.id}
              className='space-y-3 rounded-md border p-4'
            >
              <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
                <div className='min-w-0'>
                  <h4 className='font-medium [overflow-wrap:anywhere] break-words'>
                    {application.invoice_title}
                  </h4>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    #{application.id} |{' '}
                    {formatInvoiceTime(application.created_at)}
                  </p>
                </div>
                <div className='flex shrink-0 items-center gap-2'>
                  <InvoiceStatusBadge status={application.status} />
                  {application.status === 'completed' &&
                    application.pdf_name && (
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        disabled={props.downloadingId === application.id}
                        onClick={() => props.onDownload(application)}
                      >
                        {props.downloadingId === application.id ? (
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
                </div>
              </div>

              <dl className='grid gap-x-4 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4'>
                <div>
                  <dt className='text-muted-foreground'>
                    {t('Invoice total amount')}
                  </dt>
                  <dd className='font-medium tabular-nums'>
                    {formatInvoiceMoney(
                      application.total_amount_micros,
                      application.currency,
                      i18n.resolvedLanguage
                    )}
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground'>{t('Taxpayer ID')}</dt>
                  <dd className='[overflow-wrap:anywhere] break-words'>
                    {application.taxpayer_id || '-'}
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground'>{t('Bank name')}</dt>
                  <dd className='[overflow-wrap:anywhere] break-words'>
                    {application.bank_name || '-'}
                  </dd>
                </div>
                <div>
                  <dt className='text-muted-foreground'>
                    {t('Subscriptions')}
                  </dt>
                  <dd>{application.items?.length ?? 0}</dd>
                </div>
              </dl>

              {application.remark && (
                <div className='text-sm'>
                  <p className='text-muted-foreground'>{t('Invoice remark')}</p>
                  <p className='mt-1 [overflow-wrap:anywhere] break-words whitespace-pre-wrap'>
                    {application.remark}
                  </p>
                </div>
              )}

              {application.status === 'rejected' &&
                application.rejection_reason && (
                  <Alert variant='destructive'>
                    <AlertTitle>{t('Application rejected')}</AlertTitle>
                    <AlertDescription className='[overflow-wrap:anywhere] break-words whitespace-pre-wrap'>
                      {application.rejection_reason}
                    </AlertDescription>
                  </Alert>
                )}
            </article>
          ))}
        </div>
      ) : (
        <div className='text-muted-foreground rounded-md border border-dashed p-8 text-center text-sm'>
          {t('No invoice applications')}
        </div>
      )}

      {pageCount > 1 ? (
        <div className='flex items-center justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            size='icon-sm'
            disabled={props.isFetching || props.page <= 1}
            aria-label={t('Previous page')}
            title={t('Previous page')}
            onClick={() => props.onPageChange(Math.max(1, props.page - 1))}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} />
          </Button>
          <span className='text-muted-foreground min-w-20 text-center text-sm tabular-nums'>
            {t('{{page}} / {{pages}}', {
              page: props.page,
              pages: pageCount,
            })}
          </span>
          <Button
            type='button'
            variant='outline'
            size='icon-sm'
            disabled={props.isFetching || props.page >= pageCount}
            aria-label={t('Next page')}
            title={t('Next page')}
            onClick={() =>
              props.onPageChange(Math.min(pageCount, props.page + 1))
            }
          >
            <HugeiconsIcon icon={ArrowRight01Icon} />
          </Button>
        </div>
      ) : null}
    </section>
  )
}
