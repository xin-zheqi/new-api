import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
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
import { assert, describe, expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import { CHANNEL_TYPE_NEW_API, CHANNEL_TYPE_SUB2API } from '../../constants'
import { CHANNEL_TYPE_ADVANCED_CUSTOM } from '../../lib/advanced-custom'
import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { channelSchema } from '../../types'
import { ToolOutputRepairSetting } from '../tool-output-repair-setting'

const supportedChannelTypes = [
  1,
  43,
  14,
  24,
  CHANNEL_TYPE_ADVANCED_CUSTOM,
  CHANNEL_TYPE_SUB2API,
  CHANNEL_TYPE_NEW_API,
]

function SettingsForm(props: {
  channelType: number
  disabled?: boolean
  enabled?: boolean
  onSave: (value: string) => void
}) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: props.channelType,
      tool_output_repair: props.enabled,
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(buildSettingJSON(values))
        )}
      >
        <ToolOutputRepairSetting disabled={props.disabled} />
        <button type='submit'>Save</button>
      </form>
    </Form>
  )
}

describe('Tool output repair channel setting', () => {
  describe.each(supportedChannelTypes)('channel type %s', (channelType) => {
    test.each([undefined, false, true])(
      'preserves setting %s through create, update and reload',
      (enabled) => {
        const channel = channelSchema.parse({
          id: 1,
          name: 'Test channel',
          key: '',
          type: channelType,
          status: 1,
          created_time: 0,
          test_time: 0,
          response_time: 0,
          balance_updated_time: 0,
          setting: JSON.stringify({ tool_output_repair: enabled }),
        })
        const values = transformChannelToFormDefaults(channel)
        const payloads = [
          transformFormDataToCreatePayload(values).channel,
          transformFormDataToUpdatePayload(values, channel.id),
        ]
        for (const payload of payloads) {
          assert(typeof payload.setting === 'string')
          expect(JSON.parse(payload.setting).tool_output_repair).toBe(
            enabled === true
          )
          expect(
            transformChannelToFormDefaults({
              ...channel,
              setting: payload.setting,
            }).tool_output_repair
          ).toBe(enabled === true)
        }
      }
    )
  })

  test('new channels default to tool output repair disabled', () => {
    expect(
      JSON.parse(buildSettingJSON(CHANNEL_FORM_DEFAULT_VALUES))
        .tool_output_repair
    ).toBe(false)
  })

  test.each(supportedChannelTypes)(
    'channel type %s exposes an accessible switch and saves on and off',
    async (channelType) => {
      const user = userEvent.setup()
      const saved: string[] = []
      render(
        <SettingsForm
          channelType={channelType}
          onSave={(value) => saved.push(value)}
        />
      )
      const toggle = screen.getByRole('switch', {
        name: 'Repair missing tool outputs',
      })
      expect(toggle).not.toBeChecked()
      await user.click(toggle)
      await user.click(screen.getByRole('button', { name: 'Save' }))
      expect(JSON.parse(saved[0]).tool_output_repair).toBe(true)
      toggle.focus()
      await user.keyboard(' ')
      await user.click(screen.getByRole('button', { name: 'Save' }))
      expect(JSON.parse(saved[1]).tool_output_repair).toBe(false)
    }
  )

  test('locked channel settings prevent toggling', async () => {
    const user = userEvent.setup()
    render(
      <SettingsForm channelType={1} disabled enabled onSave={() => undefined} />
    )
    const toggle = screen.getByRole('switch', {
      name: 'Repair missing tool outputs',
    })
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    await user.click(toggle)
    expect(toggle).toBeChecked()
  })
})
