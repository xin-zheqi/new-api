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
import { z } from 'zod'
import { useForm, type Control, type Resolver } from 'react-hook-form'
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
import { Switch } from '@/components/ui/switch'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../system-settings/components/settings-form-layout'
import { SettingsPageFormActions } from '../system-settings/components/settings-page-context'
import { SettingsSection } from '../system-settings/components/settings-section'
import { useUpdateOption } from '../system-settings/hooks/use-update-option'

const schema = z.object({
  enabled: z.boolean(),
  requireRecharge: z.boolean(),
  minRechargeAmount: z.coerce.number().min(0),
  rechargeWindowDays: z.coerce.number().int().min(0),
  countRedemptionAsRecharge: z.boolean(),
  minAccountAgeDays: z.coerce.number().int().min(0),
  minRequestCount: z.coerce.number().int().min(0),
  requireEmailVerified: z.boolean(),
})

type Values = z.infer<typeof schema>

type LotterySettingsSectionProps = {
  defaultValues: Values
}

export function LotterySettingsSection(props: LotterySettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: props.defaultValues,
  })
  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')

  async function onSubmit(values: Values) {
    const updates: Array<{ key: string; value: string }> = []
    const pairs: Array<[keyof Values, string]> = [
      ['enabled', 'LotteryEnabled'],
      ['requireRecharge', 'LotteryRequireRecharge'],
      ['minRechargeAmount', 'LotteryMinRechargeAmount'],
      ['rechargeWindowDays', 'LotteryRechargeWindowDays'],
      ['countRedemptionAsRecharge', 'LotteryCountRedemptionAsRecharge'],
      ['minAccountAgeDays', 'LotteryMinAccountAgeDays'],
      ['minRequestCount', 'LotteryMinRequestCount'],
      ['requireEmailVerified', 'LotteryRequireEmailVerified'],
    ]

    for (const [field, key] of pairs) {
      if (values[field] !== props.defaultValues[field]) {
        updates.push({ key, value: String(values[field]) })
      }
    }

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
    form.reset(values)
  }

  return (
    <SettingsSection title={t('Lucky Draw Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save lucky draw settings'
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable lucky draw')}</FormLabel>
                  <FormDescription>
                    {t('Show the lucky draw page and allow users to join active draws.')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {enabled && (
            <>
              <FormField
                control={form.control}
                name='requireRecharge'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Require recharge history')}</FormLabel>
                      <FormDescription>
                        {t('Only users with successful recharge records can join draws.')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending || isSubmitting}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />

              <FormField
                control={form.control}
                name='requireEmailVerified'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Require bound email')}</FormLabel>
                      <FormDescription>
                        {t('Reduce repeated or disposable account participation.')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending || isSubmitting}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />

              <div className='grid gap-6 md:grid-cols-3'>
                <NumberField
                  control={form.control}
                  name='minRechargeAmount'
                  label={t('Minimum recharge amount')}
                  description={t('Set 0 to accept any successful recharge.')}
                />
                <NumberField
                  control={form.control}
                  name='rechargeWindowDays'
                  label={t('Recharge validity window')}
                  description={t('Set 0 to allow recharge records from any time. Unit: days.')}
                />
                <NumberField
                  control={form.control}
                  name='minAccountAgeDays'
                  label={t('Minimum account age')}
                  description={t('Days since registration required to join.')}
                />
                <NumberField
                  control={form.control}
                  name='minRequestCount'
                  label={t('Minimum request count')}
                  description={t('Require real API usage before joining draws.')}
                />
              </div>

              <FormField
                control={form.control}
                name='countRedemptionAsRecharge'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Count quota redemption as recharge')}</FormLabel>
                      <FormDescription>
                        {t('When enabled, quota redemption codes can satisfy recharge-based lottery conditions after amount conversion.')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending || isSubmitting}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

function NumberField(props: {
  control: Control<Values>
  name:
    | 'minRechargeAmount'
    | 'rechargeWindowDays'
    | 'minAccountAgeDays'
    | 'minRequestCount'
  label: string
  description: string
}) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <Input type='number' min={0} {...field} />
          </FormControl>
          <FormDescription>{props.description}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
