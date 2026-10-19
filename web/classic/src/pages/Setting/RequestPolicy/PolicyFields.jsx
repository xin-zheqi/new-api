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
import React, { useState } from 'react';
import { Button, Banner } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

// The server validates the complete policy. Only changed fields are submitted,
// so a section cannot overwrite settings edited in another browser tab.
export default function PolicyFields({ options, fields, onSave }) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const submit = async (event) => {
    event.preventDefault();
    if (saving) return;
    setSaving(true);
    setError('');
    try {
      const changes = Object.fromEntries(
        Object.entries(draft).filter(([key, value]) => value !== options[key]),
      );
      if (Object.keys(changes).length) await onSave(changes);
      setDraft({});
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };
  return (
    <form onSubmit={submit} className='space-y-4 max-w-3xl py-4'>
      <fieldset disabled={saving} className='space-y-4'>
        {fields.map(([key, label, type, choices]) => {
          const value = draft[key] ?? options[key] ?? '';
          const id = `policy-${key}`;
          const change = (next) => {
            const patch = { [key]: next };
            // The legacy flag is an alias for passive recovery. Keep it in sync
            // with the explicit mode so the old monitoring screen stays correct.
            if (key === 'monitor_setting.channel_test_mode') {
              patch['monitor_setting.auto_test_only_auto_disabled'] = String(
                next === 'passive_recovery',
              );
            }
            setDraft((current) => ({ ...current, ...patch }));
          };
          return (
            <div key={key} className='flex flex-col gap-2'>
              <label htmlFor={id}>{t(label)}</label>
              {type === 'checkbox' ? (
                <input
                  id={id}
                  type='checkbox'
                  checked={value === 'true'}
                  onChange={(event) => change(String(event.target.checked))}
                />
              ) : type === 'select' ? (
                <select
                  id={id}
                  className='bg-transparent border rounded p-2'
                  value={value}
                  onChange={(event) => change(event.target.value)}
                >
                  {choices.map(([option, text]) => (
                    <option key={option} value={option}>
                      {t(text)}
                    </option>
                  ))}
                </select>
              ) : type === 'textarea' ? (
                <textarea
                  id={id}
                  className='bg-transparent border rounded p-2'
                  rows={5}
                  value={value}
                  onChange={(event) => change(event.target.value)}
                />
              ) : (
                <input
                  id={id}
                  className='bg-transparent border rounded p-2'
                  type={type}
                  min={type === 'number' ? 0 : undefined}
                  step={
                    key === 'monitor_setting.auto_test_channel_minutes'
                      ? 'any'
                      : 1
                  }
                  value={value}
                  onChange={(event) => change(event.target.value)}
                />
              )}
            </div>
          );
        })}
      </fieldset>
      {error && (
        <div>
          <Banner type='danger' description={t(error)} closeIcon={null} />
        </div>
      )}
      <Button
        htmlType='submit'
        theme='solid'
        loading={saving}
        disabled={!Object.keys(draft).length}
      >
        {t('Save Changes')}
      </Button>
    </form>
  );
}
