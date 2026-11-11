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
import { describe, expect, test } from 'vitest'

import { channelSchema } from '../../types'
import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'
import { hasAdvancedSettingsErrors } from '../channel-form-errors'

const validValues = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'Scheduled channel',
  models: 'model',
}

describe('Channel active hours persistence and validation', () => {
  test('a legacy channel or empty periods omit schedule even with an unused invalid zone', () => {
    expect(
      JSON.parse(buildSettingJSON(CHANNEL_FORM_DEFAULT_VALUES))
    ).not.toHaveProperty('channel_schedule')
    const values = {
      ...validValues,
      channel_schedule: { timezone: 'unused', periods: [] },
    }
    expect(channelFormSchema.safeParse(values).success).toBe(true)
    expect(JSON.parse(buildSettingJSON(values))).not.toHaveProperty(
      'channel_schedule'
    )
  })

  test('create, edit and reload preserve timezone, multiple ranges and other settings', () => {
    const schedule = {
      timezone: 'America/New_York',
      periods: [
        { start: '09:00', end: '18:00' },
        { start: '22:00', end: '03:00' },
      ],
    }
    const channel = channelSchema.parse({
      id: 1,
      name: 'Fixture',
      type: 1,
      key: '',
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'model',
      setting: JSON.stringify({
        channel_schedule: schedule,
        reasoning_effort_mapping: { xhigh: 'max' },
        reasoning_content_repair: true,
        tool_output_repair: true,
        http_protocol: 'http1',
      }),
    })
    const values = transformChannelToFormDefaults(channel)
    expect(values.channel_schedule).toEqual(schedule)
    for (const payload of [
      transformFormDataToCreatePayload(values).channel,
      transformFormDataToUpdatePayload(values, channel.id),
    ]) {
      expect(JSON.parse(payload.setting as string)).toMatchObject({
        channel_schedule: schedule,
        reasoning_effort_mapping: { xhigh: 'max' },
        reasoning_content_repair: true,
        tool_output_repair: true,
        http_protocol: 'http1',
      })
      expect(
        transformChannelToFormDefaults({ ...channel, setting: payload.setting })
          .channel_schedule
      ).toEqual(schedule)
      expect(payload).not.toHaveProperty('channel_schedule')
      expect(payload).not.toHaveProperty('schedule_active')
    }
  })

  test('timezone aliases are canonicalized before sending to the server', () => {
    const setting = JSON.parse(
      buildSettingJSON({
        ...validValues,
        channel_schedule: {
          timezone: ' asia/shanghai ',
          periods: [{ start: '09:00', end: '18:00' }],
        },
      })
    )
    expect(setting.channel_schedule.timezone).toBe('Asia/Shanghai')
  })

  test.each([
    'Mars/Base',
    '',
    '+08:00',
    'Local',
    'Asia/Shanghai\ninvalid',
    'a'.repeat(129),
  ])('configured invalid zone %j cannot be saved', (timezone) => {
    expect(
      channelFormSchema.safeParse({
        ...validValues,
        channel_schedule: {
          timezone,
          periods: [{ start: '09:00', end: '18:00' }],
        },
      }).success
    ).toBe(false)
  })

  test.each([
    { start: '', end: '18:00' },
    { start: '9:00', end: '18:00' },
    { start: '24:00', end: '02:00' },
    { start: '09:60', end: '18:00' },
    { start: '09:00', end: '24:01' },
    { start: '09:00', end: '09:00' },
  ])('invalid range %j blocks saving the channel', (period) => {
    expect(
      channelFormSchema.safeParse({
        ...validValues,
        channel_schedule: { timezone: 'UTC', periods: [period] },
      }).success
    ).toBe(false)
  })

  test('a full day, overnight range and 32 configured periods are valid', () => {
    for (const periods of [
      [{ start: '00:00', end: '24:00' }],
      [{ start: '23:00', end: '02:00' }],
      Array.from({ length: 32 }, () => ({ start: '09:00', end: '18:00' })),
    ]) {
      expect(
        channelFormSchema.safeParse({
          ...validValues,
          channel_schedule: { timezone: 'UTC', periods },
        }).success
      ).toBe(true)
    }
    expect(
      channelFormSchema.safeParse({
        ...validValues,
        channel_schedule: {
          timezone: 'UTC',
          periods: Array.from({ length: 33 }, () => ({
            start: '09:00',
            end: '18:00',
          })),
        },
      }).success
    ).toBe(false)
  })

  test('schedule errors open the advanced settings section and read-only active state stays separate', () => {
    expect(
      hasAdvancedSettingsErrors({
        channel_schedule: { message: 'Invalid range' },
      })
    ).toBe(true)
    expect(
      channelSchema.parse({
        id: 1,
        type: 1,
        key: '',
        status: 1,
        name: 'Scheduled',
        created_time: 0,
        test_time: 0,
        response_time: 0,
        balance_updated_time: 0,
        schedule_active: false,
      })
    ).toMatchObject({ status: 1, schedule_active: false })
  })
})
