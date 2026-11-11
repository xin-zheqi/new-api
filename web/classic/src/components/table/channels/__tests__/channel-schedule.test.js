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

import { expect, test } from 'vitest';
import {
  channelScheduleFormValues,
  serializeChannelSchedule,
} from '../modals/channelSchedule';

test('unrestricted new and cleared schedules omit the setting regardless of the unused timezone', () => {
  expect(channelScheduleFormValues()).toEqual({
    channel_schedule_timezone: 'Asia/Shanghai',
    channel_schedule_periods: [],
  });
  expect(serializeChannelSchedule('Invalid/unused', [])).toBeUndefined();
});

test('editing and saving preserves multiple daily and overnight ranges without mutating original settings', () => {
  const schedule = {
    timezone: 'Asia/Shanghai',
    periods: [
      { start: '09:00', end: '18:00' },
      { start: '22:00', end: '04:00' },
      { start: '23:00', end: '24:00' },
    ],
  };
  const values = channelScheduleFormValues(schedule);
  expect(
    serializeChannelSchedule(
      values.channel_schedule_timezone,
      values.channel_schedule_periods,
    ),
  ).toEqual(schedule);
  values.channel_schedule_periods[0].start = '10:00';
  expect(schedule.periods[0].start).toBe('09:00');
});

test('save trims inputs and canonicalizes a valid case-insensitive IANA zone for the backend', () => {
  expect(
    serializeChannelSchedule(' asia/shanghai ', [
      { start: ' 00:00 ', end: ' 24:00 ' },
    ]),
  ).toEqual({
    timezone: 'Asia/Shanghai',
    periods: [{ start: '00:00', end: '24:00' }],
  });
});

test.each(['', 'Not/AZone', '+01:00', '-0500', 'A'.repeat(129)])(
  'invalid timezone %j is rejected for configured ranges',
  (timezone) => {
    expect(() =>
      serializeChannelSchedule(timezone, [{ start: '09:00', end: '18:00' }]),
    ).toThrow('Enter a valid IANA time zone.');
  },
);

test.each([
  { start: '', end: '18:00' },
  { start: '9:00', end: '18:00' },
  { start: '24:00', end: '18:00' },
  { start: '09:00', end: '24:01' },
  { start: '09:00', end: '18:60' },
  { start: 900, end: '18:00' },
])(
  'invalid time range %j is rejected rather than silently changing active hours',
  (period) => {
    expect(() => serializeChannelSchedule('UTC', [period])).toThrow(
      'Time ranges must use HH:mm',
    );
  },
);

test('equal boundaries are rejected instead of ambiguously permitting all day', () => {
  expect(() =>
    serializeChannelSchedule('UTC', [{ start: '09:00', end: '09:00' }]),
  ).toThrow('Start and end times must differ.');
});

test('32 ranges are supported and oversized or non-array configurations are rejected', () => {
  const periods = Array.from({ length: 32 }, () => ({
    start: '09:00',
    end: '18:00',
  }));
  expect(serializeChannelSchedule('UTC', periods).periods).toHaveLength(32);
  expect(() =>
    serializeChannelSchedule('UTC', [...periods, periods[0]]),
  ).toThrow('Channel active hours support at most 32 time ranges.');
  expect(() => serializeChannelSchedule('UTC', {})).toThrow(
    'Channel active hours support at most 32 time ranges.',
  );
});
