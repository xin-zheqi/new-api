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

import React from 'react';
import { Popover } from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function ResponseModelMismatch({
  isAdmin,
  mismatch,
  expanded = false,
}) {
  const { t } = useTranslation();
  if (
    !isAdmin ||
    !Array.isArray(mismatch?.response_models) ||
    !mismatch.response_models.length
  )
    return null;

  const details = (
    <section
      aria-label={t('Response model mismatch')}
      className='flex min-w-0 flex-col gap-2 text-xs'
    >
      {!expanded && (
        <p className='font-medium'>{t('Response model mismatch')}</p>
      )}
      <dl className='grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2'>
        <dt className='text-[var(--semi-color-text-2)]'>
          {t('Request Model')}
        </dt>
        <dd className='min-w-0 font-mono break-all'>
          {mismatch.requested_model}
        </dd>
        <dt className='text-[var(--semi-color-text-2)]'>
          {t('Upstream response model')}
        </dt>
        <dd className='flex min-w-0 flex-col gap-1 font-mono break-all'>
          {mismatch.response_models.map((model) => (
            <span key={model}>{model}</span>
          ))}
        </dd>
      </dl>
      {mismatch.truncated && (
        <p className='text-[var(--semi-color-text-2)]'>
          {t('Additional model names were omitted.')}
        </p>
      )}
    </section>
  );
  if (expanded) return details;

  return (
    <Popover
      trigger='click'
      position='bottomLeft'
      content={
        <div className='w-80 max-w-[calc(100vw-2rem)] p-3'>{details}</div>
      }
    >
      <button
        type='button'
        onClick={(event) => event.stopPropagation()}
        className='inline-flex w-fit items-center gap-1 rounded border-0 bg-transparent p-0 text-[11px] text-[var(--semi-color-text-2)] hover:text-[var(--semi-color-text-0)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2'
      >
        <Info className='h-3 w-3 shrink-0' aria-hidden='true' />
        {t('Response model mismatch')}
      </button>
    </Popover>
  );
}
