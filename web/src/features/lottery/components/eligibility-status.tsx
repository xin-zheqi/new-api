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
import { AlertCircle, CheckCircle2, Clock3, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import type { LotteryEligibilityIssue, LotteryEligibilityStatus } from '../types'

function formatLotteryAmount(value?: number) {
  const amount = Number(value ?? 0)
  if (!Number.isFinite(amount)) return '$0'
  return `$${amount.toLocaleString(undefined, {
    minimumFractionDigits: amount % 1 === 0 ? 0 : 2,
    maximumFractionDigits: 2,
  })}`
}

function getIssueLabel(issue: LotteryEligibilityIssue, t: (key: string) => string) {
  if (issue.code === 'email_required') {
    return t('Bind an email address before joining draws')
  }
  if (issue.code === 'account_age_required') {
    return t('Account registration age does not meet the draw requirement')
  }
  if (issue.code === 'request_count_required') {
    return t('Request count does not meet the draw requirement')
  }
  if (issue.code === 'recharge_required') {
    return t('Recharge condition not met, temporarily unable to join draws')
  }
  return issue.message || t('Participation requirement not met')
}

function RechargeRequirement(props: { issue: LotteryEligibilityIssue }) {
  const { t } = useTranslation()
  const requiredAmount = props.issue.required_amount ?? 0
  const currentAmount = props.issue.current_amount ?? 0
  const remainingAmount = props.issue.remaining_amount ?? Math.max(requiredAmount - currentAmount, 0)
  let progress = 0
  if (requiredAmount > 0) {
    progress = Math.min(100, Math.max(0, (currentAmount / requiredAmount) * 100))
  } else if (currentAmount > 0) {
    progress = 100
  }

  return (
    <div className='border-border bg-background rounded-lg border p-3'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex min-w-0 items-center gap-2'>
          <WalletCards className='text-primary size-4 shrink-0' />
          <div className='min-w-0'>
            <div className='text-sm font-medium'>{t('Recharge requirement')}</div>
            <div className='text-muted-foreground text-xs'>
              {props.issue.window_days && props.issue.window_days > 0
                ? t('Valid recharge within the last {{days}} days', {
                    days: props.issue.window_days,
                  })
                : t('Valid recharge from any time')}
            </div>
          </div>
        </div>
        {props.issue.count_redemption_as_recharge && (
          <Badge variant='secondary' className='w-fit'>
            {t('Redemption codes count')}
          </Badge>
        )}
      </div>

      {requiredAmount > 0 ? (
        <div className='mt-3 space-y-3'>
          <Progress value={progress} />
          <div className='grid gap-2 text-sm sm:grid-cols-3'>
            <div>
              <div className='text-muted-foreground text-xs'>{t('Required amount')}</div>
              <div className='font-medium'>{formatLotteryAmount(requiredAmount)}</div>
            </div>
            <div>
              <div className='text-muted-foreground text-xs'>{t('Credited amount')}</div>
              <div className='font-medium'>{formatLotteryAmount(currentAmount)}</div>
            </div>
            <div>
              <div className='text-muted-foreground text-xs'>{t('Still needed')}</div>
              <div className='text-destructive font-medium'>
                {formatLotteryAmount(remainingAmount)}
              </div>
            </div>
          </div>
        </div>
      ) : (
        <div className='text-muted-foreground mt-3 text-sm'>
          {t('A successful recharge record is required before joining draws.')}
        </div>
      )}
    </div>
  )
}

export function EligibilityStatus(props: {
  status: LotteryEligibilityStatus
  compact?: boolean
}) {
  const { t } = useTranslation()
  const issues = props.status.issues ?? []
  const rechargeIssue = issues.find((issue) => issue.code === 'recharge_required')
  const otherIssues = issues.filter((issue) => issue.code !== 'recharge_required')

  return (
    <div
      className={
        props.status.eligible
          ? 'border-emerald-500/30 bg-emerald-500/5 rounded-lg border p-4'
          : 'border-border bg-muted/30 rounded-lg border p-4'
      }
    >
      <div className='flex items-start gap-3'>
        {props.status.eligible ? (
          <CheckCircle2 className='mt-0.5 size-5 shrink-0 text-emerald-600' />
        ) : (
          <AlertCircle className='text-destructive mt-0.5 size-5 shrink-0' />
        )}
        <div className='min-w-0 flex-1'>
          <div className='text-sm font-medium'>
            {props.status.eligible
              ? t('You meet the current draw participation requirements')
              : t('You do not meet the current draw participation requirements')}
          </div>
          {!props.compact && (
            <div className='text-muted-foreground mt-1 text-sm'>
              {props.status.eligible
                ? t('You can join open draws when the registration period is active.')
                : t('Resolve the following requirements before joining open draws.')}
            </div>
          )}
        </div>
      </div>

      {!props.status.eligible && (
        <div className='mt-3 space-y-3'>
          {rechargeIssue && <RechargeRequirement issue={rechargeIssue} />}
          {otherIssues.length > 0 && (
            <div className='space-y-2'>
              {otherIssues.map((issue) => (
                <div
                  key={issue.code}
                  className='text-muted-foreground flex items-start gap-2 text-sm'
                >
                  <Clock3 className='mt-0.5 size-4 shrink-0' />
                  <span>{getIssueLabel(issue, t)}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
