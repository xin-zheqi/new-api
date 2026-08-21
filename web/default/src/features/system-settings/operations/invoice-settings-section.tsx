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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
import { Switch } from '@/components/ui/switch'

import { updateInvoiceSettings } from '../api'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'

const schema = z.object({
  InvoiceEnabled: z.boolean(),
  InvoiceApplicationDay: z.number().int().min(1).max(28),
  InvoiceLookbackDays: z.number().int().min(1).max(3650),
  InvoiceMonthlyLimit: z.number().int().min(1).max(31),
})

type Values = z.infer<typeof schema>

export function InvoiceSettingsSection({
  defaultValues,
}: {
  defaultValues: Values
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues })
  useResetForm(form, defaultValues)
  const updateMutation = useMutation({
    mutationFn: (values: Values) =>
      updateInvoiceSettings({
        invoice_enabled: values.InvoiceEnabled,
        application_day: values.InvoiceApplicationDay,
        lookback_days: values.InvoiceLookbackDays,
        monthly_limit: values.InvoiceMonthlyLimit,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['system-options'] })
      void queryClient.invalidateQueries({ queryKey: ['status'] })
      try {
        window.localStorage.removeItem('status')
      } catch {
        // Storage can be unavailable in private mode.
      }
      toast.success(t('Invoice settings updated successfully'))
    },
    onError: (error) => {
      toast.error(
        error instanceof Error && error.message
          ? t(error.message, {
              defaultValue: t('Failed to update invoice settings'),
            })
          : t('Failed to update invoice settings')
      )
    },
  })

  const onSubmit = async (values: Values) => updateMutation.mutateAsync(values)

  return (
    <SettingsSection title={t('Invoice settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateMutation.isPending}
          />
          <FormField
            control={form.control}
            name='InvoiceEnabled'
            render={({ field }) => (
              <FormItem className='flex items-center justify-between rounded-md border p-3'>
                <div>
                  <FormLabel>{t('Enable invoice center')}</FormLabel>
                  <FormDescription>
                    {t('Allow eligible users to submit invoice applications.')}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value === true}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='InvoiceApplicationDay'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Application day of each month')}</FormLabel>
                <FormDescription>
                  {t('Users can submit applications on this day.')}
                </FormDescription>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    max={28}
                    value={field.value}
                    onChange={(event) =>
                      field.onChange(Number(event.target.value))
                    }
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='InvoiceLookbackDays'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Eligible subscription lookback (days)')}
                </FormLabel>
                <FormDescription>
                  {t(
                    'Only subscriptions created within this period can be selected.'
                  )}
                </FormDescription>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    max={3650}
                    value={field.value}
                    onChange={(event) =>
                      field.onChange(Number(event.target.value))
                    }
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='InvoiceMonthlyLimit'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Applications per user per month')}</FormLabel>
                <FormDescription>
                  {t(
                    'Maximum number of invoice applications per user in one month.'
                  )}
                </FormDescription>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    max={31}
                    value={field.value}
                    onChange={(event) =>
                      field.onChange(Number(event.target.value))
                    }
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
