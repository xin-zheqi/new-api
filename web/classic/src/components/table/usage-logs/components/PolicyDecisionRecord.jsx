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
import { policyLabel } from './policy-label';

export default function PolicyDecisionRecord({ events }) {
  const { t } = useTranslation();
  if (!Array.isArray(events)) return null;
  return (
    <ol
      aria-label={t('Request decision flow')}
      className='space-y-3 break-words'
    >
      {events.map((event, index) => (
        <li key={`${index}:${event.attempt}:${event.channel_id}`}>
          <p>
            {policyLabel(t, event.decision?.reason || '')}
            {event.channel_id ? ` · #${event.channel_id}` : ''}
          </p>
          <p>
            {t('Attempt')} {event.attempt} · {event.elapsed_ms} ms
            {event.group ? ` · ${event.group}` : ''}
            {event.rule ? ` · ${event.rule}` : ''}
            {event.status ? ` · HTTP ${event.status}` : ''}
          </p>
          <p>
            {t('Source')}: {policyLabel(t, event.decision?.source || '')}
            {event.health ? ` · ${policyLabel(t, event.health)}` : ''}
          </p>
        </li>
      ))}
    </ol>
  );
}
