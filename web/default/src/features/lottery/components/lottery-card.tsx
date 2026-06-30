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
import { Award, CalendarDays, Gift, Sparkles, Trophy, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import type { LotteryActivity } from '../types'
import {
  formatTime,
  getRoundStatusDotClass,
  getRoundStatusLabel,
  isRoundDrawn,
  weekdayLabel,
} from '../utils'
import { EligibilityStatus } from './eligibility-status'
import { Metric } from './metric'

export function LotteryCard(props: {
  lottery: LotteryActivity
  onJoin: () => void
  joining: boolean
}) {
  const { t } = useTranslation()
  const round = props.lottery.round
  const canJoin =
    round?.status === 'open' &&
    !props.lottery.joined &&
    props.lottery.eligibility?.eligible !== false &&
    Date.now() / 1000 < round.registration_end
  const drawn = isRoundDrawn(round?.status)
  let actionLabel = t('Join draw')
  if (drawn) {
    actionLabel = t('Draw completed')
  } else if (props.lottery.joined) {
    actionLabel = t('Joined')
  }
  const roundDotClass = getRoundStatusDotClass(round?.status)

  return (
    <Card className={props.lottery.won ? 'border-primary/50' : undefined}>
      <CardHeader>
        <div className='flex items-start justify-between gap-3'>
          <div className='flex min-w-0 flex-col gap-1'>
            <div className='flex items-center gap-2'>
              <CardTitle className='truncate'>{props.lottery.title}</CardTitle>
              {props.lottery.won && (
                <Badge variant='default'>
                  <Award data-icon='inline-start' />
                  {t('You won')}
                </Badge>
              )}
            </div>
            <CardDescription className='line-clamp-2'>
              {props.lottery.description || t('No description')}
            </CardDescription>
          </div>
          <div className='flex items-center gap-2'>
            <Badge variant={props.lottery.mode === 'scheduled' ? 'secondary' : 'outline'}>
              {props.lottery.mode === 'scheduled' ? t('Scheduled') : t('One-time')}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <div className='grid gap-3 sm:grid-cols-3'>
          <Metric icon={<Gift />} label={t('Prize')} value={props.lottery.prize_name} />
          <Metric
            icon={<Trophy />}
            label={t('Winners')}
            value={String(props.lottery.winner_count)}
          />
          <Metric
            icon={<Users />}
            label={t('Participants')}
            value={String(props.lottery.participant_count)}
          />
        </div>

        {round && (
          <div className='bg-muted/40 grid gap-2 rounded-lg p-3 text-sm md:grid-cols-3'>
            <div>
              <div className='text-muted-foreground'>{t('Registration')}</div>
              <div>{formatTime(round.registration_start)}</div>
              <div>{formatTime(round.registration_end)}</div>
            </div>
            <div>
              <div className='text-muted-foreground'>{t('Draw time')}</div>
              <div>{formatTime(round.draw_time)}</div>
            </div>
            <div>
              <div className='text-muted-foreground'>{t('Status')}</div>
              <Badge variant='outline' className='gap-1.5'>
                <span className={`size-2 rounded-full ${roundDotClass}`} />
                {getRoundStatusLabel(round.status, t)}
              </Badge>
            </div>
          </div>
        )}

        {props.lottery.mode === 'scheduled' && (
          <div className='text-muted-foreground flex items-center gap-2 text-sm'>
            <CalendarDays />
            <span>
              {(props.lottery.schedule_weekdays ?? [])
                .map((day) => weekdayLabel(day, t))
                .join(', ')}{' '}
              {props.lottery.schedule_start_time}-{props.lottery.schedule_end_time}
            </span>
          </div>
        )}

        <div className='flex flex-col gap-2'>
          <div className='flex items-center justify-between gap-2'>
            <span className='text-sm font-medium'>{t('Recent participants')}</span>
            <span className='text-muted-foreground text-sm'>
              {t('{{count}} total', {
                count: props.lottery.participant_count,
              })}
            </span>
          </div>
          <div className='flex min-h-9 flex-wrap gap-2'>
            {props.lottery.participants.slice(0, 12).map((participant) => (
              <Badge key={participant.id} variant='secondary'>
                {participant.masked_name}
              </Badge>
            ))}
            {props.lottery.participants.length === 0 && (
              <span className='text-muted-foreground text-sm'>
                {t('No participants yet')}
              </span>
            )}
          </div>
        </div>

        {props.lottery.winners && props.lottery.winners.length > 0 && (
          <div className='bg-muted/40 flex flex-col gap-2 rounded-lg p-3'>
            <div className='flex items-center justify-between gap-2'>
              <span className='text-sm font-medium'>{t('Winners')}</span>
            </div>
            <div className='flex flex-wrap gap-2'>
              {props.lottery.winners.map((winner, index) => (
                <Badge
                  key={`${winner.masked_name}-${winner.won_at}-${index}`}
                  variant='outline'
                >
                  {winner.masked_name}
                </Badge>
              ))}
            </div>
          </div>
        )}

        {drawn && (!props.lottery.winners || props.lottery.winners.length === 0) && (
          <div className='text-muted-foreground text-sm'>
            {t('This draw has been completed without winners.')}
          </div>
        )}

        {props.lottery.eligibility && !props.lottery.eligibility.eligible && !drawn && (
          <EligibilityStatus status={props.lottery.eligibility} compact />
        )}

        <Button disabled={!canJoin || props.joining || drawn} onClick={props.onJoin} className='w-full sm:w-fit'>
          <Sparkles data-icon='inline-start' />
          {actionLabel}
        </Button>
      </CardContent>
    </Card>
  )
}
