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
import { CancelCircleIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'

import {
  createInvoiceRejectionSchema,
  type InvoiceRejectionFormValues,
} from '../lib/invoice-form'

export function InvoiceRejectDialog(props: {
  open: boolean
  isSubmitting: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (reason: string) => Promise<void>
}) {
  const { t } = useTranslation()
  const schema = useMemo(() => createInvoiceRejectionSchema(t), [t])
  const form = useForm<InvoiceRejectionFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { reason: '' },
  })

  useEffect(() => {
    if (!props.open) form.reset({ reason: '' })
  }, [form, props.open])

  const submit = async (values: InvoiceRejectionFormValues) => {
    const parsed = schema.parse(values)
    await props.onConfirm(parsed.reason)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!props.isSubmitting) props.onOpenChange(open)
      }}
    >
      <DialogContent>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(submit)}>
            <DialogHeader>
              <DialogTitle>{t('Reject invoice application')}</DialogTitle>
              <DialogDescription>
                {t(
                  'The reason is shown to the user. Rejected subscriptions become eligible for a corrected application.'
                )}
              </DialogDescription>
            </DialogHeader>
            <div className='py-4'>
              <FormField
                control={form.control}
                name='reason'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Rejection reason')}</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={5}
                        className='resize-y'
                        autoFocus
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                disabled={props.isSubmitting}
                onClick={() => props.onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='submit'
                variant='destructive'
                disabled={props.isSubmitting}
              >
                {props.isSubmitting ? (
                  <Spinner data-icon='inline-start' />
                ) : (
                  <HugeiconsIcon
                    icon={CancelCircleIcon}
                    data-icon='inline-start'
                  />
                )}
                {t('Reject application')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
