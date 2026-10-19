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
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import { updateSystemOption } from '../api'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'

const schema = z.object({
  enabled: z.boolean(),
  retention_days: z.number().int().min(1).max(90),
})
type Values = z.infer<typeof schema>
export function ContentAuditSection(props: { value: string }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  let defaults: Values = {
    enabled: false,
    retention_days: 7,
  }
  try {
    const parsed = schema.parse(JSON.parse(props.value))
    defaults = parsed
  } catch {
    /* Safe defaults before configuration. */
  }
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: defaults,
  })
  const save = useMutation({
    mutationFn: (values: Values) =>
      updateSystemOption({
        key: 'ContentAuditSettings',
        value: JSON.stringify(values),
      }),
    onSuccess: () => {
      const current = form.getValues()
      form.reset(current)
      void client.invalidateQueries({ queryKey: ['status'] })
      void client.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Saved'))
    },
  })
  return (
    <SettingsSection title={t('Content audit')}>
      <SettingsForm
        onSubmit={form.handleSubmit((values) => save.mutate(values))}
      >
        <SettingsPageFormActions
          onSave={form.handleSubmit((values) => save.mutate(values))}
          isSaving={save.isPending}
        />
        <Alert>
          <AlertDescription>
            {t(
              'Before enabling content audit, configure a persistent CRYPTO_SECRET or SESSION_SECRET in the server environment (Docker environment variables or a Compose/Kubernetes Secret). Keep it stable across replicas and restarts; it is not stored on this page.'
            )}
          </AlertDescription>
        </Alert>
        <FieldGroup>
          <Field orientation='horizontal'>
            <Controller
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <Switch
                  id='audit-site-enabled'
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor='audit-site-enabled'>
              {t('Enable content audit for users')}
            </FieldLabel>
          </Field>
          <Field>
            <FieldLabel htmlFor='audit-retention'>
              {t('Hit record retention (days)')}
            </FieldLabel>
            <Input
              id='audit-retention'
              type='number'
              min={1}
              max={90}
              {...form.register('retention_days', { valueAsNumber: true })}
            />
            <FieldDescription>
              {t(
                '1–90 days. Expired records are hidden immediately and hard-deleted within 10 minutes. Database files and backups may retain historical bytes; this is not secure media erasure.'
              )}
            </FieldDescription>
          </Field>
        </FieldGroup>
        {form.formState.errors.retention_days && (
          <p role='alert'>{t('Enter a whole number from 1 to 90.')}</p>
        )}
        {save.error && <p role='alert'>{save.error.message}</p>}
      </SettingsForm>
    </SettingsSection>
  )
}
