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
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'

import type { AuditResult } from './types'

export function AuditMatches(props: { result: AuditResult }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-3'>
      <p>
        {t('Matches')}: {props.result.count}
      </p>
      {props.result.truncated && (
        <Alert>
          <AlertDescription>
            {t(
              'Only the first 48 matches are stored; all matches are enforced. Snippets are limited to 160 bytes.'
            )}
          </AlertDescription>
        </Alert>
      )}
      {props.result.matches.map((match) => (
        <article
          key={match.number}
          className='flex flex-col gap-2 rounded-lg border p-3'
        >
          <div className='flex flex-wrap items-center gap-2'>
            <strong>
              {match.number}. {match.rule_name}
            </strong>
            <Badge variant='secondary'>{t(match.action)}</Badge>
            {match.applied && <Badge>{t('Applied')}</Badge>}
          </div>
          <code className='text-muted-foreground text-xs break-all'>
            {match.path}
          </code>
          <label>{t('Matched text')}</label>
          <pre className='bg-muted max-h-40 overflow-auto rounded-md p-2 break-all whitespace-pre-wrap'>
            {match.original}
          </pre>
          {match.action === 'replace' && (
            <>
              <label>{t('Replacement')}</label>
              <pre className='bg-muted max-h-40 overflow-auto rounded-md p-2 break-all whitespace-pre-wrap'>
                {match.replacement || t('(empty)')}
              </pre>
            </>
          )}
          {match.overlap && (
            <p className='text-muted-foreground text-sm'>
              {t(
                'Overlapping replacement skipped: an earlier rule takes priority.'
              )}
            </p>
          )}
          {match.truncated && (
            <p className='text-muted-foreground text-sm'>
              {t('Snippet truncated')}
            </p>
          )}
        </article>
      ))}
    </div>
  )
}
