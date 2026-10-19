import dayjs from 'dayjs'
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
import { Clock3, Eye, EyeOff, KeyRound, Search } from 'lucide-react'
import { type ReactNode, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { useSystemConfigStore } from '@/stores/system-config-store'

type TokenUsage = {
  name?: string
  total_granted?: number
  total_used?: number
  total_available?: number
  unlimited_quota?: boolean
  model_limits?: Record<string, boolean>
  expires_at?: number
  quota_period?: string
  period_limit?: number
  period_used?: number
  period_remaining?: number
  period_start?: number
  recent_requests?: RecentRequest[]
  recent_requests_error?: boolean
}

type RecentRequest = {
  request_id: string
  created_at: number
  model_name: string
  prompt_tokens: number
  completion_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  cache_write_5m_tokens: number
  cache_write_1h_tokens: number
  text_input_tokens: number
  text_output_tokens: number
  audio_input_tokens: number
  audio_output_tokens: number
  quota: number
}

type TokenUsageResponse = {
  code?: boolean
  message?: string
  data?: TokenUsage
}

const PERIOD_LABELS: Record<string, string> = {
  day: 'Daily',
  week: 'Weekly',
  month: 'Monthly',
}

function finiteNumber(value: unknown): number | null {
  const numberValue = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(numberValue) ? numberValue : null
}

function quotaPercent(used: number, limit: number): number {
  if (limit <= 0) return 0
  return Math.min(100, Math.max(0, (used / limit) * 100))
}

export function BalanceQuery() {
  const { t } = useTranslation()
  const [token, setToken] = useState('')
  const [showToken, setShowToken] = useState(false)
  const [usage, setUsage] = useState<TokenUsage | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const requestVersion = useRef(0)
  const currencyConfig = useSystemConfigStore((state) => state.config.currency)

  const formatQuota = (value: unknown): string => {
    const numberValue = finiteNumber(value)
    if (numberValue === null) return '-'
    const amount = numberValue / Math.max(currencyConfig.quotaPerUnit, 1)
    if (currencyConfig.quotaDisplayType === 'TOKENS') {
      return new Intl.NumberFormat(undefined, {
        notation: 'compact',
        maximumFractionDigits: 2,
      }).format(numberValue)
    }
    if (currencyConfig.quotaDisplayType === 'CNY') {
      return `¥${(amount * currencyConfig.usdExchangeRate).toFixed(amount >= 1 ? 2 : 4)}`
    }
    if (currencyConfig.quotaDisplayType === 'CUSTOM') {
      return `${currencyConfig.customCurrencySymbol}${(amount * currencyConfig.customCurrencyExchangeRate).toFixed(amount >= 1 ? 2 : 4)}`
    }
    return `$${amount.toFixed(amount >= 1 ? 2 : 4)}`
  }

  const formatDate = (timestamp: unknown): string => {
    const seconds = finiteNumber(timestamp)
    return seconds && seconds > 0
      ? dayjs.unix(seconds).format('YYYY-MM-DD HH:mm')
      : t('No expiration')
  }

  const formatCharge = (quota: number): string => {
    const usd = quota / Math.max(currencyConfig.quotaPerUnit, 1)
    let symbol = '$'
    let amount = usd
    if (currencyConfig.quotaDisplayType === 'CNY') {
      symbol = '¥'
      amount *= currencyConfig.usdExchangeRate
    } else if (currencyConfig.quotaDisplayType === 'CUSTOM') {
      symbol = currencyConfig.customCurrencySymbol
      amount *= currencyConfig.customCurrencyExchangeRate
    }
    let digits = 6
    if (amount >= 1) digits = 2
    else if (amount >= 0.0001) digits = 4
    return `${symbol}${amount.toFixed(digits)}`
  }

  const period = usage?.quota_period || ''
  const periodLimit = finiteNumber(usage?.period_limit)
  const periodUsed = finiteNumber(usage?.period_used) ?? 0
  const periodRemaining =
    finiteNumber(usage?.period_remaining) ??
    (periodLimit === null ? null : Math.max(0, periodLimit - periodUsed))
  const hasPeriodQuota = Boolean(period && periodLimit && periodLimit > 0)
  const periodProgress =
    hasPeriodQuota && periodLimit ? quotaPercent(periodUsed, periodLimit) : 0
  const modelLimits = useMemo(
    () => Object.keys(usage?.model_limits || {}),
    [usage?.model_limits]
  )
  let quotaModeLabel = t('Total quota')
  if (hasPeriodQuota) quotaModeLabel = t('Periodic quota')
  else if (usage?.unlimited_quota) quotaModeLabel = t('Unlimited quota')

  const queryBalance = async (): Promise<void> => {
    const trimmedToken = token.trim()
    if (!trimmedToken) {
      requestVersion.current += 1
      setError(t('Please enter an API key'))
      setUsage(null)
      return
    }

    setLoading(true)
    setError('')
    const version = ++requestVersion.current
    try {
      const response = await fetch('/api/usage/token/', {
        method: 'GET',
        cache: 'no-store',
        headers: {
          Accept: 'application/json',
          Authorization: `Bearer ${trimmedToken}`,
        },
      })
      const body = (await response
        .json()
        .catch(() => null)) as TokenUsageResponse | null
      if (!response.ok || body?.code === false || !body?.data) {
        throw new Error(body?.message || t('Unable to query balance'))
      }
      if (version === requestVersion.current) setUsage(body.data)
    } catch (queryError) {
      if (version === requestVersion.current) {
        setUsage(null)
        setError(
          queryError instanceof Error
            ? queryError.message
            : t('Unable to query balance')
        )
      }
    } finally {
      if (version === requestVersion.current) setLoading(false)
    }
  }

  return (
    <PublicLayout showMainContainer={false}>
      <main className='bg-muted/20 min-h-[calc(100vh-4rem)] px-4 py-12 sm:px-6 lg:px-8'>
        <div className='mx-auto grid w-full max-w-5xl gap-8 lg:grid-cols-[0.8fr_1.2fr] lg:items-start'>
          <section className='space-y-5 pt-2'>
            <div className='space-y-3'>
              <h1 className='text-3xl font-semibold tracking-tight sm:text-4xl'>
                {t('Balance Query')}
              </h1>
            </div>
            <div className='bg-background/70 text-muted-foreground flex items-start gap-3 rounded-lg border p-4 text-sm'>
              <KeyRound
                className='text-primary mt-0.5 size-4 shrink-0'
                aria-hidden='true'
              />
              <p>
                {t(
                  'Your key is used only for this request and is not stored by the page.'
                )}
              </p>
            </div>
          </section>

          <Card className='border-border/70 shadow-sm'>
            <CardHeader>
              <CardTitle>{t('Enter API Key')}</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='relative'>
                <Input
                  aria-label={t('API Key')}
                  type={showToken ? 'text' : 'password'}
                  value={token}
                  onChange={(event) => {
                    requestVersion.current += 1
                    setToken(event.target.value)
                    setUsage(null)
                    setLoading(false)
                  }}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') void queryBalance()
                  }}
                  placeholder='sk-...'
                  autoComplete='off'
                  className='h-10 pr-10 font-mono text-sm'
                />
                <button
                  type='button'
                  aria-label={showToken ? t('Hide API key') : t('Show API key')}
                  className='text-muted-foreground hover:text-foreground absolute top-1/2 right-2 -translate-y-1/2 rounded-md p-1.5'
                  onClick={() => setShowToken((value) => !value)}
                >
                  {showToken ? (
                    <EyeOff className='size-4' />
                  ) : (
                    <Eye className='size-4' />
                  )}
                </button>
              </div>
              <Button
                type='button'
                className='h-10 w-full'
                onClick={() => void queryBalance()}
                disabled={loading}
              >
                <Search className='size-4' aria-hidden='true' />
                {loading ? t('Querying...') : t('Query')}
              </Button>
              {error && (
                <p role='alert' className='text-destructive text-sm'>
                  {error}
                </p>
              )}
            </CardContent>
          </Card>

          {usage && (
            <section className='space-y-4 lg:col-span-2'>
              <Card className='border-border/70 shadow-sm'>
                <CardHeader className='flex-row items-center justify-between border-b'>
                  <div>
                    <CardTitle>{usage.name || t('Unnamed key')}</CardTitle>
                    <p className='text-muted-foreground mt-1 text-xs'>
                      {t('Expires at')}: {formatDate(usage.expires_at)}
                    </p>
                  </div>
                  <div className='bg-primary/10 text-primary rounded-full px-3 py-1 text-xs font-medium'>
                    {quotaModeLabel}
                  </div>
                </CardHeader>
                <CardContent className='pt-5'>
                  {hasPeriodQuota ? (
                    <div className='space-y-5'>
                      <div className='flex items-center justify-between gap-3 text-sm'>
                        <span className='font-medium'>
                          {t(PERIOD_LABELS[period] || 'Quota period')}
                        </span>
                        <span className='text-muted-foreground'>
                          {formatQuota(periodRemaining)} /{' '}
                          {formatQuota(periodLimit)}
                        </span>
                      </div>
                      <Progress
                        value={periodProgress}
                        aria-label={t('Periodic quota usage')}
                      />
                      <div className='grid gap-3 sm:grid-cols-3'>
                        <QuotaMetric
                          label={t('Period limit')}
                          value={formatQuota(periodLimit)}
                        />
                        <QuotaMetric
                          label={t('Period used')}
                          value={formatQuota(periodUsed)}
                        />
                        <QuotaMetric
                          label={t('Period remaining')}
                          value={formatQuota(periodRemaining)}
                        />
                      </div>
                      {usage.period_start ? (
                        <div className='text-muted-foreground flex items-center gap-2 text-xs'>
                          <Clock3 className='size-3.5' aria-hidden='true' />
                          {t('Current period started')}:{' '}
                          {formatDate(usage.period_start)}
                        </div>
                      ) : null}
                    </div>
                  ) : (
                    <div className='grid gap-3 sm:grid-cols-3'>
                      <QuotaMetric
                        label={t('Total quota')}
                        value={
                          usage.unlimited_quota
                            ? t('Unlimited quota')
                            : formatQuota(usage.total_granted)
                        }
                      />
                      <QuotaMetric
                        label={t('Used quota')}
                        value={formatQuota(usage.total_used)}
                      />
                      <QuotaMetric
                        label={t('Remaining quota')}
                        value={
                          usage.unlimited_quota
                            ? t('Unlimited quota')
                            : formatQuota(usage.total_available)
                        }
                      />
                    </div>
                  )}
                </CardContent>
              </Card>

              {modelLimits.length > 0 && (
                <Card className='border-border/70 shadow-sm'>
                  <CardHeader>
                    <CardTitle>{t('Model limits')}</CardTitle>
                  </CardHeader>
                  <CardContent className='flex flex-wrap gap-2'>
                    {modelLimits.map((model) => (
                      <span
                        key={model}
                        className='bg-muted/40 rounded-md border px-2.5 py-1 font-mono text-xs'
                      >
                        {model}
                      </span>
                    ))}
                  </CardContent>
                </Card>
              )}

              <RecentRequests
                requests={usage.recent_requests}
                error={usage.recent_requests_error}
                formatCharge={formatCharge}
              />
            </section>
          )}
        </div>
      </main>
    </PublicLayout>
  )
}

