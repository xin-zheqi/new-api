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
import { useTranslation } from 'react-i18next';
import { Banner, Tag } from '@douyinfe/semi-ui';
export default function Matches({ result }) {
  const { t } = useTranslation();
  return (
    <div className='flex flex-col gap-3'>
      <div>
        {t('Matches')}: {result.count}
      </div>
      {result.truncated && (
        <Banner
          type='info'
          description={t(
            'Only the first 48 matches are stored; all matches are enforced. Snippets are limited to 160 bytes.',
          )}
        />
      )}
      {result.matches.map((match, index) => (
        <article
          key={index}
          className='rounded-xl border p-3 flex flex-col gap-2'
        >
          <div className='flex flex-wrap gap-2'>
            <strong>
              {index + 1}. {match.rule_name}
            </strong>
            <Tag>{t(match.action)}</Tag>
            {match.applied && <Tag color='blue'>{t('Applied')}</Tag>}
          </div>
          <code className='break-all text-xs'>{match.path}</code>
          <span>{t('Matched text')}</span>
          <pre className='max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-lg p-2 bg-[var(--semi-color-fill-0)]'>
            {match.original}
          </pre>
          {match.action === 'replace' && (
            <>
              <span>{t('Replacement')}</span>
              <pre className='max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-lg p-2 bg-[var(--semi-color-fill-0)]'>
                {match.replacement || t('(empty)')}
              </pre>
            </>
          )}
          {match.overlap && (
            <p>
              {t(
                'Overlapping replacement skipped: an earlier rule takes priority.',
              )}
            </p>
          )}
          {match.truncated && <p>{t('Snippet truncated')}</p>}
        </article>
      ))}
    </div>
  );
}
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
