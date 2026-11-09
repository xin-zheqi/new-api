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
  MAX_REASONING_EFFORT_MAPPINGS,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { ReasoningEffortMappingSetting } from '../reasoning-effort-mapping-setting'

function MappingForm(props: {
  disabled?: boolean
  mappings?: ChannelFormValues['reasoning_effort_mapping']
  onSave: (value: string) => void
}) {
  const form = useForm<ChannelFormValues>({
    resolver: zodResolver(channelFormSchema),
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Mapping test',
      models: 'model',
      reasoning_effort_mapping: props.mappings ?? [],
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(buildSettingJSON(values))
        )}
      >
        <ReasoningEffortMappingSetting disabled={props.disabled} />
        <button type='submit'>Save</button>
        <button
          type='button'
          onClick={() =>
            form.reset({
              ...form.getValues(),
              reasoning_effort_mapping: [{ from: 'high', to: 'max' }],
            })
          }
        >
          Reload
        </button>
      </form>
    </Form>
  )
}

describe('Reasoning effort mapping editor', () => {
  test('adding, editing and removing rows only saves the configured mappings', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(<MappingForm onSave={(value) => saved.push(value)} />)

    const addButton = screen.getByRole('button', { name: 'Add mapping' })
    await user.click(addButton)
    expect(saved).toHaveLength(0)
    await user.type(
      screen.getByRole('textbox', { name: 'Before mapping' }),
      ' xhigh '
    )
    await user.type(
      screen.getByRole('textbox', { name: 'After mapping' }),
      ' max '
    )
    await user.click(addButton)
    const secondRow = screen.getByRole('group', {
      name: 'Reasoning effort mapping 2',
    })
    await user.type(
      within(secondRow).getByRole('textbox', { name: 'Before mapping' }),
      'low'
    )
    await user.type(
      within(secondRow).getByRole('textbox', { name: 'After mapping' }),
      'medium'
    )
    await user.click(
      within(secondRow).getByRole('button', { name: 'Remove mapping' })
    )
    expect(saved).toHaveLength(0)

    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[0]).reasoning_effort_mapping).toEqual({
      xhigh: 'max',
    })
    await user.click(screen.getByRole('button', { name: 'Remove mapping' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[1])).not.toHaveProperty('reasoning_effort_mapping')
  })

  test('incomplete rows show a field error and prevent saving', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(
      <MappingForm
        mappings={[{ from: 'xhigh', to: '' }]}
        onSave={(value) => saved.push(value)}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      await screen.findByText('Reasoning effort mappings require both values.')
    ).toBeVisible()
    expect(
      screen.getByRole('textbox', { name: 'After mapping' })
    ).toHaveAttribute('aria-invalid', 'true')
    expect(saved).toHaveLength(0)
  })

  test('duplicate sources show an error on the duplicate row', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(
      <MappingForm
        mappings={[
          { from: 'high', to: 'max' },
          { from: ' high ', to: 'low' },
        ]}
        onSave={(value) => saved.push(value)}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      await screen.findByText(
        'Reasoning effort mapping sources must be unique.'
      )
    ).toBeVisible()
    expect(
      screen.getAllByRole('textbox', { name: 'Before mapping' })[1]
    ).toHaveAttribute('aria-invalid', 'true')
    expect(saved).toHaveLength(0)
  })

  test('locked settings prevent adding, editing and removing mappings', async () => {
    const user = userEvent.setup()
    render(
      <MappingForm
        disabled
        mappings={[{ from: 'high', to: 'max' }]}
        onSave={() => undefined}
      />
    )

    expect(
      screen.getByRole('textbox', { name: 'Before mapping' })
    ).toBeDisabled()
    expect(
      screen.getByRole('textbox', { name: 'After mapping' })
    ).toBeDisabled()
    const add = screen.getByRole('button', { name: 'Add mapping' })
    const remove = screen.getByRole('button', { name: 'Remove mapping' })
    expect(add).toBeDisabled()
    expect(remove).toBeDisabled()
    await user.click(remove)
    expect(screen.getByRole('textbox', { name: 'Before mapping' })).toHaveValue(
      'high'
    )
  })

  test('loading new form defaults replaces the editor rows', async () => {
    const user = userEvent.setup()
    render(
      <MappingForm
        mappings={[
          { from: 'low', to: 'medium' },
          { from: 'xhigh', to: 'max' },
        ]}
        onSave={() => undefined}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Reload' }))
    expect(screen.getByRole('textbox', { name: 'Before mapping' })).toHaveValue(
      'high'
    )
    expect(screen.getByRole('textbox', { name: 'After mapping' })).toHaveValue(
      'max'
    )
  })

  test('the configured limit prevents adding another mapping', () => {
    render(
      <MappingForm
        mappings={Array.from(
          { length: MAX_REASONING_EFFORT_MAPPINGS },
          (_, index) => ({ from: `effort-${index}`, to: 'max' })
        )}
        onSave={() => undefined}
      />
    )
    expect(screen.getByRole('button', { name: 'Add mapping' })).toBeDisabled()
  })
})
