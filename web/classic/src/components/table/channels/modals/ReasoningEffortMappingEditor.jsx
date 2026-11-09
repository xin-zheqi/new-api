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

import React, { useId } from 'react';
import { Button, Input, Typography } from '@douyinfe/semi-ui';
import { IconArrowRight, IconDelete, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

export default function ReasoningEffortMappingEditor(props) {
  const { t } = useTranslation();
  const labelId = useId();
  const rows = props.value || [];

  return (
    <section className='flex flex-col gap-2 py-2' aria-labelledby={labelId}>
      <div className='flex items-center justify-between gap-2'>
        <Typography.Text id={labelId} className='font-medium'>
          {t('Reasoning effort mapping')}
        </Typography.Text>
        <Button
          size='small'
          type='tertiary'
          icon={<IconPlus aria-hidden='true' />}
          htmlType='button'
          aria-label={t('Add mapping')}
          disabled={rows.length >= 64}
          onClick={() => props.onChange([...rows, { from: '', to: '' }])}
        >
          {t('Add mapping')}
        </Button>
      </div>
      {rows.map((row, index) => (
        <div className='flex min-w-0 items-center gap-2' key={index}>
          <Input
            className='min-w-0 flex-1'
            size='small'
            value={row.from}
            aria-label={t('Before mapping')}
            placeholder={t('Before mapping')}
            onChange={(value) =>
              props.onChange(
                rows.map((item, i) =>
                  i === index ? { ...item, from: value } : item,
                ),
              )
            }
          />
          <IconArrowRight
            className='shrink-0 text-[var(--semi-color-text-2)]'
            aria-hidden='true'
          />
          <Input
            className='min-w-0 flex-1'
            size='small'
            value={row.to}
            aria-label={t('After mapping')}
            placeholder={t('After mapping')}
            onChange={(value) =>
              props.onChange(
                rows.map((item, i) =>
                  i === index ? { ...item, to: value } : item,
                ),
              )
            }
          />
          <Button
            className='shrink-0'
            size='small'
            type='tertiary'
            htmlType='button'
            icon={<IconDelete aria-hidden='true' />}
            aria-label={t('Remove mapping')}
            onClick={() => props.onChange(rows.filter((_, i) => i !== index))}
          />
        </div>
      ))}
      <Typography.Text type='tertiary' size='small'>
        {t(
          'Map existing effort values once after conversion. Leave empty to keep requests unchanged. Targets must be supported by the upstream; body passthrough skips mapping.',
        )}
      </Typography.Text>
    </section>
  );
}
