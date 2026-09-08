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
import { Code2, Palette } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm, useWatch, type Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Button } from '@/components/ui/button'
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

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { RateLimitVisualEditor } from './rate-limit-visual-editor'

const isValidJSON = (value: string | undefined) => {
  if (!value || value.trim() === '') return true
  try {
    const parsed = JSON.parse(value)
    if (typeof parsed !== 'object' || Array.isArray(parsed)) {
      return false
    }
    for (const [, val] of Object.entries(parsed)) {
      if (!Array.isArray(val) || val.length !== 2) return false
      if (typeof val[0] !== 'number' || typeof val[1] !== 'number') return false
      if (val[0] < 0 || val[1] < 1) return false
      if (val[0] > 2147483647 || val[1] > 2147483647) return false
    }
    return true
  } catch {
    return false
  }
}

const createRateLimitSchema = (t: (key: string) => string) =>
  z.object({
    GlobalApiRateLimitEnabled: z.boolean(),
    GlobalApiRateLimitCount: z.number().int().min(1).max(100000000),
    GlobalApiRateLimitDuration: z.number().int().min(1).max(1200),
    GlobalWebRateLimitEnabled: z.boolean(),
    GlobalWebRateLimitCount: z.number().int().min(1).max(100000000),
    GlobalWebRateLimitDuration: z.number().int().min(1).max(1200),
    CriticalRateLimitEnabled: z.boolean(),
    CriticalRateLimitCount: z.number().int().min(1).max(100000000),
    CriticalRateLimitDuration: z.number().int().min(1).max(1200),
    ModelRequestRateLimitEnabled: z.boolean(),
    ModelRequestRateLimitDurationMinutes: z.number().min(0),
    ModelRequestRateLimitCount: z.number().min(0).max(100000000),
    ModelRequestRateLimitSuccessCount: z.number().min(1).max(100000000),
    ModelRequestRateLimitGroup: z
      .string()
      .optional()
      .refine(isValidJSON, {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
  })

type RateLimitFormValues = z.infer<ReturnType<typeof createRateLimitSchema>>

type RateLimitSectionProps = {
  defaultValues: RateLimitFormValues
}

type RouteRateLimitEnabledField =
  | 'GlobalApiRateLimitEnabled'
  | 'GlobalWebRateLimitEnabled'
  | 'CriticalRateLimitEnabled'

type RouteRateLimitNumberField =
  | 'GlobalApiRateLimitCount'
  | 'GlobalApiRateLimitDuration'
  | 'GlobalWebRateLimitCount'
  | 'GlobalWebRateLimitDuration'
  | 'CriticalRateLimitCount'
  | 'CriticalRateLimitDuration'

type RouteRateLimitControlsProps = {
  control: Control<RateLimitFormValues>
  enabledName: RouteRateLimitEnabledField
  countName: RouteRateLimitNumberField
  durationName: RouteRateLimitNumberField
  label: string
  description: string
}

function RouteRateLimitControls(props: RouteRateLimitControlsProps) {
  const { t } = useTranslation()
  const enabled = useWatch({
    control: props.control,
    name: props.enabledName,
  })

  return (
    <section
      data-settings-form-span='full'
      className='border-border space-y-4 border-b pb-6 last:border-b-0 last:pb-0'
    >
      <FormField
        control={props.control}
        name={props.enabledName}
        render={({ field }) => (
          <SettingsSwitchItem className='py-0'>
            <SettingsSwitchContent>
              <FormLabel>{props.label}</FormLabel>
              <FormDescription>{props.description}</FormDescription>
            </SettingsSwitchContent>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </SettingsSwitchItem>
        )}
      />
      <div className='grid gap-4 md:grid-cols-2'>
        <FormField
          control={props.control}
          name={props.countName}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Max requests per period')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={1}
                  max={100000000}
                  step={1}
                  disabled={!enabled}
                  {...field}
                  onChange={(event) =>
                    field.onChange(Number.parseInt(event.target.value, 10) || 0)
                  }
                />
              </FormControl>
              <FormDescription>{t('times')}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={props.control}
          name={props.durationName}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Limit period')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={1}
                  max={1200}
                  step={1}
                  disabled={!enabled}
                  {...field}
                  onChange={(event) =>
                    field.onChange(Number.parseInt(event.target.value, 10) || 0)
                  }
                />
              </FormControl>
              <FormDescription>{t('seconds')}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
    </section>
  )
}

export function RateLimitSection(props: RateLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [useVisualEditor, setUseVisualEditor] = useState(true)

  const rateLimitSchema = createRateLimitSchema(t)

  const form = useForm<RateLimitFormValues>({
    resolver: zodResolver(rateLimitSchema),
    mode: 'onChange', // Enable real-time validation
    defaultValues: props.defaultValues,
  })

  useEffect(() => {
    form.reset(props.defaultValues)
  }, [form, props.defaultValues])

  const onSubmit = async (values: RateLimitFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== props.defaultValues[key as keyof RateLimitFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Rate Limiting')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save rate limits'
          />
          <div data-settings-form-span='full' className='space-y-1'>
            <h3 className='text-sm font-semibold'>{t('Route rate limits')}</h3>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Route limits are applied by client IP and update immediately.'
              )}
            </p>
          </div>
          <RouteRateLimitControls
            control={form.control}
            enabledName='GlobalApiRateLimitEnabled'
            countName='GlobalApiRateLimitCount'
            durationName='GlobalApiRateLimitDuration'
            label={t('API route rate limit')}
            description={t('Controls requests to API endpoints.')}
          />
          <RouteRateLimitControls
            control={form.control}
            enabledName='GlobalWebRateLimitEnabled'
            countName='GlobalWebRateLimitCount'
            durationName='GlobalWebRateLimitDuration'
            label={t('Web route rate limit')}
            description={t('Controls frontend page and asset requests.')}
          />
          <RouteRateLimitControls
            control={form.control}
            enabledName='CriticalRateLimitEnabled'
            countName='CriticalRateLimitCount'
            durationName='CriticalRateLimitDuration'
            label={t('Login and sensitive action rate limit')}
            description={t(
              'Controls login, session refresh, password reset, and other sensitive actions.'
            )}
          />

          <div data-settings-form-span='full' className='pt-2'>
            <h3 className='text-sm font-semibold'>
              {t('Model request rate limits')}
            </h3>
          </div>
          <FormField
            control={form.control}
            name='ModelRequestRateLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable rate limiting')}</FormLabel>
                  <FormDescription>
                    {t('Controls model requests separately from route limits.')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='grid gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='ModelRequestRateLimitDurationMinutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Limit period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(
                            Number.parseInt(e.target.value, 10) || 0
                          )
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('minutes')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Time window for rate limiting')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max requests per period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        max={100000000}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(
                            Number.parseInt(e.target.value, 10) || 0
                          )
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Including failed requests, 0 = unlimited')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitSuccessCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max successful requests')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={1}
                        max={100000000}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(
                            Number.parseInt(e.target.value, 10) || 1
                          )
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Only successful requests')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='ModelRequestRateLimitGroup'
            render={({ field }) => (
              <FormItem>
                <div className='flex items-center justify-between'>
                  <FormLabel>{t('Group-based rate limits')}</FormLabel>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => setUseVisualEditor(!useVisualEditor)}
                  >
                    {useVisualEditor ? (
                      <>
                        <Code2 className='mr-2 h-4 w-4' />
                        {t('JSON Mode')}
                      </>
                    ) : (
                      <>
                        <Palette className='mr-2 h-4 w-4' />
                        {t('Visual Mode')}
                      </>
                    )}
                  </Button>
                </div>
                <FormControl>
                  {useVisualEditor ? (
                    <RateLimitVisualEditor
                      value={field.value || ''}
                      onChange={field.onChange}
                    />
                  ) : (
                    <JsonCodeEditor
                      value={field.value || ''}
                      onChange={field.onChange}
                      name={field.name}
                      onBlur={field.onBlur}
                      textareaRef={field.ref}
                      placeholder={`{\n  "default": [200, 100],\n  "vip": [0, 1000]\n}`}
                      aria-invalid={Boolean(
                        form.formState.errors.ModelRequestRateLimitGroup
                      )}
                    />
                  )}
                </FormControl>
                {!useVisualEditor && (
                  <FormDescription>
                    <div className='space-y-1 text-xs'>
                      <p className='font-semibold'>{t('Format:')}</p>
                      <ul className='list-inside list-disc space-y-0.5 pl-2'>
                        <li>
                          {t('JSON object:')}{' '}
                          {`{"groupName": [maxRequests, maxSuccess]}`}
                        </li>
                        <li>
                          {t('Example:')}{' '}
                          {`{"default": [200, 100], "vip": [0, 1000]}`}
                        </li>
                        <li>
                          {t(
                            'maxRequests ≥ 0, maxSuccess ≥ 1, both ≤ 2,147,483,647'
                          )}
                        </li>
                        <li>
                          {t(
                            'Group config overrides global limits, shares the same period'
                          )}
                        </li>
                      </ul>
                    </div>
                  </FormDescription>
                )}
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
