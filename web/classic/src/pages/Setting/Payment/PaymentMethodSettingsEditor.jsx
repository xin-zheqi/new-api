/*
Copyright (C) 2025 QuantumNous

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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Button, Input, Switch, Tag, Typography } from '@douyinfe/semi-ui';
import { RotateCcw } from 'lucide-react';
import {
  getPaymentMethodDisplayName,
  getPaymentMethodDefinitions,
  getPaymentMethodSetting,
  isPaymentMethodDefaultName,
  normalizePaymentMethodType,
  parsePaymentMethodSettings,
} from '../../../components/topup/paymentMethodSettings';

const { Text } = Typography;

export default function PaymentMethodSettingsEditor({
  value,
  epayMethodsValue,
  onChange,
  t,
}) {
  const overrides = useMemo(() => parsePaymentMethodSettings(value), [value]);
  const definitions = useMemo(
    () => getPaymentMethodDefinitions(epayMethodsValue),
    [epayMethodsValue],
  );

  const getDefinitionDisplayName = (definition) => {
    if (
      definition.configuredName &&
      !isPaymentMethodDefaultName(definition.type, definition.configuredName, t)
    ) {
      return definition.configuredName;
    }
    return t(definition.nameKey || definition.defaultName);
  };

  // Keep the input independent from the serialized overlay. Removing an
  // override while the administrator is typing must not immediately replace
  // an empty value with the translated default label.
  const [draftNames, setDraftNames] = useState({});
  const lastEmittedValue = useRef(null);

  useEffect(() => {
    if (lastEmittedValue.current === value) {
      lastEmittedValue.current = null;
      return;
    }

    const nextDraftNames = {};
    definitions.forEach((definition) => {
      const defaultName = getDefinitionDisplayName(definition);
      nextDraftNames[definition.type] = getPaymentMethodDisplayName(
        definition.type,
        defaultName,
        overrides,
        t,
      );
    });
    setDraftNames(nextDraftNames);
  }, [definitions, overrides, t, value]);

  const updateEntry = (definition, patch) => {
    const current = getPaymentMethodSetting(definition.type, overrides);
    const defaultName = getDefinitionDisplayName(definition);
    const editingName = patch.name !== undefined;
    const draftName = draftNames[definition.type];
    const nextName = editingName
      ? patch.name
      : (draftName ?? current?.name ?? defaultName);
    const nextEnabled = patch.enabled ?? current?.enabled ?? true;
    const next = overrides.filter(
      (entry) =>
        normalizePaymentMethodType(entry.type) !==
        normalizePaymentMethodType(definition.type),
    );
    const normalizedName = (nextName || '').trim();
    if (editingName) {
      setDraftNames((previous) => ({
        ...previous,
        [definition.type]: nextName,
      }));
    }
    // Empty input restores the default label. Do not send an invalid empty
    // name to the server; a disabled method still needs a valid label.
    if (!normalizedName) {
      if (!nextEnabled) {
        next.push({
          type: definition.type,
          name: definition.defaultName,
          enabled: false,
        });
      }
      const nextValue = JSON.stringify(next, null, 2);
      lastEmittedValue.current = nextValue;
      onChange(nextValue);
      return;
    }
    const usesDefaultName = isPaymentMethodDefaultName(
      definition.type,
      normalizedName,
      t,
    );

    // Store only real overrides. This keeps the option small and lets a
    // language change update built-in labels without rewriting the setting.
    if (!usesDefaultName || !nextEnabled) {
      next.push({
        type: definition.type,
        name: usesDefaultName ? definition.defaultName : normalizedName,
        enabled: nextEnabled,
      });
    }
    const nextValue = JSON.stringify(next, null, 2);
    lastEmittedValue.current = nextValue;
    onChange(nextValue);
  };

  const resetEntry = (definition) => {
    const next = overrides.filter(
      (entry) =>
        normalizePaymentMethodType(entry.type) !==
        normalizePaymentMethodType(definition.type),
    );
    setDraftNames((previous) => ({
      ...previous,
      [definition.type]: getDefinitionDisplayName(definition),
    }));
    const nextValue = JSON.stringify(next, null, 2);
    lastEmittedValue.current = nextValue;
    onChange(nextValue);
  };

  return (
    <div
      style={{
        border: '1px solid var(--semi-color-border)',
        borderRadius: 6,
        padding: 12,
      }}
    >
      <div style={{ marginBottom: 12 }}>
        <Text type='tertiary' size='small'>
          {t('统一控制所有支付方式的启用状态和显示名称')}
        </Text>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {definitions.map((definition) => {
          const override = getPaymentMethodSetting(definition.type, overrides);
          const defaultName = getDefinitionDisplayName(definition);
          const name = getPaymentMethodDisplayName(
            definition.type,
            defaultName,
            overrides,
            t,
          );
          const draftName = draftNames[definition.type] ?? name;
          const enabled = override?.enabled !== false;
          const changed = Boolean(override);

          return (
            <div
              key={definition.type}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                flexWrap: 'wrap',
                padding: '8px 0',
                borderBottom: '1px solid var(--semi-color-border)',
              }}
            >
              <div style={{ minWidth: 150, flex: '1 1 180px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <Text strong>{draftName || defaultName}</Text>
                  <Tag size='small' color='grey'>
                    {definition.type}
                  </Tag>
                </div>
                <Text type='tertiary' size='small'>
                  {t(definition.gateway)}
                </Text>
              </div>
              <Input
                value={draftName}
                maxLength={128}
                placeholder={defaultName}
                aria-label={`${t('显示名称')} ${draftName || defaultName}`}
                onChange={(nextName) =>
                  updateEntry(definition, { name: nextName })
                }
                style={{ flex: '1 1 200px', minWidth: 180 }}
              />
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <Switch
                  checked={enabled}
                  size='small'
                  aria-label={`${t('启用')} ${draftName || defaultName}`}
                  onChange={(checked) =>
                    updateEntry(definition, { enabled: checked })
                  }
                />
                <Text type='tertiary' size='small'>
                  {enabled ? t('已启用') : t('已禁用')}
                </Text>
                {changed && (
                  <Button
                    type='tertiary'
                    theme='borderless'
                    size='small'
                    icon={<RotateCcw size={14} />}
                    aria-label={t('重置为默认')}
                    onClick={() => resetEntry(definition)}
                  />
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
