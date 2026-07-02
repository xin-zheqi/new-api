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
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Edit3,
  Eye,
  ListFilter,
  Plus,
  RefreshCw,
  Trash2,
  Trophy,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import {
  deleteLottery,
  drawLotteryRound,
  getAdminLotteries,
  updateLotteryStatus,
} from '../api'
import type {
  LotteryActivity,
  LotteryDrawStatusFilter,
} from '../types'
import {
  formatTime,
  getRoundStatusDotClass,
  getRoundStatusLabel,
  isRoundUndrawn,
} from '../utils'
import { CreateLotteryDialog } from './create-lottery-dialog'
import { LotteryResultsDialog } from './lottery-results-dialog'

export function LotteryAdmin() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [openCreate, setOpenCreate] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [mode, setMode] = useState('')
  const [status, setStatus] = useState('')
  const [drawStatus, setDrawStatus] = useState<LotteryDrawStatusFilter>('all')
  const [editingLottery, setEditingLottery] = useState<LotteryActivity | null>(null)
  const [detailLottery, setDetailLottery] = useState<LotteryActivity | null>(null)
  const [resultsLottery, setResultsLottery] = useState<LotteryActivity | null>(null)

  const adminQuery = useQuery({
    queryKey: ['admin-lotteries', keyword, mode, status, drawStatus],
    queryFn: () =>
      getAdminLotteries({
        p: 1,
        page_size: 50,
        keyword,
        mode,
        status,
        draw_status: drawStatus,
      }),
  })
  const statusMutation = useMutation({
    mutationFn: (payload: { id: number; status: number }) =>
      updateLotteryStatus(payload.id, payload.status),
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Draw status updated'))
        await queryClient.invalidateQueries({ queryKey: ['admin-lotteries'] })
        await queryClient.invalidateQueries({ queryKey: ['lotteries'] })
      }
    },
  })
  const drawMutation = useMutation({
    mutationFn: drawLotteryRound,
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Draw completed'))
        await queryClient.invalidateQueries({ queryKey: ['admin-lotteries'] })
        await queryClient.invalidateQueries({ queryKey: ['lotteries'] })
      }
    },
  })
  const deleteMutation = useMutation({
    mutationFn: deleteLottery,
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Draw deleted'))
        await queryClient.invalidateQueries({ queryKey: ['admin-lotteries'] })
        await queryClient.invalidateQueries({ queryKey: ['lotteries'] })
      }
    },
  })

  const lotteries = adminQuery.data?.data.items ?? []

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
        <div className='flex flex-col gap-2 md:flex-row md:items-center'>
          <Input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder={t('Search draw or prize')}
            className='md:w-64'
          />
          <NativeSelect value={mode} onChange={(event) => setMode(event.target.value)}>
            <NativeSelectOption value=''>{t('All modes')}</NativeSelectOption>
            <NativeSelectOption value='once'>{t('One-time')}</NativeSelectOption>
            <NativeSelectOption value='scheduled'>{t('Scheduled')}</NativeSelectOption>
          </NativeSelect>
          <NativeSelect
            value={status}
            onChange={(event) => setStatus(event.target.value)}
          >
            <NativeSelectOption value=''>{t('All statuses')}</NativeSelectOption>
            <NativeSelectOption value='1'>{t('Enabled')}</NativeSelectOption>
            <NativeSelectOption value='2'>{t('Disabled')}</NativeSelectOption>
            <NativeSelectOption value='3'>{t('Deleted')}</NativeSelectOption>
          </NativeSelect>
          <NativeSelect
            value={drawStatus}
            onChange={(event) =>
              setDrawStatus(event.target.value as LotteryDrawStatusFilter)
            }
          >
            <NativeSelectOption value='all'>{t('All draws')}</NativeSelectOption>
            <NativeSelectOption value='undrawn'>{t('Undrawn')}</NativeSelectOption>
            <NativeSelectOption value='drawn'>{t('Drawn')}</NativeSelectOption>
          </NativeSelect>
        </div>
        <div className='flex gap-2'>
          <Button variant='outline' onClick={() => adminQuery.refetch()}>
            <RefreshCw data-icon='inline-start' />
            {t('Refresh')}
          </Button>
          <Button onClick={() => setOpenCreate(true)}>
            <Plus data-icon='inline-start' />
            {t('Create draw')}
          </Button>
        </div>
      </div>

      <div className='grid gap-3'>
        {lotteries.map((lottery) => (
          <AdminLotteryRow
            key={lottery.id}
            lottery={lottery}
            updating={statusMutation.isPending || drawMutation.isPending}
            onToggle={() =>
              statusMutation.mutate({
                id: lottery.id,
                status: lottery.status === 1 ? 2 : 1,
              })
            }
            onDraw={() => {
              if (lottery.round) drawMutation.mutate(lottery.round.id)
            }}
            onEdit={() => setEditingLottery(lottery)}
            onDetail={() => setDetailLottery(lottery)}
            onResults={() => setResultsLottery(lottery)}
            onDelete={() => deleteMutation.mutate(lottery.id)}
          />
        ))}
      </div>

      <CreateLotteryDialog open={openCreate} onOpenChange={setOpenCreate} />
      <CreateLotteryDialog
        open={Boolean(editingLottery)}
        onOpenChange={(open) => {
          if (!open) setEditingLottery(null)
        }}
        lottery={editingLottery}
      />
      <CreateLotteryDialog
        open={Boolean(detailLottery)}
        onOpenChange={(open) => {
          if (!open) setDetailLottery(null)
        }}
        lottery={detailLottery}
        readOnly
      />
      <LotteryResultsDialog
        open={Boolean(resultsLottery)}
        onOpenChange={(open) => {
          if (!open) setResultsLottery(null)
        }}
        lottery={resultsLottery}
      />
    </div>
  )
}

