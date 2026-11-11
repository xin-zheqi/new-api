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
import { zodResolver } from '@hookform/resolvers/zod'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { ChannelScheduleSetting } from '../channel-schedule-setting'

function ScheduleForm(props: {
  schedule?: ChannelFormValues['channel_schedule']
  disabled?: boolean
  onSave: (setting: string) => void
}) {
  const form = useForm<ChannelFormValues>({
    resolver: zodResolver(channelFormSchema),
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Schedule test',
      models: 'model',
      channel_schedule: props.schedule ?? {
        timezone: 'Asia/Shanghai',
        periods: [],
      },
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(buildSettingJSON(values))
        )}
      >
        <ChannelScheduleSetting disabled={props.disabled} />
        <button type='submit'>Save</button>
        <button
          type='button'
          onClick={() =>
            form.reset({
              ...form.getValues(),
              channel_schedule: {
                timezone: 'UTC',
                periods: [{ start: '22:00', end: '03:00' }],
              },
            })
          }
        >
          Reload
        </button>
      </form>
    </Form>
  )
}

describe('Channel active hours editor', () => {
  test('adding multiple daily and overnight ranges saves timezone and deleting all removes the restriction', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(<ScheduleForm onSave={(value) => saved.push(value)} />)
    const add = screen.getByRole('button', { name: 'Add time range' })
    await user.click(add)
    expect(saved).toHaveLength(0)
    await user.type(
      screen.getByRole('textbox', { name: 'Start time' }),
      '09:00'
    )
    await user.type(screen.getByRole('textbox', { name: 'End time' }), '18:00')
    await user.click(add)
    const second = within(
      screen.getByRole('group', { name: 'Channel active hours 2' })
    )
    await user.type(
      second.getByRole('textbox', { name: 'Start time' }),
      '23:00'
    )
    await user.type(second.getByRole('textbox', { name: 'End time' }), '02:00')
    await user.clear(screen.getByRole('combobox', { name: 'Time zone' }))
    await user.type(screen.getByRole('combobox', { name: 'Time zone' }), 'UTC')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[0]).channel_schedule).toEqual({
      timezone: 'UTC',
      periods: [
        { start: '09:00', end: '18:00' },
        { start: '23:00', end: '02:00' },
      ],
    })
    await user.click(
      screen.getAllByRole('button', { name: 'Remove time range' })[0]
    )
    await user.click(screen.getByRole('button', { name: 'Remove time range' }))
    expect(
      screen.queryByRole('textbox', { name: 'Start time' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[1])).not.toHaveProperty('channel_schedule')
  })

  test('equal endpoints and invalid timezone show field errors and prevent saving', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(
      <ScheduleForm
        schedule={{
          timezone: 'invalid',
          periods: [{ start: '09:00', end: '09:00' }],
        }}
        onSave={(value) => saved.push(value)}
      />
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      await screen.findByText('Enter a valid IANA time zone.')
    ).toBeVisible()
    expect(screen.getByText('Start and end times must differ.')).toBeVisible()
    expect(screen.getByRole('textbox', { name: 'End time' })).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(saved).toHaveLength(0)
  })

  test('an incomplete new range displays a time-format error', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(<ScheduleForm onSave={(value) => saved.push(value)} />)
    await user.click(screen.getByRole('button', { name: 'Add time range' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      (
        await screen.findAllByText(
          'Time ranges must use HH:mm (24:00 is allowed only as an end time).'
        )
      )[0]
    ).toBeVisible()
    expect(saved).toHaveLength(0)
  })

  test('locked channel settings disable zone, ranges and add/remove controls', async () => {
    const user = userEvent.setup()
    render(
      <ScheduleForm
        disabled
        schedule={{
          timezone: 'UTC',
          periods: [{ start: '00:00', end: '24:00' }],
        }}
        onSave={() => undefined}
      />
    )
    expect(screen.getByRole('combobox', { name: 'Time zone' })).toBeDisabled()
    expect(screen.getByRole('textbox', { name: 'Start time' })).toBeDisabled()
    expect(screen.getByRole('textbox', { name: 'End time' })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Add time range' })
    ).toBeDisabled()
    const remove = screen.getByRole('button', { name: 'Remove time range' })
    expect(remove).toBeDisabled()
    await user.click(remove)
    expect(screen.getByRole('textbox', { name: 'Start time' })).toHaveValue(
      '00:00'
    )
  })

  test('reloading defaults replaces both timezone and periods', async () => {
    const user = userEvent.setup()
    render(<ScheduleForm onSave={() => undefined} />)
    await user.click(screen.getByRole('button', { name: 'Reload' }))
    expect(screen.getByRole('combobox', { name: 'Time zone' })).toHaveValue(
      'UTC'
    )
    expect(screen.getByRole('textbox', { name: 'Start time' })).toHaveValue(
      '22:00'
    )
    expect(screen.getByRole('textbox', { name: 'End time' })).toHaveValue(
      '03:00'
    )
  })

  test('the 32-period limit disables adding while existing periods can still be removed', async () => {
    const user = userEvent.setup()
    render(
      <ScheduleForm
        schedule={{
          timezone: 'UTC',
          periods: Array.from({ length: 32 }, () => ({
            start: '09:00',
            end: '18:00',
          })),
        }}
        onSave={() => undefined}
      />
    )
    expect(
      screen.getByRole('button', { name: 'Add time range' })
    ).toBeDisabled()
    await user.click(
      screen.getAllByRole('button', { name: 'Remove time range' })[0]
    )
    expect(screen.getByRole('button', { name: 'Add time range' })).toBeEnabled()
  })
})
