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
import { zodResolver } from '@hookform/resolvers/zod'
import { SentIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useMemo, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { FieldGroup } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

import {
  formatInvoiceMoney,
  createInvoiceApplicationSchema,
  type InvoiceApplicationFormValues,
} from '../lib/invoice-form'
import type { InvoiceApplicationPayload, InvoiceSubscription } from '../types'

const defaultValues: InvoiceApplicationFormValues = {
  invoice_title: '',
  taxpayer_id: '',
  bank_name: '',
  remark: '',
  subscription_ids: [],
}

export function InvoiceApplicationForm(props: {
  subscriptions: InvoiceSubscription[]
  disabledReason?: string
  isSubmitting: boolean
  onSubmit: (payload: InvoiceApplicationPayload) => Promise<void>
}) {
  const { t, i18n } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id ?? 0)
  const [pendingPayload, setPendingPayload] = useState<InvoiceApplicationPayload | null>(null)
  const schema = useMemo(() => createInvoiceApplicationSchema(t), [t])
  const form = useForm<InvoiceApplicationFormValues>({
    resolver: zodResolver(schema),
    defaultValues,
  })

  const invoiceDefaultsKey = `invoice-form-defaults-${userId}`
  useEffect(() => {
    if (userId <= 0) return
    try {
      const saved = localStorage.getItem(invoiceDefaultsKey)
      if (!saved) return
      const parsed = JSON.parse(saved) as Partial<InvoiceApplicationFormValues>
      form.reset({
        ...defaultValues,
        invoice_title: parsed.invoice_title?.trim() || '',
        taxpayer_id: parsed.taxpayer_id?.trim().toUpperCase() || '',
        bank_name: parsed.bank_name?.trim() || '',
        remark: parsed.remark?.trim() || '',
      })
    } catch {
      localStorage.removeItem(invoiceDefaultsKey)
    }
  }, [form, invoiceDefaultsKey, userId])
  const selectedIds =
    useWatch({ control: form.control, name: 'subscription_ids' }) ??
    defaultValues.subscription_ids
  const selectedSubscriptions = props.subscriptions.filter((subscription) =>
    selectedIds.includes(subscription.id)
  )
  const selectedCurrency = selectedSubscriptions[0]?.paid_currency ?? ''
  const selectedTotalMicros = selectedSubscriptions.reduce(
    (sum, subscription) => sum + subscription.paid_amount_micros,
    0
  )
  const availableCurrencies = new Set(
    props.subscriptions.map((subscription) => subscription.paid_currency)
  )
  let selectableSubscriptions: InvoiceSubscription[] = []
  if (selectedCurrency) {
    selectableSubscriptions = props.subscriptions.filter(
      (subscription) => subscription.paid_currency === selectedCurrency
    )
  } else if (availableCurrencies.size === 1) {
    selectableSubscriptions = props.subscriptions
  }
  const showSelectAll =
    props.subscriptions.length > 1 && selectableSubscriptions.length > 0

  const submit = async (values: InvoiceApplicationFormValues) => {
    try {
      const parsed = schema.parse(values)
      const selectedItems = props.subscriptions.filter((item) =>
        parsed.subscription_ids.includes(item.id)
      )
      if (selectedItems.length === 0) {
        form.setError('subscription_ids', {
          message: t('Select at least one eligible subscription'),
        })
        return
      }
      const currencies = new Set(selectedItems.map((item) => item.paid_currency))
      if (currencies.size !== 1) {
        form.setError('subscription_ids', {
          message: t('Each invoice application can include only one currency.'),
        })
        return
      }
      setPendingPayload({
        ...parsed,
        redemption_ids: selectedItems
          .filter((item) => item.item_type === 'redemption_recharge')
          .map((item) => item.redemption_id)
          .filter((id): id is number => typeof id === 'number' && id > 0),
        subscription_ids: selectedItems
          .filter((item) => item.item_type !== 'redemption_recharge')
          .map((item) => item.id),
      })
    } catch {
      // The form or mutation already presents the actionable error.
    }
  }

  return (
    <>
      <Form {...form}>
      <form
        className='space-y-5 rounded-md border p-4 sm:p-5'
        onSubmit={form.handleSubmit(submit)}
      >
        <div>
          <h3 className='text-base font-semibold'>
            {t('New invoice application')}
          </h3>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t(
              'Please verify the invoice details carefully. Completed invoices cannot be changed or reissued.'
            )}
          </p>
        </div>

        <FormField
          control={form.control}
          name='subscription_ids'
          render={({ field, fieldState }) => (
            <FormItem>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <div>
                  <FormLabel>{t('Eligible subscriptions')}</FormLabel>
                  <FormDescription>
                    {t('{{count}} subscription(s) selected', {
                      count: field.value.length,
                    })}
                    {availableCurrencies.size > 1
                      ? ` ${t('Each invoice application can include only one currency.')}`
                      : ''}
                  </FormDescription>
                </div>
                {showSelectAll ? (
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => {
                      const allSelected = selectableSubscriptions.every(
                        (subscription) => field.value.includes(subscription.id)
                      )
                      field.onChange(
                        allSelected
                          ? []
                          : selectableSubscriptions.map(
                              (subscription) => subscription.id
                            )
                      )
                      form.clearErrors('subscription_ids')
                    }}
                  >
                    {selectableSubscriptions.every((subscription) =>
                      field.value.includes(subscription.id)
                    )
                      ? t('Clear selection')
                      : t('Select all')}
                  </Button>
                ) : null}
              </div>

              {props.subscriptions.length > 0 ? (
                <div
                  role='group'
                  aria-invalid={fieldState.invalid}
                  className='grid max-h-72 gap-2 overflow-y-auto rounded-md border p-2'
                >
                  {props.subscriptions.map((subscription) => {
                    const checked = field.value.includes(subscription.id)
                    const incompatibleCurrency = Boolean(
                      selectedCurrency &&
                      subscription.paid_currency !== selectedCurrency
                    )
                    return (
                      <label
                        key={subscription.id}
                        aria-disabled={incompatibleCurrency}
                        title={
                          incompatibleCurrency
                            ? t(
                                'Each invoice application can include only one currency.'
                              )
                            : undefined
                        }
                        className={cn(
                          'flex items-center gap-3 rounded-md p-2.5 transition-colors',
                          incompatibleCurrency
                            ? 'cursor-not-allowed opacity-50'
                            : 'hover:bg-muted/50 cursor-pointer'
                        )}
                      >
                        <Checkbox
                          checked={checked}
                          disabled={incompatibleCurrency}
                          onCheckedChange={(nextChecked) => {
                            field.onChange(
                              nextChecked
                                ? [...field.value, subscription.id]
                                : field.value.filter(
                                    (id) => id !== subscription.id
                                  )
                            )
                            form.clearErrors('subscription_ids')
                          }}
                        />
                        <span className='min-w-0 flex-1'>
                          <span className='block font-medium [overflow-wrap:anywhere]'>
                            {subscription.item_type === 'redemption_recharge'
                                ? t('Redemption code balance recharge')
                                : subscription.item_type === 'top_up'
                                  ? subscription.plan_title || t('Balance recharge')
                                  : subscription.plan_title || t('Subscription')}
                          </span>
                          <span className='text-muted-foreground block text-xs'>
                            {new Date(
                              subscription.created_at * 1000
                            ).toLocaleDateString()}
                          </span>
                        </span>
                        <span className='shrink-0 font-medium tabular-nums'>
                          {formatInvoiceMoney(
                            subscription.paid_amount_micros,
                            subscription.paid_currency,
                            i18n.resolvedLanguage
                          )}
                        </span>
                      </label>
                    )
                  })}
                </div>
              ) : (
                <div className='text-muted-foreground rounded-md border border-dashed p-6 text-center text-sm'>
                  {t('No eligible subscriptions')}
                </div>
              )}
              <FormMessage />
            </FormItem>
          )}
        />

        <FieldGroup className='grid gap-4 md:grid-cols-2'>
          <FormField
            control={form.control}
            name='invoice_title'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Full invoice title')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='organization'
                    maxLength={255}
                    placeholder={t('Enter the full invoice title')}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='taxpayer_id'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Taxpayer ID')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    maxLength={32}
                    placeholder={t('Optional')}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='bank_name'
            render={({ field }) => (
              <FormItem className='md:col-span-2'>
                <FormLabel>{t('Bank name')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='organization'
                    maxLength={255}
                    placeholder={t('Optional')}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='remark'
            render={({ field }) => (
              <FormItem className='md:col-span-2'>
                <FormLabel>{t('Invoice remark')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    maxLength={1000}
                    placeholder={t('Optional')}
                    className='resize-y'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </FieldGroup>

        <div className='flex flex-col gap-2 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
          <div className='text-sm'>
            <span className='text-muted-foreground'>
              {t('Selected amount')}:
            </span>{' '}
            <span className='font-semibold tabular-nums'>
              {formatInvoiceMoney(
                selectedTotalMicros,
                selectedCurrency,
                i18n.resolvedLanguage
              )}
            </span>
          </div>
          <div className='flex flex-col items-stretch gap-1 sm:items-end'>
            <Button
              type='submit'
              disabled={
                props.isSubmitting ||
                props.subscriptions.length === 0 ||
                !!props.disabledReason
              }
            >
              {props.isSubmitting ? (
                <Spinner data-icon='inline-start' />
              ) : (
                <HugeiconsIcon icon={SentIcon} data-icon='inline-start' />
              )}
              {t('Submit application')}
            </Button>
            {props.disabledReason && (
              <p className='text-muted-foreground max-w-md text-xs'>
                {props.disabledReason}
              </p>
            )}
          </div>
        </div>
      </form>
      </Form>
      <AlertDialog
        open={pendingPayload !== null}
        onOpenChange={(open) => {
          if (!open) setPendingPayload(null)
        }}
      >
        <AlertDialogContent size='sm'>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm invoice application')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('Please verify the invoice details carefully. Completed invoices cannot be changed or reissued.')}
            </AlertDialogDescription>
            {pendingPayload ? (
              <div className='w-full space-y-1 text-sm'>
                <p><strong>{t('Invoice title')}:</strong> {pendingPayload.invoice_title}</p>
                <p><strong>{t('Taxpayer ID')}:</strong> {pendingPayload.taxpayer_id}</p>
                <p><strong>{t('Selected {{count}}', { count: pendingPayload.subscription_ids.length + pendingPayload.redemption_ids.length })}</strong></p>
              </div>
            ) : null}
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                if (!pendingPayload) return
                const payload = pendingPayload
                setPendingPayload(null)
                await props.onSubmit(payload)
                localStorage.setItem(
                  invoiceDefaultsKey,
                  JSON.stringify({
                    invoice_title: payload.invoice_title,
                    taxpayer_id: payload.taxpayer_id,
                    bank_name: payload.bank_name,
                    remark: payload.remark,
                  })
                )
                form.reset(defaultValues)
              }}
            >
              {t('Submit application')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
