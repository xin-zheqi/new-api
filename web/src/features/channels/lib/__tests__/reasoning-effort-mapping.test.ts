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
  name: 'Mapping test',
  models: 'model',
}

describe('Reasoning effort mapping form contract', () => {
  test('new and cleared mappings are omitted from settings', () => {
    expect(
      JSON.parse(buildSettingJSON(CHANNEL_FORM_DEFAULT_VALUES))
    ).not.toHaveProperty('reasoning_effort_mapping')
    const values = { ...validValues, reasoning_effort_mapping: [] }
    expect(JSON.parse(buildSettingJSON(values))).not.toHaveProperty(
      'reasoning_effort_mapping'
    )
  })

  test('create, edit and reload preserve arbitrary mappings and other settings', () => {
    const channel = channelSchema.parse({
      id: 1,
      name: 'Test channel',
      key: '',
      type: 1,
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      setting: JSON.stringify({
        reasoning_effort_mapping: {
          xhigh: 'max',
          High: 'custom-strength',
        },
        proxy: 'http://proxy.example:8080',
        reasoning_content_repair: true,
        tool_output_repair: true,
        system_prompt: 'Existing system prompt',
      }),
    })
    const values = transformChannelToFormDefaults(channel)
    expect(values.reasoning_effort_mapping).toEqual([
      { from: 'xhigh', to: 'max' },
      { from: 'High', to: 'custom-strength' },
    ])
    values.reasoning_effort_mapping?.push({ from: '__proto__', to: 'max' })
    for (const payload of [
      transformFormDataToCreatePayload(values).channel,
      transformFormDataToUpdatePayload(values, channel.id),
    ]) {
      const settings = JSON.parse(payload.setting as string)
      expect(settings.reasoning_effort_mapping).toEqual(
        JSON.parse('{"xhigh":"max","High":"custom-strength","__proto__":"max"}')
      )
      expect(settings).toMatchObject({
        proxy: 'http://proxy.example:8080',
        reasoning_content_repair: true,
        tool_output_repair: true,
        system_prompt: 'Existing system prompt',
      })
      expect(
        transformChannelToFormDefaults({ ...channel, setting: payload.setting })
          .reasoning_effort_mapping
      ).toEqual(values.reasoning_effort_mapping)
    }
  })

  test('validation trims values and keeps case-sensitive source keys distinct', () => {
    const result = channelFormSchema.parse({
      ...validValues,
      reasoning_effort_mapping: [
        { from: ' high ', to: ' max ' },
        { from: 'High', to: 'other' },
      ],
    })
    expect(result.reasoning_effort_mapping).toEqual([
      { from: 'high', to: 'max' },
      { from: 'High', to: 'other' },
    ])
  })

  test.each([
    [{ from: 'high', to: '' }],
    [{ from: ' ', to: 'max' }],
    [
      { from: 'high', to: 'max' },
      { from: ' high ', to: 'other' },
    ],
    [{ from: 'a'.repeat(65), to: 'max' }],
    [{ from: 'high', to: 'a'.repeat(65) }],
    [{ from: 'hi\u0000gh', to: 'max' }],
    [{ from: 'high', to: 'ma\nx' }],
  ])('invalid rows %j are rejected', (...rows) => {
    expect(
      channelFormSchema.safeParse({
        ...validValues,
        reasoning_effort_mapping: rows,
      }).success
    ).toBe(false)
  })

  test('the value limit counts Unicode characters instead of UTF-16 units', () => {
    expect(
      channelFormSchema.safeParse({
        ...validValues,
        reasoning_effort_mapping: [{ from: '😀'.repeat(64), to: 'max' }],
      }).success
    ).toBe(true)
  })

  test('more than 64 mapping entries is rejected', () => {
    expect(
      channelFormSchema.safeParse({
        ...validValues,
        reasoning_effort_mapping: Array.from({ length: 65 }, (_, index) => ({
          from: `effort-${index}`,
          to: 'max',
        })),
      }).success
    ).toBe(false)
  })

  test('mapping validation opens the advanced settings section', () => {
    expect(
      hasAdvancedSettingsErrors({
        reasoning_effort_mapping: [{ from: { message: 'Invalid mapping' } }],
      })
    ).toBe(true)
  })
})
