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
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { formatTime, getRoundStatusDotClass, getRoundStatusLabel } from '../utils'

function RoundWinnerCard(props: {
  winner: NonNullable<LotteryRoundDetail['winners']>[number]
}) {
  const { t } = useTranslation()
  return (
    <div className='border-border/60 bg-background rounded-lg border p-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <span className='font-medium'>
          {props.winner.username || props.winner.masked_name}
        </span>
        <Badge variant='outline'>{props.winner.masked_name}</Badge>
      </div>
      <div className='text-muted-foreground mt-1 text-xs'>
        {t('Winning time')}：{formatTime(props.winner.won_at)}
      </div>
      {props.winner.prizes && props.winner.prizes.length > 0 && (
        <div className='mt-2 flex flex-wrap gap-2'>
          {props.winner.prizes.map((prize) => (
            <Badge key={prize} variant='secondary' className='max-w-full truncate'>
              {prize}
            </Badge>
          ))}
        </div>
      )}
    </div>
  )
}

function RoundDetailCard(props: { detail: LotteryRoundDetail }) {
  const { t } = useTranslation()
  const round = props.detail.round
  const winners = props.detail.winners ?? []
  const drawn = round.status === 'finished' || round.status === 'insufficient_prizes'

  return (
    <div className='border-border/60 bg-muted/30 flex flex-col gap-3 rounded-xl border p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='flex min-w-0 flex-wrap items-center gap-2'>
          <Badge variant='outline' className='gap-1.5'>
            <span className={`size-2 rounded-full ${getRoundStatusDotClass(round.status)}`} />
            {getRoundStatusLabel(round.status, t)}
          </Badge>
          <span className='text-muted-foreground text-sm'>
            {t('Round')}: {round.round_key}
          </span>
        </div>
        <div className='text-muted-foreground text-sm'>
          {t('Draw time')}：{formatTime(round.draw_time)}
        </div>
      </div>

      <div className='grid gap-2 text-sm md:grid-cols-3'>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Registration start')}</div>
          <div>{formatTime(round.registration_start)}</div>
        </div>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Registration end')}</div>
          <div>{formatTime(round.registration_end)}</div>
        </div>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Participants')}</div>
          <div>{t('{{count}} total', { count: props.detail.participant_count })}</div>
        </div>
      </div>

      {drawn ? (
        winners.length > 0 ? (
          <div className='grid gap-2'>
            <div className='text-sm font-medium'>{t('Winning users')}</div>
            <div className='grid gap-2 xl:grid-cols-2'>
              {winners.map((winner, index) => (
                <RoundWinnerCard
                  key={winner.user_id ?? `${winner.masked_name}-${winner.won_at}-${index}`}
                  winner={winner}
                />
              ))}
            </div>
          </div>
        ) : (
          <div className='text-muted-foreground text-sm'>
            {t('This draw has been completed without winners.')}
          </div>
        )
      ) : (
        <div className='text-muted-foreground text-sm'>
          {t('This round has not drawn yet.')}
        </div>
      )}
    </div>
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
      <DialogContent className='max-h-[90vh] overflow-auto sm:max-w-5xl'>
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
                <div className='rounded-lg border border-border/60 bg-background px-3 py-2'>
                  <div className='text-muted-foreground text-xs'>{t('Assigned prizes')}</div>
                  <div className='font-medium'>{assignedPrizeCount}</div>
                </div>
                <div className='rounded-lg border border-border/60 bg-background px-3 py-2'>
                  <div className='text-muted-foreground text-xs'>{t('Unassigned prizes')}</div>
                  <div className='font-medium'>{availablePrizeCount}</div>
                </div>
              </div>
            </div>
            <div className='grid gap-2 sm:grid-cols-3'>
              <div className='border-border/60 bg-background rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>{t('Current round')}</div>
                <div className='font-medium'>
                  {props.lottery.round ? props.lottery.round.round_key : t('None')}
                </div>
              </div>
              <div className='border-border/60 bg-background rounded-lg border p-3'>
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
              <div className='border-border/60 bg-background rounded-lg border p-3'>
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
            <RoundDetailCard key={detail.round.id} detail={detail} />
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
