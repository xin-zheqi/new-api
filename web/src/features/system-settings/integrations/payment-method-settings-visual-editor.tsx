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
import { Settings2 } from 'lucide-react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  getPaymentMethodDisplayName,
  getPaymentMethodSetting,
  isPaymentMethodDefaultName,
  normalizePaymentMethodType,
  parsePaymentMethodSettings,
} from '@/features/wallet/lib/payment-methods'

type PaymentMethodSettingsVisualEditorProps = {
  value: string
  epayMethodsValue: string
  onChange: (value: string) => void
}

type PaymentMethodSettingEntry = {
  type: string
  name: string
  enabled: boolean
}

type PaymentMethodDefinition = {
  type: string
  defaultName: string
  gateway: string
  nameKey?: string
  configuredName?: string
}

const PAYMENT_METHOD_DEFINITIONS: PaymentMethodDefinition[] = [
  { type: 'alipay', defaultName: 'Alipay', gateway: 'Epay', nameKey: 'Alipay' },
  {
    type: 'wxpay',
    defaultName: 'WeChat Pay',
    gateway: 'Epay',
    nameKey: 'WeChat Pay',
  },
  { type: 'stripe', defaultName: 'Stripe', gateway: 'Stripe' },
  { type: 'creem', defaultName: 'Creem', gateway: 'Creem' },
  {
    type: 'wechat_native_v3',
    defaultName: 'WeChat Pay',
    gateway: 'WeChat Pay',
    nameKey: 'WeChat Pay',
  },
  {
    type: 'waffo',
    defaultName: 'Waffo Payment',
    gateway: 'Waffo',
    nameKey: 'Waffo Payment',
  },
  {
    type: 'waffo_pancake',
    defaultName: 'Waffo Pancake',
    gateway: 'Waffo Pancake',
  },
]

function parseEpayDefinitions(value: string): PaymentMethodDefinition[] {
  try {
    const parsed: unknown = JSON.parse(value || '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.flatMap((item) => {
      if (!item || typeof item !== 'object') return []
      const record = item as Record<string, unknown>
      const type = typeof record.type === 'string' ? record.type.trim() : ''
      const name = typeof record.name === 'string' ? record.name.trim() : ''
      if (
        !type ||
        !name ||
        type.length > 64 ||
        [...type].some((character) => {
          const codePoint = character.codePointAt(0) ?? 0
          return codePoint < 0x21 || codePoint > 0x7e
        }) ||
        [...name].length > 128 ||
        [...name].some((character) => /\p{Cc}/u.test(character)) ||
        type === 'waffo' ||
        type.startsWith('waffo:')
      ) {
        return []
      }
      return [
        {
          type,
          defaultName: name,
          configuredName: name,
          gateway: 'Epay',
        },
      ]
    })
  } catch {
    return []
  }
}

