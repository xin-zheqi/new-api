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
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Field, FieldContent, FieldGroup } from '@/components/ui/field'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../lib/channel-form'

export function ToolOutputRepairSetting(props: { disabled?: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()

  return (
    <FormField
      control={form.control}
      name='tool_output_repair'
      render={({ field }) => (
        <FieldGroup>
          <FormItem className='px-4 py-3'>
            <Field orientation='horizontal' className='items-center gap-4'>
              <FieldContent>
                <FormLabel>{t('Repair missing tool outputs')}</FormLabel>
                <FormDescription>
                  {t(
                    'Repair explicit Chat Completions and Responses tool history. Reuse matching results or mark missing results as unavailable; never executes tools. Stateful Responses history and body passthrough are skipped.'
                  )}
                </FormDescription>
              </FieldContent>
              <FormControl>
                <Switch
                  checked={field.value === true}
                  onCheckedChange={field.onChange}
                  disabled={props.disabled}
                  onBlur={field.onBlur}
                  name={field.name}
                  ref={field.ref}
                />
              </FormControl>
            </Field>
          </FormItem>
        </FieldGroup>
      )}
    />
  )
}
