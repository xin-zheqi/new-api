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
import { Add01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useMediaQuery } from '@/hooks'
import { useAuthStore } from '@/stores/auth-store'

import { createTicket, getMyTickets, getTicket, replyToTicket } from './api'
import { CreateTicketDialog } from './components/create-ticket-dialog'
import { TicketHistoryList } from './components/ticket-history-list'
import { TicketReplyForm } from './components/ticket-reply-form'
import { TicketThread } from './components/ticket-thread'
import { TICKET_PAGE_SIZE, ticketQueryKeys } from './constants'
import { getTicketErrorMessage } from './lib/ticket-error'
import type { TicketCreatePayload, TicketWritePayload } from './types'

export function SupportTickets() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const subjectId = useAuthStore((state) => state.auth.user?.id ?? 0)
  const isCompact = useMediaQuery('(max-width: 1023px)')
  const [page, setPage] = useState(1)
  const [selectedTicketId, setSelectedTicketId] = useState<number | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [mobileTab, setMobileTab] = useState('history')

  const listQuery = useQuery({
    queryKey: ticketQueryKeys.userList(subjectId, page, TICKET_PAGE_SIZE),
    queryFn: () => getMyTickets(page, TICKET_PAGE_SIZE),
    enabled: subjectId > 0,
    placeholderData: (previousData) => previousData,
    refetchInterval: 30_000,
  })
  const firstTicketId = listQuery.data?.items[0]?.id ?? null
  const activeTicketId = listQuery.data?.active_ticket_id ?? null
  const activeTicketOnPage =
    listQuery.data?.items.some((ticket) => ticket.id === activeTicketId) ===
    true
      ? activeTicketId
      : null
  const resolvedTicketId =
    selectedTicketId ?? activeTicketOnPage ?? firstTicketId ?? null

  const detailQuery = useQuery({
    queryKey: ticketQueryKeys.detail(subjectId, resolvedTicketId ?? 0, 'user'),
    queryFn: () => getTicket(resolvedTicketId as number),
    enabled: resolvedTicketId !== null,
    refetchInterval: (query) =>
      query.state.data?.status === 'closed' ? false : 30_000,
  })

  const createMutation = useMutation({
    mutationFn: createTicket,
    onSuccess: (ticket) => {
      queryClient.setQueryData(
        ticketQueryKeys.detail(subjectId, ticket.id, 'user'),
        ticket
      )
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(subjectId),
      })
      setPage(1)
      setSelectedTicketId(ticket.id)
      setMobileTab('conversation')
      toast.success(t('Support ticket created.'))
    },
    onError: (error) => {
      toast.error(getTicketErrorMessage(error, t))
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(subjectId),
      })
    },
  })

  const replyMutation = useMutation({
    mutationFn: (payload: TicketWritePayload) =>
      replyToTicket(resolvedTicketId as number, payload),
    onSuccess: (ticket) => {
      queryClient.setQueryData(
        ticketQueryKeys.detail(subjectId, ticket.id, 'user'),
        ticket
      )
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(subjectId),
      })
      toast.success(t('Reply sent.'))
    },
    onError: (error) => {
      toast.error(getTicketErrorMessage(error, t))
      void queryClient.invalidateQueries({
        queryKey: ticketQueryKeys.userLists(subjectId),
      })
      if (resolvedTicketId !== null) void detailQuery.refetch()
    },
  })

  const selectTicket = (id: number) => {
    setSelectedTicketId(id)
    setMobileTab('conversation')
  }

  const history = (
    <TicketHistoryList
      items={listQuery.data?.items ?? []}
      selectedId={resolvedTicketId}
      page={page}
      pageSize={TICKET_PAGE_SIZE}
      total={listQuery.data?.total ?? 0}
      isLoading={listQuery.isLoading}
      errorMessage={
        listQuery.isError
          ? getTicketErrorMessage(
              listQuery.error,
              t,
              'Failed to load support tickets.'
            )
          : undefined
      }
      onSelect={selectTicket}
      onPageChange={(nextPage) => {
        setPage(nextPage)
        setSelectedTicketId(null)
      }}
    />
  )

  let conversation = (
    <TicketThread
      ticket={detailQuery.data}
      isLoading={detailQuery.isLoading}
      viewerRole='user'
      composer={
        detailQuery.data ? (
          <TicketReplyForm
            audience='user'
            status={detailQuery.data.status}
            messageCount={detailQuery.data.message_count}
            isSubmitting={replyMutation.isPending}
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
    conversation = (
      <div className='flex size-full items-start justify-center rounded-md border p-4'>
        <Alert variant='destructive' className='max-w-lg'>
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

  let workspace = (
    <div className='grid size-full min-h-0 grid-cols-[minmax(250px,320px)_minmax(0,1fr)] gap-3'>
      {history}
      {conversation}
    </div>
  )
  if (isCompact) {
    workspace = (
      <Tabs
        value={mobileTab}
        onValueChange={(value) => setMobileTab(String(value))}
        className='size-full min-h-0'
      >
        <TabsList className='grid w-full shrink-0 grid-cols-2'>
          <TabsTrigger value='history'>{t('Ticket history')}</TabsTrigger>
          <TabsTrigger value='conversation'>{t('Conversation')}</TabsTrigger>
        </TabsList>
        <TabsContent value='history' className='min-h-0 overflow-hidden'>
          {history}
        </TabsContent>
        <TabsContent value='conversation' className='min-h-0 overflow-hidden'>
          {conversation}
        </TabsContent>
      </Tabs>
    )
  }

  const canCreate =
    !listQuery.isLoading &&
    !listQuery.isError &&
    !createMutation.isPending &&
    activeTicketId === null
  let createButtonTitle = t('Create support ticket')
  if (listQuery.isLoading) {
    createButtonTitle = t('Loading...')
  } else if (listQuery.isError) {
    createButtonTitle = t('Failed to load support tickets.')
  } else if (activeTicketId !== null) {
    createButtonTitle = t(
      'Your current ticket must be closed before creating another.'
    )
  }

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Support tickets')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            disabled={!canCreate}
            title={createButtonTitle}
            onClick={() => setCreateOpen(true)}
          >
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {activeTicketId === null ? t('New ticket') : t('Ticket open')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>{workspace}</SectionPageLayout.Content>
      </SectionPageLayout>

      <CreateTicketDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        isSubmitting={createMutation.isPending}
        onSubmit={async (payload: TicketCreatePayload) => {
          await createMutation.mutateAsync(payload)
          return true
        }}
      />
    </>
  )
}
