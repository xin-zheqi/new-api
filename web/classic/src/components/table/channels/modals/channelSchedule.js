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

export const MAX_CHANNEL_SCHEDULE_PERIODS = 32;
export const DEFAULT_CHANNEL_SCHEDULE_TIMEZONE = 'Asia/Shanghai';

export function channelScheduleFormValues(schedule) {
  return {
    channel_schedule_timezone:
      typeof schedule?.timezone === 'string'
        ? schedule.timezone
        : DEFAULT_CHANNEL_SCHEDULE_TIMEZONE,
    channel_schedule_periods: Array.isArray(schedule?.periods)
      ? schedule.periods.map((period) => ({
          start: typeof period?.start === 'string' ? period.start : '',
          end: typeof period?.end === 'string' ? period.end : '',
        }))
      : [],
  };
}

// An empty list removes the restriction entirely. Validation runs only while
// saving, never during relay traffic or channel list rendering.
export function serializeChannelSchedule(timezone, periods = []) {
  if (
    !Array.isArray(periods) ||
    periods.length > MAX_CHANNEL_SCHEDULE_PERIODS
  ) {
    throw new Error('Channel active hours support at most 32 time ranges.');
  }
  if (periods.length === 0) return undefined;
  const zone = typeof timezone === 'string' ? timezone.trim() : '';
  let canonicalZone;
  try {
    if (zone.length > 128 || !/^[A-Za-z][A-Za-z0-9_+\/-]*$/.test(zone))
      throw new Error();
    canonicalZone = new Intl.DateTimeFormat('en-US', {
      timeZone: zone,
    }).resolvedOptions().timeZone;
  } catch {
    throw new Error('Enter a valid IANA time zone.');
  }
  const normalized = periods.map((period) => {
    const start = typeof period?.start === 'string' ? period.start.trim() : '';
    const end = typeof period?.end === 'string' ? period.end.trim() : '';
    if (
      !/^([01]\d|2[0-3]):[0-5]\d$/.test(start) ||
      (!/^([01]\d|2[0-3]):[0-5]\d$/.test(end) && end !== '24:00')
    ) {
      throw new Error(
        'Time ranges must use HH:mm (24:00 is allowed only as an end time).',
      );
    }
    if (start === end) throw new Error('Start and end times must differ.');
    return { start, end };
  });
  return { timezone: canonicalZone, periods: normalized };
}