function AdminLotteryRow(props: {
  lottery: LotteryActivity
  updating: boolean
  onToggle: () => void
  onDraw: () => void
  onEdit: () => void
  onDetail: () => void
  onResults: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const round = props.lottery.round
  const deleted = props.lottery.status === 3
  const canDraw = Boolean(
    round && (round.status === 'pending' || round.status === 'open')
  )
  const canEdit = Boolean(!deleted && props.lottery.can_edit && isRoundUndrawn(round?.status))
  let statusLabel = t('Disabled')
  if (props.lottery.status === 1) statusLabel = t('Enabled')
  if (props.lottery.status === 3) statusLabel = t('Deleted')
  const [drawConfirmOpen, setDrawConfirmOpen] = useState(false)

  return (
    <Card>
      <CardContent className='flex flex-col gap-4 p-4'>
        <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
          <div className='min-w-0'>
            <div className='flex flex-wrap items-center gap-2'>
              <h3 className='truncate text-base font-medium'>{props.lottery.title}</h3>
              <Badge variant={props.lottery.status === 1 ? 'secondary' : 'outline'}>
                {statusLabel}
              </Badge>
              <Badge variant='outline'>
                {props.lottery.mode === 'scheduled' ? t('Scheduled') : t('One-time')}
              </Badge>
            </div>
            <div className='text-muted-foreground text-sm'>
              {props.lottery.prize_name} · {t('{{count}} total', {
                count: props.lottery.participant_count,
              })}
            </div>
            <div className='mt-2 flex flex-wrap gap-2 text-xs'>
              <Badge variant='secondary'>
                {t('Assigned prizes')}: {props.lottery.assigned_prize_count ?? 0}
              </Badge>
              <Badge variant='outline'>
                {t('Unassigned prizes')}: {props.lottery.available_prize_count ?? 0}
              </Badge>
            </div>
          </div>
          <div className='flex flex-wrap gap-2'>
            <Button variant='outline' disabled={props.updating || deleted} onClick={props.onToggle}>
              {props.lottery.status === 1 ? t('Disable') : t('Enable')}
            </Button>
            {canEdit ? (
              <Button
                variant='outline'
                disabled={props.updating}
                onClick={props.onEdit}
              >
                <Edit3 data-icon='inline-start' />
                {t('Edit')}
              </Button>
            ) : (
              <Button variant='outline' onClick={props.onDetail}>
                <Eye data-icon='inline-start' />
                {t('View details')}
              </Button>
            )}
            <Button variant='outline' onClick={props.onResults}>
              <Trophy data-icon='inline-start' />
              {t('View winning details')}
            </Button>
            <AlertDialog open={drawConfirmOpen} onOpenChange={setDrawConfirmOpen}>
              <AlertDialogTrigger
                render={
                  <Button
                    variant='outline'
                    disabled={props.updating || !canDraw || deleted}
                  />
                }
              >
                <ListFilter data-icon='inline-start' />
                {t('Draw now')}
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>{t('Confirm draw now')}</AlertDialogTitle>
                  <AlertDialogDescription>
                    {t('This will immediately draw the current round and cannot be undone.')}
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
                  <AlertDialogAction
                    onClick={() => {
                      setDrawConfirmOpen(false)
                      props.onDraw()
                    }}
                  >
                    {t('Draw now')}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
            <AlertDialog>
              <AlertDialogTrigger
                render={
                  <Button variant='outline' disabled={props.updating || deleted} />
                }
              >
                <Trash2 data-icon='inline-start' />
                {t('Delete')}
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>{t('Delete draw')}</AlertDialogTitle>
                  <AlertDialogDescription>
                    {t('Deleted draws will be hidden from users and cannot be joined anymore.')}
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
                  <AlertDialogAction onClick={props.onDelete}>
                    {t('Delete')}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </div>
        {round && (
          <div className='grid gap-2 text-sm md:grid-cols-4'>
            <span className='inline-flex items-center gap-1.5'>
              <span className={`size-2 rounded-full ${getRoundStatusDotClass(round.status)}`} />
              {getRoundStatusLabel(round.status, t)}
            </span>
            <span>{formatTime(round.registration_start)}</span>
            <span>{formatTime(round.registration_end)}</span>
            <span>{formatTime(round.draw_time)}</span>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
