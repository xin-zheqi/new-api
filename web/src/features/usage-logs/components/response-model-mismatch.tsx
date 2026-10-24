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
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

import type { LogOtherData } from '../types'

export function ResponseModelMismatch(props: {
  isAdmin: boolean
  mismatch?: NonNullable<LogOtherData['admin_info']>['response_model_mismatch']
  expanded?: boolean
}) {
  const { t } = useTranslation()
  if (!props.isAdmin || !props.mismatch?.response_models?.length) return null

  const details = (
    <section
      aria-label={t('Response model mismatch')}
      className='flex min-w-0 flex-col gap-2 text-xs'
    >
      <p className='font-medium'>{t('Response model mismatch')}</p>
      <dl className='grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2'>
        <dt className='text-muted-foreground'>{t('Request Model')}</dt>
        <dd className='min-w-0 font-mono break-all'>
          {props.mismatch.requested_model}
        </dd>
        <dt className='text-muted-foreground'>
          {t('Upstream response model')}
        </dt>
        <dd className='flex min-w-0 flex-col gap-1 font-mono break-all'>
          {props.mismatch.response_models.map((model) => (
            <span key={model}>{model}</span>
          ))}
        </dd>
      </dl>
      {props.mismatch.truncated && (
        <p className='text-muted-foreground'>
          {t('Additional model names were omitted.')}
        </p>
      )}
    </section>
  )
  if (props.expanded) return details

  return (
    <Popover>
      <PopoverTrigger
        render={
          <button type='button' onClick={(event) => event.stopPropagation()} />
        }
        className='text-muted-foreground hover:text-foreground focus-visible:ring-ring inline-flex w-fit items-center gap-1 rounded text-[11px] focus-visible:ring-2 focus-visible:outline-none'
      >
        <Info className='size-3 shrink-0' aria-hidden='true' />
        {t('Response model mismatch')}
      </PopoverTrigger>
      <PopoverContent align='start' className='w-80 max-w-[calc(100vw-2rem)]'>
        {details}
      </PopoverContent>
    </Popover>
  )
}
