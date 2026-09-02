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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import { AdminSupportTickets } from '@/features/support-tickets/admin'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const adminTicketsSearchSchema = z.object({
  page: z.number().int().positive().optional().catch(1),
  pageSize: z.number().int().min(1).max(50).optional().catch(undefined),
  filter: z
    .string()
    .refine((value) => [...value].length <= 100)
    .optional()
    .catch(''),
  status: z
    .array(z.enum(['waiting_admin', 'waiting_user', 'closed']))
    .max(1)
    .default(['waiting_admin'])
    .catch(['waiting_admin']),
  userId: z
    .string()
    .regex(/^[1-9]\d*$/)
    .max(19)
    .optional()
    .catch(''),
})

export const Route = createFileRoute('/_authenticated/support-tickets/admin')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: adminTicketsSearchSchema,
  component: AdminSupportTickets,
})
