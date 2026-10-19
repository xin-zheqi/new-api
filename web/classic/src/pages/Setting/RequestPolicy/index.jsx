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
import React, { useCallback, useEffect, useState } from 'react';
import { Banner, Button, Spin, Tabs, TabPane } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers/api';
import SettingsChannelAffinity from '../Operation/SettingsChannelAffinity';
import PolicyFields from './PolicyFields';

const sections = [
  [
    'retry',
    'Retry policy',
    [
      ['RetryTimes', 'Retry times', 'number'],
      ['AutomaticRetryStatusCodes', 'Retry status codes', 'text'],
    ],
  ],
  [
    'checks',
    'Request checks',
    [
      ['CheckSensitiveEnabled', 'Enable sensitive word filtering', 'checkbox'],
      ['CheckSensitiveOnPromptEnabled', 'Enable prompt checking', 'checkbox'],
      ['SensitiveWords', 'Sensitive words', 'textarea'],
    ],
  ],
  [
    'health',
    'Channel health',
    [
      ['AutomaticDisableChannelEnabled', 'Auto-disable channels', 'checkbox'],
      ['AutomaticEnableChannelEnabled', 'Auto-enable channels', 'checkbox'],
      ['AutomaticDisableStatusCodes', 'Auto-disable status codes', 'text'],
      ['AutomaticDisableKeywords', 'Auto-disable keywords', 'textarea'],
      ['ChannelDisableThreshold', 'Channel disable threshold', 'number'],
      [
        'monitor_setting.auto_test_channel_enabled',
        'Enable scheduled channel tests',
        'checkbox',
      ],
      [
        'monitor_setting.auto_test_channel_minutes',
        'Channel test interval (minutes)',
        'number',
      ],
      [
        'monitor_setting.auto_test_channel_time_range',
        'Channel test time range',
        'text',
      ],
      [
        'monitor_setting.channel_test_concurrency',
        'Channel test concurrency',
        'number',
      ],
      [
        'monitor_setting.channel_test_mode',
        'Channel test mode',
        'select',
        [
          ['scheduled_all', 'Scheduled testing'],
          ['auto_ban_only', 'Test automatically disabled channels'],
          ['passive_recovery', 'Passive recovery'],
        ],
      ],
    ],
  ],
];

export default function RequestPolicySetting() {
  const { t } = useTranslation();
  const [options, setOptions] = useState(null);
  const [error, setError] = useState('');
  const [active, setActive] = useState('routing');
  const refresh = useCallback(async () => {
    setError('');
    try {
      const { data } = await API.get('/api/option/request_policy');
      if (!data.success)
        throw new Error(data.message || 'Failed to load settings');
      setOptions(data.data.options);
    } catch (err) {
      setError(err.message);
    }
  }, []);
  useEffect(() => {
    void refresh();
  }, [refresh]);
  const save = async (changes) => {
    const { data } = await API.patch('/api/option/request_policy', {
      options: changes,
    });
    if (!data.success)
      throw new Error(data.message || 'Failed to update settings');
    setOptions(data.data.options);
  };
  if (error)
    return (
      <div>
        <Banner type='danger' description={t(error)} />
        <Button onClick={refresh}>{t('Retry')}</Button>
      </div>
    );
  if (!options) return <Spin />;
  return (
    <div className='p-4'>
      <Tabs activeKey={active} onChange={setActive}>
        <TabPane itemKey='routing' tab={t('Session routing')}>
          {active === 'routing' && (
            <SettingsChannelAffinity options={options} refresh={refresh} />
          )}
        </TabPane>
        {sections.map(([key, title, fields]) => (
          <TabPane key={key} itemKey={key} tab={t(title)}>
            {active === key && (
              <PolicyFields
                key={key}
                options={options}
                fields={fields}
                onSave={save}
              />
            )}
          </TabPane>
        ))}
      </Tabs>
    </div>
  );
}
