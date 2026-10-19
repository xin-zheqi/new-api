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
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Card,
  Input,
  Select,
  Space,
  Switch,
  TextArea,
} from '@douyinfe/semi-ui';
import { IconPlusCircle } from '@douyinfe/semi-icons';
import { showError, showSuccess } from '../../helpers';
import {
  auditRequest,
  newRule,
  normalizePolicy,
  presets,
  regexFlagOptions,
} from './api';
import Matches from './Matches';
export default function Rules({ data, onSaved }) {
  const { t } = useTranslation();
  const maxRules = Math.min(data.max_rules || 20, 20);
  const [policy, setPolicy] = useState(data.policy);
  const [busy, setBusy] = useState(false);
  const [sample, setSample] = useState('');
  const [preview, setPreview] = useState(null);
  const update = (index, changes) =>
    setPolicy((current) => ({
      ...current,
      rules: current.rules.map((rule, i) =>
        i === index ? { ...rule, ...changes } : rule,
      ),
    }));
  const add = (preset = {}) =>
    setPolicy((current) => {
      if (current.rules.length >= maxRules) return current;
      return {
        ...current,
        rules: [...current.rules, { ...newRule(), ...preset }],
      };
    });
  const move = (from, to) =>
    setPolicy((current) => {
      const rules = [...current.rules];
      [rules[from], rules[to]] = [rules[to], rules[from]];
      return { ...current, rules };
    });
  const run = async (test = false) => {
    setBusy(true);
    try {
      const requestPolicy = normalizePolicy(policy);
      if (test) {
        setPreview(null);
        setPreview(
          await auditRequest('/test', 'POST', {
            policy: requestPolicy,
            text: sample,
          }),
        );
      } else {
        await auditRequest('/policy', 'PUT', requestPolicy);
        onSaved(requestPolicy);
        showSuccess(t('Saved'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className='flex flex-col gap-4'>
      <Banner
        type='info'
        description={t(
          'Rules use RE2 (no lookaround or backreferences). Matching is case-sensitive unless i is set; m = multiline, s = dot matches newline. Use ${1} or ${name} in replacements.',
        )}
      />
      <Banner
        type='info'
        description={t(
          'Block takes priority. Rules inspect original text; the first replacement wins overlapping matches. First match means once per text field. Test presets before enabling them.',
        )}
      />
      <Space>
        <Switch
          aria-label={t('Enable my content audit')}
          checked={policy.enabled}
          onChange={(enabled) => setPolicy({ ...policy, enabled })}
        />
        <span>{t('Enable my content audit')}</span>
      </Space>
      <div className='flex flex-col gap-3'>
        <div className='border-border/60 flex flex-col gap-2 border-t pt-3'>
          <div className='text-sm font-medium'>
            {t('Common privacy presets')}
          </div>
          <div className='text-muted-foreground text-sm'>
            {t('Choose a preset to add a ready-to-edit rule.')}
          </div>
          <Space wrap>
            {presets.map((preset) => (
              <Button
                key={preset.name}
                theme='light'
                disabled={policy.rules.length >= maxRules}
                onClick={() => add({ ...preset, name: t(preset.name) })}
              >
                {t(preset.name)}
              </Button>
            ))}
          </Space>
        </div>
      </div>
      {policy.rules.map((rule, index) => (
        <Card
          key={rule.id}
          title={t('Rule') + ' ' + (index + 1)}
          className='!rounded-xl'
        >
          <div className='grid gap-4 sm:grid-cols-2'>
            <label className='flex flex-col gap-2'>
              {t('Rule name')}
              <Input
                aria-label={t('Rule name')}
                value={rule.name}
                maxLength={128}
                onChange={(name) => update(index, { name })}
              />
            </label>
            <label className='flex flex-col gap-2'>
              {t('Action')}
              <Select
                aria-label={t('Action')}
                value={rule.action}
                onChange={(action) =>
                  update(index, {
                    action,
                    mode: action === 'replace' ? rule.mode : 'first',
                  })
                }
                optionList={['warn', 'replace', 'block'].map((value) => ({
                  value,
                  label: t(value),
                }))}
              />
            </label>
            <label className='flex flex-col gap-2'>
              {t('Regular expression')}
              <Input
                aria-label={t('Regular expression')}
                value={rule.pattern}
                maxLength={512}
                autoComplete='off'
                spellCheck={false}
                onChange={(pattern) => update(index, { pattern })}
              />
            </label>
            <label className='flex flex-col gap-2'>
              {t('Replacement')}
              <Input
                aria-label={t('Replacement')}
                value={rule.replacement}
                maxLength={512}
                disabled={rule.action !== 'replace'}
                autoComplete='off'
                onChange={(replacement) => update(index, { replacement })}
              />
            </label>
            <label className='flex flex-col gap-2'>
              {t('Matching flags')}
              <Select
                multiple
                aria-label={t('Matching flags')}
                value={rule.flags ? rule.flags.split('') : []}
                onChange={(flags) => {
                  const selected = Array.isArray(flags)
                    ? flags
                    : flags
                      ? [flags]
                      : [];
                  update(index, {
                    flags: ['i', 'm', 's']
                      .filter((flag) => selected.includes(flag))
                      .join(''),
                  });
                }}
                optionList={regexFlagOptions.map((option) => ({
                  value: option.value,
                  label: t(option.label),
                }))}
                placeholder={t('No flags')}
              />
              <span className='text-muted-foreground text-sm'>
                {t(
                  'Select any combination: i ignores letter case, m makes ^ and $ match each line, and s lets . match line breaks.',
                )}
              </span>
            </label>
            {rule.action === 'replace' && (
              <label className='flex flex-col gap-2'>
                {t('Match mode')}
                <Select
                  aria-label={t('Match mode')}
                  value={rule.mode}
                  onChange={(mode) => update(index, { mode })}
                  optionList={[
                    { value: 'all', label: t('All matches') },
                    { value: 'first', label: t('First match per text field') },
                  ]}
                />
              </label>
            )}
          </div>
          <Space wrap className='mt-4'>
            <Switch
              aria-label={t('Enabled')}
              checked={rule.enabled}
              onChange={(enabled) => update(index, { enabled })}
            />
            <span>{t('Enabled')}</span>
            <Button
              disabled={index === 0}
              onClick={() => move(index, index - 1)}
            >
              {t('Move up')}
            </Button>
            <Button
              disabled={index === policy.rules.length - 1}
              onClick={() => move(index, index + 1)}
            >
              {t('Move down')}
            </Button>
            <Button
              type='danger'
              onClick={() =>
                setPolicy({
                  ...policy,
                  rules: policy.rules.filter((entry) => entry.id !== rule.id),
                })
              }
            >
              {t('Delete')}
            </Button>
          </Space>
        </Card>
      ))}
      <Button
        block
        theme='outline'
        icon={<IconPlusCircle />}
        className='border-dashed'
        disabled={policy.rules.length >= maxRules}
        onClick={() => add()}
      >
        {t('Add rule')}
      </Button>
      <Button
        theme='solid'
        className='self-start'
        loading={busy}
        onClick={() => run()}
      >
        {t('Save rules')}
      </Button>
      <Card title={t('Test with synthetic text')} className='!rounded-xl'>
        <div className='flex flex-col gap-3'>
          <p>
            {t(
              'Do not upload real secrets. Preview runs on this server without saving a hit record or sending text upstream.',
            )}
          </p>
          <label>
            {t('Test text')}
            <TextArea
              aria-label={t('Test text')}
              autoComplete='off'
              maxLength={8192}
              value={sample}
              onChange={setSample}
            />
          </label>
          <Button
            className='self-start'
            loading={busy}
            onClick={() => run(true)}
          >
            {t('Test rules')}
          </Button>
          {preview && (
            <>
              <Matches result={preview.result} />
              <pre className='max-h-64 overflow-auto whitespace-pre-wrap break-all'>
                {preview.text}
              </pre>
            </>
          )}
        </div>
      </Card>
    </div>
  );
}
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
