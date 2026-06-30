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
import type { ReactNode } from 'react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function Metric(props: { icon: ReactNode; label: string; value: string }) {
  return (
    <div className='bg-muted/40 flex min-w-0 items-center gap-2 rounded-lg p-3'>
      <div className='text-muted-foreground shrink-0 [&_svg]:size-4'>{props.icon}</div>
      <div className='min-w-0 flex-1'>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
        <Tooltip>
          <TooltipTrigger render={<div className='truncate text-sm font-medium' />}>
            {props.value}
          </TooltipTrigger>
          <TooltipContent>{props.value}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  )
}
