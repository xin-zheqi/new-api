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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { channelSchema, type Channel } from '../../types'
import { ChannelCard } from '../channel-card'
import { useChannelsColumns } from '../channels-columns'
import { ChannelsProvider } from '../channels-provider'

function ScheduleStatuses({
  channels,
  cardView = false,
}: {
  channels: Channel[]
  cardView?: boolean
}) {
  const columns = useChannelsColumns({ enableSelection: false }).filter(
    (column) => 'accessorKey' in column && column.accessorKey === 'status'
  )
  const table = useReactTable({
    data: channels,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  return table.getRowModel().rows.map((row) => (
    <section key={row.id} aria-label={row.original.name}>
      {cardView ? (
        <ChannelCard row={row} isSelected={false} />
      ) : (
        row
          .getAllCells()
          .map((cell) => (
            <div key={cell.id}>
              {flexRender(cell.column.columnDef.cell, cell.getContext())}
            </div>
          ))
      )}
    </section>
  ))
}

function renderStatuses(channels: Channel[], cardView = false) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChannelsProvider>
        <ScheduleStatuses channels={channels} cardView={cardView} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

function channel(
  id: number,
  status: number,
  schedule_active?: boolean
): Channel {
  return channelSchema.parse({
    id,
    status,
    schedule_active,
    name: `Channel ${id}`,
    type: 1,
    key: '',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  })
}

describe('Channel schedule status presentation', () => {
  test('a scheduled pause is separate from manual and health disables', () => {
    const channels = [
      channel(1, 1, false),
      channel(2, 1, true),
      channel(3, 1),
      channel(4, 2, false),
      channel(5, 3, false),
    ]
    renderStatuses(channels)
    expect(
      within(screen.getByRole('region', { name: 'Channel 1' })).getByText(
        'Scheduled pause'
      )
    ).toBeVisible()
    expect(
      within(screen.getByRole('region', { name: 'Channel 2' })).getByText(
        'Enabled'
      )
    ).toBeVisible()
    expect(
      within(screen.getByRole('region', { name: 'Channel 3' })).getByText(
        'Enabled'
      )
    ).toBeVisible()
    expect(
      within(screen.getByRole('region', { name: 'Channel 4' })).getByText(
        'Disabled'
      )
    ).toBeVisible()
    expect(
      within(screen.getByRole('region', { name: 'Channel 5' })).getByText(
        'Auto Disabled'
      )
    ).toBeVisible()
    expect(screen.getAllByText('Scheduled pause')).toHaveLength(1)
    expect(channels.map((value) => value.status)).toEqual([1, 1, 1, 2, 3])
  })

  test('card view displays the scheduled pause and retains the health-disabled status', () => {
    renderStatuses(
      [
        channel(1, 1, false),
        channel(2, 1, true),
        channel(3, 2, false),
        channel(4, 3, false),
      ],
      true
    )
    expect(
      within(screen.getByRole('region', { name: 'Channel 1' })).getByText(
        'Scheduled pause'
      )
    ).toBeVisible()
    expect(
      within(screen.getByRole('region', { name: 'Channel 2' })).queryByText(
        'Scheduled pause'
      )
    ).not.toBeInTheDocument()
    expect(
      within(screen.getByRole('region', { name: 'Channel 3' })).queryByText(
        'Scheduled pause'
      )
    ).not.toBeInTheDocument()
    expect(
      within(screen.getByRole('region', { name: 'Channel 4' })).getByText(
        'Auto Disabled'
      )
    ).toBeVisible()
  })
})
