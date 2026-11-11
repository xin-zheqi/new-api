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

import React, { useId } from 'react';
import { AutoComplete, Button, Input, Typography } from '@douyinfe/semi-ui';
import { IconArrowRight, IconDelete, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { MAX_CHANNEL_SCHEDULE_PERIODS } from './channelSchedule';

const TIMEZONES = [
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Singapore',
  'UTC',
  'Europe/London',
  'Europe/Berlin',
  'America/New_York',
  'America/Los_Angeles',
];

export default function ChannelScheduleEditor(props) {
  const { t } = useTranslation();
  const labelId = useId();
  const timezoneId = useId();
  const periods = props.periods || [];

  return (
    <section className='flex flex-col gap-2 py-2' aria-labelledby={labelId}>
      <div className='flex items-center justify-between gap-2'>
        <Typography.Text id={labelId} className='font-medium'>
          {t('Channel active hours')}
        </Typography.Text>
        <Button
          size='small'
          type='tertiary'
          htmlType='button'
          icon={<IconPlus aria-hidden='true' />}
          disabled={periods.length >= MAX_CHANNEL_SCHEDULE_PERIODS}
          onClick={() =>
            props.onPeriodsChange([...periods, { start: '', end: '' }])
          }
        >
          {t('Add time range')}
        </Button>
      </div>
      {periods.length > 0 && (
        <>
          <label
            id={timezoneId}
            className='text-sm text-[var(--semi-color-text-1)]'
          >
            {t('Time zone')}
          </label>
          <AutoComplete
            className='w-full'
            aria-labelledby={timezoneId}
            value={props.timezone}
            data={TIMEZONES.filter((zone) =>
              zone.toLowerCase().includes((props.timezone || '').toLowerCase()),
            )}
            onChange={props.onTimezoneChange}
            onSelect={props.onTimezoneChange}
          />
          {periods.map((period, index) => (
            <div className='flex min-w-0 items-center gap-2' key={index}>
              <Input
                className='min-w-0 flex-1'
                size='small'
                value={period.start}
                aria-label={t('Start time')}
                placeholder='09:00'
                maxLength={5}
                onChange={(value) =>
                  props.onPeriodsChange(
                    periods.map((item, i) =>
                      i === index ? { ...item, start: value } : item,
                    ),
                  )
                }
              />
              <IconArrowRight
                className='shrink-0 text-[var(--semi-color-text-2)]'
                aria-hidden='true'
              />
              <Input
                className='min-w-0 flex-1'
                size='small'
                value={period.end}
                aria-label={t('End time')}
                placeholder='18:00'
                maxLength={5}
                onChange={(value) =>
                  props.onPeriodsChange(
                    periods.map((item, i) =>
                      i === index ? { ...item, end: value } : item,
                    ),
                  )
                }
              />
              <Button
                className='shrink-0'
                size='small'
                type='tertiary'
                htmlType='button'
                icon={<IconDelete aria-hidden='true' />}
                aria-label={t('Remove time range')}
                onClick={() =>
                  props.onPeriodsChange(periods.filter((_, i) => i !== index))
                }
              />
            </div>
          ))}
        </>
      )}
      <Typography.Text type='tertiary' size='small'>
        {t(
          'Daily ranges use the selected time zone. Overnight ranges are supported; the end time is excluded. Leave empty for unrestricted hours. Manual and health disables remain in effect.',
        )}
      </Typography.Text>
    </section>
  );
}
