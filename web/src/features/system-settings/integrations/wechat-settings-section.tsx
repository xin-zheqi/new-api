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

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { safeNumberFieldProps } from '../utils/numeric-field'

type WechatFormValues = {
  WechatEnabled: boolean
  WechatMchID: string
  WechatAppID: string
  WechatApiV3Key: string
  WechatCertificateSerialNumber: string
  WechatPrivateKey: string
  WechatMerchantCertificate: string
  WechatPayPublicKeyID: string
  WechatPayPublicKey: string
  WechatPlatformCertificate: string
  WechatNotifyURL: string
  WechatOrderExpireMinutes: number
  WechatUnitPrice: number
}

export function WechatSettingsSection() {
  const { t } = useTranslation()
  const form = useFormContext<WechatFormValues>()
  const enabled = form.watch('WechatEnabled')

  return (
    <div className='flex flex-col gap-5'>
      <div>
        <h3 className='text-lg font-medium'>{t('WeChat Pay')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t('Official WeChat Pay API v3 QR-code payment.')}
        </p>
      </div>

      <Alert className='min-w-0'>
        <AlertTitle>{t('WeChat Pay configuration')}</AlertTitle>
        <AlertDescription className='min-w-0'>
          <p className='break-words'>
            {t(
              'Use the credentials issued for the same WeChat merchant account.'
            )}
          </p>
        </AlertDescription>
      </Alert>

      <FormField
        control={form.control}
        name='WechatEnabled'
        render={({ field }) => (
          <SettingsSwitchItem>
            <SettingsSwitchContent>
              <FormLabel>{t('Enable WeChat Pay')}</FormLabel>
              <FormDescription>
                {t('Show WeChat Pay for wallet and subscription checkout.')}
              </FormDescription>
            </SettingsSwitchContent>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </SettingsSwitchItem>
        )}
      />

      <div className='grid gap-5 md:grid-cols-2'>
        <FormField
          control={form.control}
          name='WechatMchID'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Merchant ID')}</FormLabel>
              <FormControl>
                <Input inputMode='numeric' autoComplete='off' {...field} />
              </FormControl>
              <FormDescription>
                {t('WeChat merchant number (mchid).')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='WechatAppID'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('AppID')}</FormLabel>
              <FormControl>
                <Input autoComplete='off' placeholder='wx...' {...field} />
              </FormControl>
              <FormDescription>
                {t('The appid bound to this merchant account.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='WechatCertificateSerialNumber'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Merchant certificate serial number')}</FormLabel>
              <FormControl>
                <Input
                  autoComplete='off'
                  placeholder={t('Uppercase hexadecimal value')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='WechatOrderExpireMinutes'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Order expiration (minutes)')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={1}
                  max={120}
                  step={1}
                  {...safeNumberFieldProps(field)}
                />
              </FormControl>
              <FormDescription>
                {t('Allowed range: 1 to 120 minutes.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='WechatUnitPrice'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Price (local currency / USD)')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={0.000001}
                  max={1_000_000}
                  step='0.01'
                  {...safeNumberFieldProps(field)}
                />
              </FormControl>
              <FormDescription>
                {t('e.g., 8 means 8 local currency per USD')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <div className='grid gap-5 md:grid-cols-2'>
        <FormField
          control={form.control}
          name='WechatApiV3Key'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('API v3 key')}</FormLabel>
              <FormControl>
                <Input
                  type='password'
                  autoComplete='new-password'
                  placeholder={t('Enter a new value to replace the saved key')}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t('Exactly 32 bytes; leave blank to keep the saved value.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='WechatPayPublicKeyID'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('WeChat Pay public key ID')}</FormLabel>
              <FormControl>
                <Input autoComplete='off' {...field} />
              </FormControl>
              <FormDescription>
                {t('Required when using the public-key verification mode.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <div className='grid gap-5 lg:grid-cols-2'>
        <FormField
          control={form.control}
          name='WechatPrivateKey'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Merchant RSA private key')}</FormLabel>
              <FormControl>
                <Textarea
                  rows={7}
                  className='font-mono text-xs'
                  autoComplete='off'
                  placeholder='-----BEGIN PRIVATE KEY-----'
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'PKCS#8 or PKCS#1 RSA private key. Leave blank to keep the saved value.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='WechatMerchantCertificate'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Merchant certificate')}</FormLabel>
              <FormControl>
                <Textarea
                  rows={7}
                  className='font-mono text-xs'
                  autoComplete='off'
                  placeholder='-----BEGIN CERTIFICATE-----'
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Optional certificate used to verify that the serial number and merchant private key match.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <div className='grid gap-5 lg:grid-cols-2'>
        <FormField
          control={form.control}
          name='WechatPayPublicKey'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('WeChat Pay public key')}</FormLabel>
              <FormControl>
                <Textarea
                  rows={7}
                  className='font-mono text-xs'
                  autoComplete='off'
                  placeholder='-----BEGIN PUBLIC KEY-----'
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Use this or the platform certificate for response and callback verification.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='WechatPlatformCertificate'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Platform certificate')}</FormLabel>
              <FormControl>
                <Textarea
                  rows={7}
                  className='font-mono text-xs'
                  autoComplete='off'
                  placeholder='-----BEGIN CERTIFICATE-----'
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Alternative to the WeChat Pay public key; keep it updated before expiry.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <FormField
        control={form.control}
        name='WechatNotifyURL'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Notification URL')}</FormLabel>
            <FormControl>
              <Input
                type='url'
                inputMode='url'
                autoComplete='url'
                placeholder={t('Leave blank to use the server callback URL')}
                {...field}
              />
            </FormControl>
            <FormDescription>
              {t(
                'Must be an HTTPS URL. The default is /api/wechat/notify on the configured server address.'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      {!enabled && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'Configuration is saved but the WeChat payment method remains hidden until enabled.'
          )}
        </p>
      )}
    </div>
  )
}
