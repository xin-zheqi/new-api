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
import { useId } from 'react'
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
  MAX_CHANNEL_SCHEDULE_PERIODS,
  type ChannelFormValues,
} from '../lib/channel-form'

const COMMON_TIMEZONES = [
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Singapore',
  'Asia/Hong_Kong',
  'UTC',
  'America/New_York',
  'America/Los_Angeles',
  'Europe/London',
  'Europe/Berlin',
]

export function ChannelScheduleSetting(props: { disabled?: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const timezoneListId = useId()
  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'channel_schedule.periods',
  })
  const errors = form.formState.errors.channel_schedule?.periods
  const errorMessage = errors?.message || errors?.root?.message

  return (
    <FieldSet className='gap-3' disabled={props.disabled}>
      <FieldLegend variant='label'>{t('Channel active hours')}</FieldLegend>
      <FieldDescription>
        {t(
          'Daily ranges use the selected time zone. Overnight ranges are supported; the end time is excluded. Leave empty for unrestricted hours. Manual and health disables remain in effect.'
        )}
      </FieldDescription>
      <FormField
        control={form.control}
        name='channel_schedule.timezone'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Time zone')}</FormLabel>
            <FormControl>
              <Input
                {...field}
                value={field.value ?? ''}
                list={timezoneListId}
                disabled={props.disabled}
                placeholder='Asia/Shanghai'
              />
            </FormControl>
            <datalist id={timezoneListId}>
              {COMMON_TIMEZONES.map((zone) => (
                <option key={zone} value={zone} />
              ))}
            </datalist>
            <FormMessage />
          </FormItem>
        )}
      />
      {fields.length > 0 && (
        <FieldGroup className='gap-3'>
          {fields.map((period, index) => (
            <Field
              key={period.id}
              aria-label={`${t('Channel active hours')} ${index + 1}`}
              className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-start gap-2'
            >
              <FormField
                control={form.control}
                name={`channel_schedule.periods.${index}.start`}
                render={({ field }) => (
                  <FormItem className='min-w-0'>
                    <FormLabel>{t('Start time')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        disabled={props.disabled}
                        placeholder='09:00'
                        inputMode='numeric'
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`channel_schedule.periods.${index}.end`}
                render={({ field }) => (
                  <FormItem className='min-w-0'>
                    <FormLabel>{t('End time')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        disabled={props.disabled}
                        placeholder='18:00'
                        inputMode='numeric'
                      />
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
                aria-label={t('Remove time range')}
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
          props.disabled || fields.length >= MAX_CHANNEL_SCHEDULE_PERIODS
        }
        onClick={() => append({ start: '', end: '' })}
      >
        <Plus aria-hidden='true' data-icon='inline-start' />
        {t('Add time range')}
      </Button>
    </FieldSet>
  )
}
