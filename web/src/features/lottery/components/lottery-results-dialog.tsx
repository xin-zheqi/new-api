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
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Separator } from '@/components/ui/separator'
import { getAdminLotteryRounds } from '../api'
import type { LotteryActivity, LotteryRoundDetail } from '../types'
import {
  formatRoundLabel,
  formatTime,
  getRoundStatusDotClass,
  getRoundStatusLabel,
} from '../utils'

function RoundResultRow(props: {
  detail: LotteryRoundDetail
  lotteryMode?: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const round = props.detail.round
  const winners = props.detail.winners ?? []
  const drawn = round.status === 'finished' || round.status === 'insufficient_prizes'
  const roundLabel = formatRoundLabel(
    props.lotteryMode,
    round.round_key,
    round.draw_time,
    t
  )

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='overflow-hidden rounded-xl border'
    >
      <CollapsibleTrigger className='hover:bg-muted/50 flex w-full flex-col gap-3 p-4 text-left transition-colors lg:grid lg:grid-cols-[1.2fr_1fr_1fr_.8fr_.8fr_auto] lg:items-center'>
        <div className='flex min-w-0 items-center gap-2'>
          <Badge variant='outline' className='gap-1.5'>
            <span className={`size-2 rounded-full ${getRoundStatusDotClass(round.status)}`} />
            {getRoundStatusLabel(round.status, t)}
          </Badge>
        </div>
        <div className='min-w-0'>
          <div className='text-muted-foreground text-xs'>{t('Round')}</div>
          <div className='truncate font-medium'>{roundLabel}</div>
        </div>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Draw time')}</div>
          <div>{formatTime(round.draw_time)}</div>
        </div>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Participants')}</div>
          <div>{props.detail.participant_count || 0}</div>
        </div>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Winning users')}</div>
          <div>{winners.length}</div>
        </div>
        <span
          className='bg-muted/50 flex size-8 items-center justify-center rounded-lg'
          aria-hidden='true'
        >
          <ChevronDown
            data-icon='inline-start'
            className={open ? 'rotate-180 transition-transform' : 'transition-transform'}
          />
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className='bg-muted/20 border-t p-4'>
          <div className='grid gap-3 text-sm md:grid-cols-3'>
            <div>
              <div className='text-muted-foreground text-xs'>{t('Registration start')}</div>
              <div>{formatTime(round.registration_start)}</div>
            </div>
            <div>
              <div className='text-muted-foreground text-xs'>{t('Registration end')}</div>
              <div>{formatTime(round.registration_end)}</div>
            </div>
            <div>
              <div className='text-muted-foreground text-xs'>{t('Internal round key')}</div>
              <div className='font-mono'>{round.round_key || '-'}</div>
            </div>
          </div>

          <Separator className='my-4' />

          {drawn && winners.length > 0 ? (
            <div className='flex flex-col gap-3'>
              {winners.map((winner, index) => {
                const prizeDetails =
                  winner.prize_details && winner.prize_details.length > 0
                    ? winner.prize_details
                    : (winner.prizes ?? []).map((code, prizeIndex) => ({
                        id: prizeIndex,
                        prize_name: '',
                        code,
                      }))
                return (
                  <div
                    key={winner.user_id ?? `${winner.masked_name}-${winner.won_at}-${index}`}
                    className='bg-background rounded-lg border p-3'
                  >
                    <div className='flex flex-col gap-2 md:flex-row md:items-start md:justify-between'>
                      <div>
                        <div className='font-medium'>
                          {winner.username || winner.masked_name}
                        </div>
                        <div className='text-muted-foreground text-xs'>
                          {t('User ID')}: {winner.user_id || '-'} · {t('Winning time')}:
                          {formatTime(winner.won_at)}
                        </div>
                      </div>
                      <Badge variant='secondary'>
                        {t('{{count}} total', { count: prizeDetails.length })}
                      </Badge>
                    </div>
                    <div className='mt-3 grid gap-2 md:grid-cols-2'>
                      {prizeDetails.map((prize) => (
                        <div
                          key={`${prize.id}-${prize.code}`}
                          className='bg-muted/40 min-w-0 rounded-lg px-3 py-2'
                        >
                          <div className='text-muted-foreground truncate text-xs'>
                            {prize.prize_name || t('Prize code')}
                          </div>
                          <div className='truncate font-mono text-sm'>{prize.code}</div>
                        </div>
                      ))}
                    </div>
                  </div>
                )
              })}
            </div>
          ) : (
            <div className='text-muted-foreground text-sm'>
              {drawn
                ? t('This draw has been completed without winners.')
                : t('This round has not drawn yet.')}
            </div>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

export function LotteryResultsDialog(props: {
  lottery: LotteryActivity | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(5)
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState('all')

  useEffect(() => {
    if (!props.open) {
      setPage(1)
      setPageSize(5)
      setKeyword('')
      setStatus('all')
    }
  }, [props.open])

  useEffect(() => {
    setPage(1)
  }, [keyword, status, pageSize, props.lottery?.id])

  const query = useQuery({
    queryKey: [
      'admin-lottery-rounds',
      props.lottery?.id,
      page,
      pageSize,
      keyword,
      status,
    ],
    queryFn: () =>
      getAdminLotteryRounds(props.lottery?.id ?? 0, {
        p: page,
        page_size: pageSize,
        keyword,
        status,
      }),
    enabled: props.open && Boolean(props.lottery?.id),
  })

  const pageData = query.data?.data
  const rounds = pageData?.items ?? []
  const total = pageData?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const assignedPrizeCount = props.lottery?.assigned_prize_count ?? 0
  const availablePrizeCount = props.lottery?.available_prize_count ?? 0

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-auto sm:max-w-6xl'>
        <DialogHeader>
          <DialogTitle>{t('Winning details')}</DialogTitle>
          <DialogDescription>
            {props.lottery
              ? t('Query draw rounds, winners, and prize assignments for this activity.')
              : t('Query draw rounds, winners, and prize assignments.')}
          </DialogDescription>
        </DialogHeader>

        {props.lottery && (
          <div className='border-border/60 bg-muted/20 flex flex-col gap-4 rounded-xl border p-4'>
            <div className='flex flex-wrap items-start justify-between gap-3'>
              <div className='min-w-0'>
                <div className='truncate text-base font-medium'>{props.lottery.title}</div>
                <div className='text-muted-foreground text-sm'>{props.lottery.prize_name}</div>
              </div>
              <div className='flex flex-wrap items-center gap-2 text-sm'>
                <div className='bg-background rounded-lg border px-3 py-2'>
                  <div className='text-muted-foreground text-xs'>{t('Assigned prizes')}</div>
                  <div className='font-medium'>{assignedPrizeCount}</div>
                </div>
                <div className='bg-background rounded-lg border px-3 py-2'>
                  <div className='text-muted-foreground text-xs'>{t('Unassigned prizes')}</div>
                  <div className='font-medium'>{availablePrizeCount}</div>
                </div>
              </div>
            </div>
            <div className='grid gap-2 sm:grid-cols-3'>
              <div className='bg-background rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>{t('Current round')}</div>
                <div className='font-medium'>
                  {props.lottery.round
                    ? formatRoundLabel(
                        props.lottery.mode,
                        props.lottery.round.round_key,
                        props.lottery.round.draw_time,
                        t
                      )
                    : t('None')}
                </div>
              </div>
              <div className='bg-background rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>{t('Winning users')}</div>
                <div className='font-medium'>
                  {t('{{count}} total', {
                    count: rounds.reduce(
                      (sum, detail) => sum + (detail.winners?.length ?? 0),
                      0
                    ),
                  })}
                </div>
              </div>
              <div className='bg-background rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>{t('Rounds')}</div>
                <div className='font-medium'>{t('{{count}} total', { count: total })}</div>
              </div>
            </div>
          </div>
        )}

        <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
          <div className='flex flex-col gap-2 md:flex-row md:items-center'>
            <Input
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
              placeholder={t('Search round, winner, or code')}
              className='md:w-72'
            />
            <NativeSelect
              value={status}
              onChange={(event) => setStatus(event.target.value)}
              className='md:w-40'
            >
              <NativeSelectOption value='all'>{t('All rounds')}</NativeSelectOption>
              <NativeSelectOption value='drawn'>{t('Drawn')}</NativeSelectOption>
              <NativeSelectOption value='undrawn'>{t('Undrawn')}</NativeSelectOption>
            </NativeSelect>
          </div>
          <div className='flex items-center gap-2'>
            <NativeSelect
              value={String(pageSize)}
              onChange={(event) => setPageSize(Number(event.target.value) || 5)}
              className='w-28'
            >
              <NativeSelectOption value='5'>5</NativeSelectOption>
              <NativeSelectOption value='10'>10</NativeSelectOption>
              <NativeSelectOption value='20'>20</NativeSelectOption>
            </NativeSelect>
            <div className='text-muted-foreground text-sm'>
              {total === 0
                ? t('No matching rounds')
                : t('Page {{page}} of {{total}}', {
                    page,
                    total: totalPages,
                  })}
            </div>
          </div>
        </div>

        <Separator />

        <div className='flex flex-col gap-3'>
          {rounds.map((detail) => (
            <RoundResultRow
              key={detail.round.id}
              detail={detail}
              lotteryMode={props.lottery?.mode}
            />
          ))}
          {!query.isLoading && rounds.length === 0 && (
            <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
              {t('No matching rounds')}
            </div>
          )}
        </div>

        <div className='flex items-center justify-between gap-3'>
          <Button
            variant='outline'
            disabled={page <= 1 || query.isFetching}
            onClick={() => setPage((current) => Math.max(1, current - 1))}
          >
            {t('Previous')}
          </Button>
          <div className='text-muted-foreground text-sm'>
            {t('Page {{page}} of {{total}}', { page, total: totalPages })}
          </div>
          <Button
            variant='outline'
            disabled={page >= totalPages || query.isFetching}
            onClick={() => setPage((current) => Math.min(totalPages, current + 1))}
          >
            {t('Next')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
