import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
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

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

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
  const updateOption = useUpdateOption()
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues })
  useResetForm(form, defaultValues)

  const onSubmit = async (values: Values) => {
    for (const [key, value] of Object.entries(values)) {
      if (value !== defaultValues[key as keyof Values]) {
        await updateOption.mutateAsync({ key, value })
      }
    }
  }

  return (
    <SettingsSection title={t('Invoice settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
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
