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
import { Plus, Trash2 } from 'lucide-react'
import { useFieldArray, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import {
  MAX_REASONING_EFFORT_MAPPINGS,
  type ChannelFormValues,
} from '../lib/channel-form'

export function ReasoningEffortMappingSetting(props: { disabled?: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'reasoning_effort_mapping',
  })
  const mappingError = form.formState.errors.reasoning_effort_mapping
  const errorMessage = mappingError?.message || mappingError?.root?.message

  return (
    <FieldSet className='gap-3 px-4 py-3' disabled={props.disabled}>
      <FieldLegend variant='label'>{t('Reasoning effort mapping')}</FieldLegend>
      <FieldDescription>
        {t(
          'Map existing effort values once after conversion. Leave empty to keep requests unchanged. Targets must be supported by the upstream; body passthrough skips mapping.'
        )}
      </FieldDescription>
      {fields.length > 0 && (
        <FieldGroup className='gap-3'>
          {fields.map((row, index) => (
            <Field
              key={row.id}
              aria-label={`${t('Reasoning effort mapping')} ${index + 1}`}
              className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-start gap-2'
              data-disabled={props.disabled || undefined}
            >
              <FormField
                control={form.control}
                name={`reasoning_effort_mapping.${index}.from`}
                render={({ field }) => (
                  <FormItem className='min-w-0'>
                    <FormLabel>{t('Before mapping')}</FormLabel>
                    <FormControl>
                      <Input {...field} disabled={props.disabled} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`reasoning_effort_mapping.${index}.to`}
                render={({ field }) => (
                  <FormItem className='min-w-0'>
                    <FormLabel>{t('After mapping')}</FormLabel>
                    <FormControl>
                      <Input {...field} disabled={props.disabled} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='mt-6'
                aria-label={t('Remove mapping')}
                disabled={props.disabled}
                onClick={() => remove(index)}
              >
                <Trash2 aria-hidden='true' />
              </Button>
            </Field>
          ))}
        </FieldGroup>
      )}
      {errorMessage && <FieldError>{t(errorMessage)}</FieldError>}
      <Button
        type='button'
        variant='outline'
        size='sm'
        className='w-fit'
        disabled={
          props.disabled || fields.length >= MAX_REASONING_EFFORT_MAPPINGS
        }
        onClick={() => append({ from: '', to: '' })}
      >
        <Plus aria-hidden='true' data-icon='inline-start' />
        {t('Add mapping')}
      </Button>
    </FieldSet>
  )
}
