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
import * as z from 'zod'
import { useMemo, useRef, useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

const tokenSafetySchema = z.object({
  token_setting: z.object({
    max_group_count: z.coerce.number().min(1).max(100),
  }),
})

type TokenSafetyFormInput = z.input<typeof tokenSafetySchema>
type TokenSafetyFormValues = z.output<typeof tokenSafetySchema>

type TokenSafetyDefaults = {
  'token_setting.max_group_count': number
}

const buildFormDefaults = (
  defaults: TokenSafetyDefaults
): TokenSafetyFormInput => ({
  token_setting: {
    max_group_count: defaults['token_setting.max_group_count'],
  },
})

const normalizeFormValues = (
  values: TokenSafetyFormValues
): TokenSafetyDefaults => ({
  'token_setting.max_group_count': values.token_setting.max_group_count,
})

type TokenSafetySectionProps = {
  defaultValues: TokenSafetyDefaults
}

export function TokenSafetySection({ defaultValues }: TokenSafetySectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )
  const form = useForm<TokenSafetyFormInput, unknown, TokenSafetyFormValues>({
    resolver: zodResolver(tokenSafetySchema),
    defaultValues: formDefaults,
  })
  const baselineRef = useRef(defaultValues)
  const baselineSerializedRef = useRef(JSON.stringify(defaultValues))

  useEffect(() => {
    const serialized = JSON.stringify(defaultValues)
    if (serialized === baselineSerializedRef.current) return
    baselineRef.current = defaultValues
    baselineSerializedRef.current = serialized
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (values: TokenSafetyFormValues) => {
    const normalized = normalizeFormValues(values)
    const changedKeys = (
      Object.keys(normalized) as Array<keyof TokenSafetyDefaults>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (changedKeys.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of changedKeys) {
      await updateOption.mutateAsync({ key, value: normalized[key] })
    }

    baselineRef.current = normalized
    baselineSerializedRef.current = JSON.stringify(normalized)
    form.reset(buildFormDefaults(normalized))
  }

  return (
    <SettingsSection title={t('Token safety limits')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <div>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Limit how many groups can be selected for one API key to avoid excessive routing checks.'
              )}
            </p>
          </div>

          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='token_setting.max_group_count'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Maximum groups per API key')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={100}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Allowed range: 1-100. Default: 50.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
