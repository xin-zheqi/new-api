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
import { CancelCircleIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { useAuthStore } from '@/stores/auth-store'

import { closeTicket, getAdminTicket, replyToTicketAsAdmin } from '../api'
import { ticketQueryKeys } from '../constants'
import { getTicketErrorMessage } from '../lib/ticket-error'
import type { TicketWritePayload } from '../types'
import { TicketReplyForm } from './ticket-reply-form'
import { TicketThread } from './ticket-thread'

export function AdminTicketDetailSheet(props: {
  ticketId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const subjectId = useAuthStore((state) => state.auth.user?.id ?? 0)
  const [closeConfirmOpen, setCloseConfirmOpen] = useState(false)
  const detailQuery = useQuery({
    queryKey: ticketQueryKeys.detail(subjectId, props.ticketId ?? 0, 'admin'),
    queryFn: () => getAdminTicket(props.ticketId as number),
    enabled: props.open && props.ticketId !== null,
    refetchInterval: props.open ? 20_000 : false,
  })
  const replyMutation = useMutation({
    mutationFn: (payload: TicketWritePayload) =>
      replyToTicketAsAdmin(props.ticketId as number, payload),
    onSuccess: (ticket) => {
      queryClient.setQueryData(
        ticketQueryKeys.detail(subjectId, ticket.id, 'admin'),
        ticket
      )
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(subjectId),
      })
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(subjectId),
      })
      toast.success(t('Reply sent.'))
    },
    onError: (error) => {
      toast.error(getTicketErrorMessage(error, t))
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(subjectId),
      })
      void detailQuery.refetch()
    },
  })
  const closeMutation = useMutation({
    mutationFn: () => closeTicket(props.ticketId as number),
    onSuccess: (ticket) => {
      queryClient.setQueryData(
        ticketQueryKeys.detail(subjectId, ticket.id, 'admin'),
        ticket
      )
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(subjectId),
      })
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(subjectId),
      })
      setCloseConfirmOpen(false)
      toast.success(t('Support ticket closed.'))
    },
    onError: (error) => {
      toast.error(getTicketErrorMessage(error, t))
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.adminLists(subjectId),
      })
      void detailQuery.refetch()
    },
  })

  let detailContent = (
    <TicketThread
      ticket={detailQuery.data}
      isLoading={detailQuery.isLoading}
      viewerRole='admin'
      className='rounded-none border-x-0 border-b-0'
      actions={
        detailQuery.data?.status !== 'closed' ? (
          <Button
            type='button'
            variant='destructive'
            size='sm'
            disabled={replyMutation.isPending || closeMutation.isPending}
            onClick={() => setCloseConfirmOpen(true)}
          >
            <HugeiconsIcon icon={CancelCircleIcon} data-icon='inline-start' />
            {t('Close ticket')}
          </Button>
        ) : undefined
      }
      composer={
        detailQuery.data ? (
          <TicketReplyForm
            audience='admin'
            status={detailQuery.data.status}
            messageCount={detailQuery.data.message_count}
            isSubmitting={replyMutation.isPending || closeMutation.isPending}
            onSubmit={async (payload) => {
              await replyMutation.mutateAsync(payload)
              return true
            }}
          />
        ) : undefined
      }
    />
  )
  if (detailQuery.isError) {
    detailContent = (
      <div className='p-4'>
        <Alert variant='destructive'>
          <AlertDescription>
            {getTicketErrorMessage(
              detailQuery.error,
              t,
              'Failed to load support ticket.'
            )}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <>
      <Sheet
        open={props.open}
        onOpenChange={(open) => {
          if (!open && (replyMutation.isPending || closeMutation.isPending)) {
            return
          }
          if (!open) setCloseConfirmOpen(false)
          props.onOpenChange(open)
        }}
      >
        <SheetContent className='w-full gap-0 sm:max-w-3xl'>
          <SheetHeader className='sr-only'>
            <SheetTitle>{t('Support ticket details')}</SheetTitle>
            <SheetDescription>
              {t('Review and respond to the selected support ticket.')}
            </SheetDescription>
          </SheetHeader>
          <div className='min-h-0 flex-1'>{detailContent}</div>
        </SheetContent>
      </Sheet>

      <AlertDialog
        open={closeConfirmOpen}
        onOpenChange={(open) => {
          if (!closeMutation.isPending) setCloseConfirmOpen(open)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Close this ticket?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Only administrators can close tickets. This action cannot be undone.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={closeMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={closeMutation.isPending}
              onClick={() => closeMutation.mutate()}
            >
              {closeMutation.isPending ? (
                <Spinner data-icon='inline-start' />
              ) : (
                <HugeiconsIcon
                  icon={CancelCircleIcon}
                  data-icon='inline-start'
                />
              )}
              {t('Close ticket')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
