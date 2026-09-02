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

import { StatusBadge } from '@/components/status-badge'

import { TICKET_STATUS_VARIANTS } from '../constants'
import type { TicketStatus } from '../types'

export function TicketStatusBadge(props: {
  status: TicketStatus
  audience: 'user' | 'admin'
}) {
  const { t } = useTranslation()

  let label = t('Closed')
  if (props.status === 'waiting_admin') {
    label =
      props.audience === 'admin' ? t('Needs reply') : t('Waiting for support')
  } else if (props.status === 'waiting_user') {
    label =
      props.audience === 'admin'
        ? t('Waiting for user')
        : t('Your reply is needed')
  }

  return (
    <StatusBadge
      label={label}
      variant={TICKET_STATUS_VARIANTS[props.status]}
      copyable={false}
      showDot
    />
  )
}