export function PaymentMethodSettingsVisualEditor({
  value,
  epayMethodsValue,
  onChange,
}: PaymentMethodSettingsVisualEditorProps) {
  const { t } = useTranslation()
  const overrides = React.useMemo(
    () => parsePaymentMethodSettings(value),
    [value]
  )
  const definitions = React.useMemo(() => {
    const definitions = PAYMENT_METHOD_DEFINITIONS.map((definition) => ({
      ...definition,
    }))
    const byType = new Map(
      definitions.map((definition) => [definition.type, definition])
    )
    for (const definition of parseEpayDefinitions(epayMethodsValue)) {
      const existing = byType.get(definition.type)
      if (existing) {
        if (existing.gateway === 'Epay') {
          existing.configuredName = definition.defaultName
        }
        continue
      }
      definitions.push(definition)
      byType.set(definition.type, definition)
    }
    return definitions
  }, [epayMethodsValue])

  const [draftNames, setDraftNames] = React.useState<Record<string, string>>({})
  const lastEmittedValue = React.useRef<string | null>(null)

  const getDefinitionName = React.useCallback(
    (definition: PaymentMethodDefinition) => {
      if (
        definition.configuredName &&
        !isPaymentMethodDefaultName(
          definition.type,
          definition.configuredName,
          t
        )
      ) {
        return definition.configuredName
      }
      return definition.nameKey ? t(definition.nameKey) : definition.defaultName
    },
    [t]
  )

  React.useEffect(() => {
    // Keep an in-progress edit intact when the parent echoes our own JSON
    // value. A different value means the form was refreshed externally.
    if (lastEmittedValue.current === value) {
      lastEmittedValue.current = null
      return
    }

    const nextDrafts: Record<string, string> = {}
    for (const definition of definitions) {
      const defaultName = getDefinitionName(definition)
      nextDrafts[definition.type] = getPaymentMethodDisplayName(
        definition.type,
        defaultName,
        overrides,
        t
      )
    }
    setDraftNames(nextDrafts)
  }, [definitions, getDefinitionName, overrides, t, value])

  const updateEntry = (
    definition: PaymentMethodDefinition,
    patch: Partial<PaymentMethodSettingEntry>
  ) => {
    const current = getPaymentMethodSetting(definition.type, overrides)
    const defaultDisplayName = getDefinitionName(definition)
    const canonicalName = definition.defaultName
    const editingName = patch.name !== undefined
    const nextName =
      patch.name ??
      draftNames[definition.type] ??
      current?.name ??
      defaultDisplayName
    const nextEnabled = patch.enabled ?? current?.enabled ?? true
    const nextOverrides = overrides.filter(
      (entry) =>
        normalizePaymentMethodType(entry.type) !==
        normalizePaymentMethodType(definition.type)
    )
    // Toggling a method while its field is temporarily empty should still
    // persist a valid label; an actual name edit is allowed to remain empty
    // until the administrator finishes typing.
    const normalizedName = (
      editingName ? nextName : nextName || defaultDisplayName
    ).trim()
    if (editingName) {
      setDraftNames((previous) => ({
        ...previous,
        [definition.type]: nextName,
      }))
    } else if (!nextName.trim()) {
      setDraftNames((previous) => ({
        ...previous,
        [definition.type]: defaultDisplayName,
      }))
    }
    // An empty label means "restore the default". Never emit an invalid
    // empty name because the server deliberately rejects such settings.
    if (normalizedName === '') {
      if (!nextEnabled) {
        nextOverrides.push({
          type: definition.type,
          name: canonicalName,
          enabled: false,
        })
      }
      const nextValue = JSON.stringify(nextOverrides, null, 2)
      lastEmittedValue.current = nextValue
      onChange(nextValue)
      return
    }
    const usesDefaultName = isPaymentMethodDefaultName(
      definition.type,
      normalizedName,
      t
    )

    // Keep the option an overlay: unchanged methods do not get duplicated into
    // the new setting, so clearing the field restores the gateway default.
    if (!usesDefaultName || !nextEnabled) {
      nextOverrides.push({
        type: definition.type,
        name: usesDefaultName ? canonicalName : normalizedName,
        enabled: nextEnabled,
      })
    }
    const nextValue = JSON.stringify(nextOverrides, null, 2)
    lastEmittedValue.current = nextValue
    onChange(nextValue)
  }

  return (
    <div className='space-y-3 rounded-md border p-3'>
      <div className='flex items-start gap-2'>
        <Settings2 className='text-muted-foreground mt-0.5 h-4 w-4 shrink-0' />
        <p className='text-muted-foreground text-sm'>
          {t(
            'Control which configured payment gateways users can use and choose the name shown in the wallet.'
          )}
        </p>
      </div>
      <div className='divide-y rounded-md border'>
        {definitions.map((definition) => {
          const override = getPaymentMethodSetting(definition.type, overrides)
          const defaultName = getDefinitionName(definition)
          const resolvedName = getPaymentMethodDisplayName(
            definition.type,
            defaultName,
            overrides,
            t
          )
          const name = draftNames[definition.type] ?? resolvedName
          const headingName = name || defaultName
          const enabled = override?.enabled ?? true
          return (
            <div
              key={definition.type}
              className='flex flex-col gap-3 p-3 sm:flex-row sm:items-center'
            >
              <div className='min-w-0 flex-1'>
                <div className='flex flex-wrap items-center gap-2'>
                  <span className='font-medium'>{headingName}</span>
                  <code className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-xs'>
                    {definition.type}
                  </code>
                </div>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('Payment Gateway')}: {definition.gateway}
                </p>
              </div>
              <div className='flex w-full items-center gap-3 sm:w-auto'>
                <Input
                  aria-label={`${t('Display name for this payment method.')} ${headingName}`}
                  value={name}
                  maxLength={128}
                  onChange={(event) =>
                    updateEntry(definition, { name: event.target.value })
                  }
                  className='min-w-0 flex-1 sm:w-52 sm:flex-none'
                />
                <div className='text-muted-foreground flex shrink-0 items-center gap-2 text-sm'>
                  <Switch
                    checked={enabled}
                    aria-label={t('Enable {{parameter}}', {
                      parameter: headingName,
                    })}
                    onCheckedChange={(checked) =>
                      updateEntry(definition, { enabled: checked })
                    }
                  />
                  {enabled ? t('Enabled') : t('Disabled')}
                </div>
              </div>
            </div>
          )
        })}
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'These switches control whether configured gateways are available to users. Disabled gateways are also rejected by the server for new checkout requests.'
        )}
      </p>
    </div>
  )
}
