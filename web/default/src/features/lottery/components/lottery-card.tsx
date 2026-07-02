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
import type { ReactNode } from 'react'
import {
  Award,
  CalendarDays,
  CheckCircle2,
  Clock3,
  Gift,
  Sparkles,
  Trophy,
  UserRound,
  Users,
  WalletCards,
} from 'lucide-react'
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
  getRoundStatusLabel,
  isRoundDrawn,
  weekdayLabel,
} from '../utils'
import { EligibilityStatus } from './eligibility-status'
import { Metric } from './metric'

function formatLotteryAmount(value: number) {
  const amount = Number(value || 0)
  if (!Number.isFinite(amount)) return '$0'
  return `$${amount.toLocaleString(undefined, {
    minimumFractionDigits: amount % 1 === 0 ? 0 : 2,
    maximumFractionDigits: 2,
  })}`
}

function ConditionStat(props: {
  label: string
  value: string
  icon?: ReactNode
  tone?: 'default' | 'primary'
}) {
  const toneClass =
    props.tone === 'primary'
      ? 'border-primary/20 bg-primary/5'
      : 'border-border/60 bg-background'

  return (
    <div className={`flex min-w-0 items-start gap-3 rounded-lg border p-3 ${toneClass}`}>
      {props.icon && (
        <div className='text-muted-foreground mt-0.5 shrink-0 [&_svg]:size-4'>
          {props.icon}
        </div>
      )}
      <div className='min-w-0 flex-1'>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
        <div className='truncate text-sm font-medium'>{props.value}</div>
      </div>
    </div>
  )
}

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
  const hasConditions =
    props.lottery.require_recharge ||
    props.lottery.min_recharge_amount > 0 ||
    props.lottery.recharge_window_days > 0 ||
    props.lottery.count_redemption_as_recharge ||
    props.lottery.min_account_age_days > 0 ||
    props.lottery.min_request_count > 0 ||
    props.lottery.require_email_verified
  const winnerPreview = props.lottery.winners?.slice(0, 5) ?? []
  const winnerOverflow = Math.max(
    (props.lottery.winners?.length ?? 0) - winnerPreview.length,
    0
  )

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
            <Badge
              variant={props.lottery.mode === 'scheduled' ? 'secondary' : 'outline'}
            >
              {props.lottery.mode === 'scheduled' ? t('Scheduled') : t('One-time')}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <div className='grid gap-3 sm:grid-cols-3'>
          <Metric
            icon={<Gift />}
            label={t('Prize')}
            value={props.lottery.prize_name}
          />
          <Metric
            icon={<Trophy />}
            label={t('Winner count')}
            value={String(props.lottery.winner_count)}
          />
          <Metric
            icon={<Users />}
            label={t('Participants')}
            value={String(props.lottery.participant_count)}
          />
        </div>

        {hasConditions && (
          <div className='border-border/60 bg-muted/20 flex flex-col gap-4 rounded-lg border p-4'>
            <div className='flex items-center gap-2'>
              <CheckCircle2 className='text-primary size-4 shrink-0' />
              <span className='text-sm font-medium'>
                {t('Participation conditions')}
              </span>
            </div>

            {(props.lottery.require_recharge ||
              props.lottery.min_recharge_amount > 0 ||
              props.lottery.recharge_window_days > 0 ||
              props.lottery.count_redemption_as_recharge) && (
              <div className='border-border/70 bg-background/70 flex flex-col gap-3 rounded-xl border p-4'>
                <div className='flex flex-wrap items-start justify-between gap-2'>
                  <div className='flex min-w-0 items-center gap-2'>
                    <WalletCards className='text-primary size-4 shrink-0' />
                    <div className='min-w-0'>
                      <div className='text-sm font-medium'>
                        {t('Recharge requirement')}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        {props.lottery.recharge_window_days > 0
                          ? t('Valid recharge within the last {{days}} days', {
                              days: props.lottery.recharge_window_days,
                            })
                          : t('Valid recharge from any time')}
                      </div>
                    </div>
                  </div>
                  {props.lottery.count_redemption_as_recharge && (
                    <Badge variant='secondary'>{t('Redemption codes count')}</Badge>
                  )}
                </div>

                <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-3'>
                  <ConditionStat
                    icon={<Gift />}
                    label={t('Required amount')}
                    value={
                      props.lottery.min_recharge_amount > 0
                        ? formatLotteryAmount(props.lottery.min_recharge_amount)
                        : t('Any successful recharge')
                    }
                    tone='primary'
                  />
                  <ConditionStat
                    icon={<Clock3 />}
                    label={t('Recharge validity window')}
                    value={
                      props.lottery.recharge_window_days > 0
                        ? t('{{count}} days', {
                            count: props.lottery.recharge_window_days,
                          })
                        : t('Valid recharge from any time')
                    }
                  />
                  <ConditionStat
                    icon={<Users />}
                    label={t('Recharge rule')}
                    value={t('Applied to this draw only')}
                  />
                </div>
              </div>
            )}

            <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-3'>
              {props.lottery.require_email_verified && (
                <ConditionStat
                  icon={<UserRound />}
                  label={t('Email condition')}
                  value={t('Email binding required')}
                />
              )}
              {props.lottery.min_account_age_days > 0 && (
                <ConditionStat
                  icon={<CalendarDays />}
                  label={t('Account age')}
                  value={t('{{count}} days', {
                    count: props.lottery.min_account_age_days,
                  })}
                />
              )}
              {props.lottery.min_request_count > 0 && (
                <ConditionStat
                  icon={<Users />}
                  label={t('Request count')}
                  value={t('{{count}} requests', {
                    count: props.lottery.min_request_count,
                  })}
                />
              )}
            </div>
          </div>
        )}

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
                <span className='bg-muted-foreground/55 size-2 rounded-full' />
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
              <span className='text-muted-foreground text-xs'>
                {t('{{count}} total', { count: props.lottery.winners.length })}
              </span>
            </div>
            <div className='flex flex-wrap gap-2'>
              {winnerPreview.map((winner, index) => (
                <Badge
                  key={`${winner.masked_name}-${winner.won_at}-${index}`}
                  variant='outline'
                >
                  {winner.masked_name}
                </Badge>
              ))}
              {winnerOverflow > 0 && (
                <Badge variant='secondary'>
                  {t('+{{count}} more', { count: winnerOverflow })}
                </Badge>
              )}
            </div>
          </div>
        )}

        {drawn && (!props.lottery.winners || props.lottery.winners.length === 0) && (
          <div className='text-muted-foreground text-sm'>
            {t('This draw has been completed without winners.')}
          </div>
        )}

        {props.lottery.eligibility && hasConditions && !drawn && (
          <EligibilityStatus status={props.lottery.eligibility} compact />
        )}

        <Button
          disabled={!canJoin || props.joining || drawn}
          onClick={props.onJoin}
          className='w-full sm:w-fit'
        >
          <Sparkles data-icon='inline-start' />
          {actionLabel}
        </Button>
      </CardContent>
    </Card>
  )
}