function QuotaMetric(props: { label: string; value: string }) {
  return (
    <div className='bg-muted/20 rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 text-lg font-semibold tabular-nums'>
        {props.value}
      </div>
    </div>
  )
}

function RecentRequests(props: {
  requests?: RecentRequest[]
  error?: boolean
  formatCharge: (quota: number) => string
}) {
  const { t } = useTranslation()
  let content: ReactNode
  if (props.error) {
    content = (
      <p role='status' className='text-muted-foreground text-sm'>
        {t('Request history is temporarily unavailable')}
      </p>
    )
  } else if (!props.requests?.length) {
    content = (
      <p role='status' className='text-muted-foreground text-sm'>
        {t('No recent requests')}
      </p>
    )
  } else {
    content = (
      <ul className='flex flex-col gap-3'>
        {props.requests.slice(0, 20).map((request) => {
          const cacheWriteTokens = Math.max(
            request.cache_write_tokens || 0,
            (request.cache_write_5m_tokens || 0) +
              (request.cache_write_1h_tokens || 0)
          )
          const tokenDetails = [
            [t('Input tokens'), request.prompt_tokens],
            [t('Output tokens'), request.completion_tokens],
            [t('Cache read'), request.cache_read_tokens],
            [t('Cache write'), cacheWriteTokens],
            [t('Cache write 5m'), request.cache_write_5m_tokens],
            [t('Cache write 1h'), request.cache_write_1h_tokens],
            [t('Text input'), request.text_input_tokens],
            [t('Text output'), request.text_output_tokens],
            [t('Audio input'), request.audio_input_tokens],
            [t('Audio output'), request.audio_output_tokens],
          ] as const
          const visibleDetails = tokenDetails.filter(([, count]) => count > 0)
          const fallbackKey = `${request.created_at}-${request.model_name}-${request.quota}-${request.prompt_tokens}-${request.completion_tokens}`
          return (
            <li
              key={request.request_id || fallbackKey}
              className='rounded-lg border p-4'
            >
              <div className='flex flex-wrap items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <span className='font-medium break-all'>
                      {request.model_name || '-'}
                    </span>
                  </div>
                  <div className='text-muted-foreground mt-1 text-xs tabular-nums'>
                    {dayjs
                      .unix(request.created_at)
                      .format('YYYY-MM-DD HH:mm:ss')}
                  </div>
                </div>
                <div className='text-right'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Actual charge')}
                  </div>
                  <div className='font-semibold tabular-nums'>
                    {props.formatCharge(request.quota)}
                  </div>
                </div>
              </div>
              {visibleDetails.length > 0 ? (
                <div className='text-muted-foreground mt-3 flex flex-wrap gap-x-4 gap-y-1 border-t pt-3 text-xs'>
                  {visibleDetails.map(([label, count]) => (
                    <span key={label}>
                      {label}:{' '}
                      <span className='text-foreground font-medium tabular-nums'>
                        {count.toLocaleString()}
                      </span>
                    </span>
                  ))}
                </div>
              ) : null}
            </li>
          )
        })}
      </ul>
    )
  }
  return (
    <Card className='border-border/70 shadow-sm'>
      <CardHeader>
        <CardTitle>{t('Recent requests')}</CardTitle>
        <p className='text-muted-foreground text-sm'>
          {t('Latest 20 requests for this key')}
        </p>
      </CardHeader>
      <CardContent>{content}</CardContent>
    </Card>
  )
}
