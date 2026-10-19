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
import { PlusCircle } from 'lucide-react'
import { useState } from 'react'
import { Controller, useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { MultiSelect } from '@/components/multi-select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldLabel,
  FieldGroup,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useAuthStore } from '@/stores/auth-store'

import { saveAuditPolicy, testAuditPolicy } from './api'
import { AuditMatches } from './matches'
import {
  newAuditRule,
  normalizeAuditPolicy,
  presets,
  regexFlagOptions,
  type AuditPolicy,
  type PolicyResponse,
} from './types'

const MAX_RULES = 20

const schema = z.object({
  enabled: z.boolean(),
  rules: z
    .array(
      z.object({
        id: z.string(),
        name: z.string().trim().min(1).max(128),
        enabled: z.boolean(),
        pattern: z.string().min(1).max(512),
        flags: z
          .string()
          .regex(/^[ims]*$/)
          .refine((value) => new Set(value).size === value.length),
        mode: z.enum(['all', 'first']),
        action: z.enum(['warn', 'replace', 'block']),
        replacement: z.string().max(512),
      })
    )
    .max(MAX_RULES),
})

export function AuditRules(props: { data: PolicyResponse }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const owner = useAuthStore((state) => state.auth.user?.id)
  const maxRules = Math.min(props.data.max_rules || MAX_RULES, MAX_RULES)
  const form = useForm<AuditPolicy>({
    resolver: zodResolver(schema),
    defaultValues: props.data.policy,
  })
  const fields = useFieldArray({
    control: form.control,
    name: 'rules',
    keyName: 'formKey',
  })
  const [sample, setSample] = useState('')
  const [preview, setPreview] =
    useState<Awaited<ReturnType<typeof testAuditPolicy>>>()
  const save = useMutation({
    gcTime: 0,
    mutationFn: (policy: AuditPolicy) =>
      saveAuditPolicy(normalizeAuditPolicy(policy)),
    onSuccess: (_, values) => {
      const normalized = normalizeAuditPolicy(values)
      form.reset(normalized)
      client.setQueryData<PolicyResponse>(['content-audit-policy', owner], {
        ...props.data,
        policy: normalized,
      })
      toast.success(t('Saved'))
    },
  })
  const test = useMutation({
    gcTime: 0,
    mutationFn: (policy: AuditPolicy) =>
      testAuditPolicy(normalizeAuditPolicy(policy), sample),
    onSuccess: setPreview,
  })
  const error = save.error || test.error
  const flagOptions = regexFlagOptions.map((option) => ({
    value: option.value,
    label: t(option.label),
  }))
  return (
    <div className='flex flex-col gap-4'>
      <Alert>
        <AlertDescription>
          {t(
            'Rules use RE2 (no lookaround or backreferences). Matching is case-sensitive unless i is set; m = multiline, s = dot matches newline. Use ${1} or ${name} in replacements.'
          )}
        </AlertDescription>
      </Alert>
      <Alert>
        <AlertDescription>
          {t(
            'Block takes priority. Rules inspect original text; the first replacement wins overlapping matches. First match means once per text field. Test presets before enabling them.'
          )}
        </AlertDescription>
      </Alert>
      <form
        className='flex flex-col gap-4'
        onSubmit={form.handleSubmit((values) => save.mutate(values))}
      >
        <Field orientation='horizontal'>
          <Controller
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <Switch
                id='audit-enabled'
                checked={field.value}
                onCheckedChange={field.onChange}
              />
            )}
          />
          <FieldLabel htmlFor='audit-enabled'>
            {t('Enable my content audit')}
          </FieldLabel>
        </Field>
        <div className='flex flex-col gap-3'>
          <div className='border-border/60 flex flex-col gap-2 border-t pt-3'>
            <p className='text-sm font-medium'>{t('Common privacy presets')}</p>
            <p className='text-muted-foreground text-sm'>
              {t('Choose a preset to add a ready-to-edit rule.')}
            </p>
            <div className='flex flex-wrap gap-2'>
              {presets.map((preset) => (
                <Button
                  key={preset.name}
                  type='button'
                  variant='secondary'
                  disabled={fields.fields.length >= maxRules}
                  onClick={() =>
                    fields.append({
                      ...newAuditRule(),
                      ...preset,
                      name: t(preset.name),
                    })
                  }
                >
                  {t(preset.name)}
                </Button>
              ))}
            </div>
          </div>
        </div>
        {fields.fields.map((rule, index) => (
          <Card key={rule.formKey}>
            <CardHeader>
              <CardTitle>
                {t('Rule')} {index + 1}
              </CardTitle>
            </CardHeader>
            <CardContent className='flex flex-col gap-4'>
              <FieldGroup className='grid gap-4 sm:grid-cols-2'>
                <Field>
                  <FieldLabel htmlFor={`${rule.formKey}-name`}>
                    {t('Rule name')}
                  </FieldLabel>
                  <Input
                    id={`${rule.formKey}-name`}
                    maxLength={128}
                    {...form.register(`rules.${index}.name`)}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${rule.formKey}-action`}>
                    {t('Action')}
                  </FieldLabel>
                  <Controller
                    control={form.control}
                    name={`rules.${index}.action`}
                    render={({ field }) => (
                      <NativeSelect
                        id={`${rule.formKey}-action`}
                        value={field.value}
                        onChange={(event) => {
                          const action = event.target.value
                          field.onChange(action)
                          if (action !== 'replace') {
                            form.setValue(`rules.${index}.mode`, 'first')
                          }
                        }}
                      >
                        <NativeSelectOption value='warn'>
                          {t('warn')}
                        </NativeSelectOption>
                        <NativeSelectOption value='replace'>
                          {t('replace')}
                        </NativeSelectOption>
                        <NativeSelectOption value='block'>
                          {t('block')}
                        </NativeSelectOption>
                      </NativeSelect>
                    )}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${rule.formKey}-pattern`}>
                    {t('Regular expression')}
                  </FieldLabel>
                  <Input
                    id={`${rule.formKey}-pattern`}
                    autoComplete='off'
                    spellCheck={false}
                    maxLength={512}
                    className='font-mono'
                    {...form.register(`rules.${index}.pattern`)}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${rule.formKey}-replacement`}>
                    {t('Replacement')}
                  </FieldLabel>
                  <Input
                    id={`${rule.formKey}-replacement`}
                    autoComplete='off'
                    maxLength={512}
                    disabled={form.watch(`rules.${index}.action`) !== 'replace'}
                    {...form.register(`rules.${index}.replacement`)}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${rule.formKey}-flags`}>
                    {t('Matching flags')}
                  </FieldLabel>
                  <Controller
                    control={form.control}
                    name={`rules.${index}.flags`}
                    render={({ field }) => (
                      <MultiSelect
                        id={`${rule.formKey}-flags`}
                        options={flagOptions}
                        selected={[...field.value]}
                        onChange={(values) =>
                          field.onChange(
                            ['i', 'm', 's']
                              .filter((flag) => values.includes(flag))
                              .join('')
                          )
                        }
                        placeholder={t('No flags')}
                      />
                    )}
                  />
                  <FieldDescription>
                    {t(
                      'Select any combination: i ignores letter case, m makes ^ and $ match each line, and s lets . match line breaks.'
                    )}
                  </FieldDescription>
                </Field>
                {form.watch(`rules.${index}.action`) === 'replace' && (
                  <Field>
                    <FieldLabel htmlFor={`${rule.formKey}-mode`}>
                      {t('Match mode')}
                    </FieldLabel>
                    <NativeSelect
                      id={`${rule.formKey}-mode`}
                      {...form.register(`rules.${index}.mode`)}
                    >
                      <NativeSelectOption value='all'>
                        {t('All matches')}
                      </NativeSelectOption>
                      <NativeSelectOption value='first'>
                        {t('First match per text field')}
                      </NativeSelectOption>
                    </NativeSelect>
                  </Field>
                )}
              </FieldGroup>
              <div className='flex flex-wrap items-center gap-2'>
                <Field orientation='horizontal' className='w-auto'>
                  <Controller
                    control={form.control}
                    name={`rules.${index}.enabled`}
                    render={({ field }) => (
                      <Switch
                        id={`${rule.formKey}-enabled`}
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                  <FieldLabel htmlFor={`${rule.formKey}-enabled`}>
                    {t('Enabled')}
                  </FieldLabel>
                </Field>
                <Button
                  type='button'
                  variant='outline'
                  disabled={index === 0}
                  onClick={() => fields.move(index, index - 1)}
                >
                  {t('Move up')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  disabled={index === fields.fields.length - 1}
                  onClick={() => fields.move(index, index + 1)}
                >
                  {t('Move down')}
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  onClick={() => fields.remove(index)}
                >
                  {t('Delete')}
                </Button>
              </div>
            </CardContent>
          </Card>
        ))}
        <Button
          type='button'
          variant='outline'
          className='w-full border-dashed'
          disabled={fields.fields.length >= maxRules}
          onClick={() => fields.append(newAuditRule())}
        >
          <PlusCircle className='size-4' aria-hidden='true' />
          {t('Add rule')}
        </Button>
        {Object.keys(form.formState.errors).length > 0 && (
          <p role='alert' className='text-destructive'>
            {t('Check rule names, expressions, flags and length limits.')}
          </p>
        )}
        {error && (
          <p role='alert' className='text-destructive'>
            {error.message}
          </p>
        )}
        <Button type='submit' className='self-start' disabled={save.isPending}>
          {t('Save rules')}
        </Button>
      </form>
      <Card>
        <CardHeader>
          <CardTitle>{t('Test with synthetic text')}</CardTitle>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Do not upload real secrets. Preview runs on this server without saving a hit record or sending text upstream.'
            )}
          </p>
          <Field>
            <FieldLabel htmlFor='audit-sample'>{t('Test text')}</FieldLabel>
            <Textarea
              id='audit-sample'
              autoComplete='off'
              maxLength={8192}
              value={sample}
              onChange={(event) => setSample(event.target.value)}
            />
          </Field>
          <Button
            className='self-start'
            variant='outline'
            disabled={test.isPending}
            onClick={form.handleSubmit((values) => {
              setPreview(undefined)
              test.mutate(values)
            })}
          >
            {t('Test rules')}
          </Button>
          {preview && (
            <>
              <AuditMatches result={preview.result} />
              <pre className='bg-muted max-h-64 overflow-auto rounded-lg p-3 break-all whitespace-pre-wrap'>
                {preview.text}
              </pre>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
