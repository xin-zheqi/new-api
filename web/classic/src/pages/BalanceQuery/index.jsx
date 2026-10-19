/*
Copyright (C) 2025 QuantumNous

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
import React, { useContext, useMemo, useRef, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { Button, Card, Progress, Tag, Typography } from '@douyinfe/semi-ui';
import { IconKey, IconSearch } from '@douyinfe/semi-icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import { getCurrencyConfig, renderQuota } from '../../helpers';
import { StatusContext } from '../../context/Status';
import ApiKeyInput from './ApiKeyInput';

const { Text, Title } = Typography;

const PERIOD_LABELS = {
  day: 'Daily',
  week: 'Weekly',
  month: 'Monthly',
};

function finiteNumber(value) {
  const numberValue = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numberValue) ? numberValue : null;
}

function formatDate(timestamp, noExpiration) {
  const seconds = finiteNumber(timestamp);
  return seconds && seconds > 0
    ? dayjs.unix(seconds).format('YYYY-MM-DD HH:mm')
    : noExpiration;
}

function formatCharge(quota) {
  const quotaPerUnit = Number(localStorage.getItem('quota_per_unit')) || 500000;
  const { symbol, rate } = getCurrencyConfig();
  const amount = (Number(quota) / quotaPerUnit) * rate;
  const digits = amount >= 1 ? 2 : amount >= 0.0001 ? 4 : 6;
  return `${symbol}${amount.toFixed(digits)}`;
}

function Metric({ label, value }) {
  return (
    <div
      style={{
        border: '1px solid var(--semi-color-border)',
        borderRadius: 8,
        background: 'var(--semi-color-fill-0)',
        padding: '14px 16px',
      }}
    >
      <Text type='tertiary' size='small'>
        {label}
      </Text>
      <div style={{ marginTop: 5, fontSize: 20, fontWeight: 700 }}>{value}</div>
    </div>
  );
}

export function RecentRequests({ requests, error }) {
  const { t } = useTranslation();
  return (
    <Card
      style={{
        marginTop: 16,
        border: '1px solid var(--semi-color-border)',
        borderRadius: 12,
      }}
      bodyStyle={{ padding: 24 }}
    >
      <Title heading={3} style={{ margin: 0 }}>
        {t('Recent requests')}
      </Title>
      <Text type='tertiary' size='small'>
        {t('Latest 20 requests for this key')}
      </Text>
      {error || !requests?.length ? (
        <div role='status' style={{ marginTop: 24 }}>
          <Text type='tertiary'>
            {error
              ? t('Request history is temporarily unavailable')
              : t('No recent requests')}
          </Text>
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 10, marginTop: 20 }}>
          {requests.slice(0, 20).map((request) => {
            const cacheWriteTokens = Math.max(
              request.cache_write_tokens || 0,
              (request.cache_write_5m_tokens || 0) +
                (request.cache_write_1h_tokens || 0),
            );
            const details = [
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
            ].filter(([, count]) => count > 0);
            return (
              <div
                data-testid='recent-request'
                key={
                  request.request_id ||
                  `${request.created_at}-${request.model_name}-${request.quota}-${request.prompt_tokens}`
                }
                style={{
                  border: '1px solid var(--semi-color-border)',
                  borderRadius: 8,
                  padding: '14px 16px',
                }}
              >
                <div
                  style={{
                    display: 'flex',
                    flexWrap: 'wrap',
                    justifyContent: 'space-between',
                    gap: 12,
                  }}
                >
                  <div style={{ minWidth: 0 }}>
                    <Text strong style={{ overflowWrap: 'anywhere' }}>
                      {request.model_name || '-'}
                    </Text>
                    <time
                      dateTime={dayjs.unix(request.created_at).toISOString()}
                      style={{ display: 'block', marginTop: 4 }}
                    >
                      <Text type='tertiary' size='small'>
                        {dayjs
                          .unix(request.created_at)
                          .format('YYYY-MM-DD HH:mm:ss')}
                      </Text>
                    </time>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <Text
                      type='tertiary'
                      size='small'
                      style={{ display: 'block' }}
                    >
                      {t('Actual charge')}
                    </Text>
                    <Text strong>{formatCharge(request.quota)}</Text>
                  </div>
                </div>
                {details.length > 0 ? (
                  <div
                    style={{
                      display: 'flex',
                      flexWrap: 'wrap',
                      gap: '4px 16px',
                      borderTop: '1px solid var(--semi-color-border)',
                      marginTop: 12,
                      paddingTop: 10,
                    }}
                  >
                    {details.map(([label, count]) => (
                      <Text key={label} type='tertiary' size='small'>
                        {label}:{' '}
                        <Text strong>{Number(count).toLocaleString()}</Text>
                      </Text>
                    ))}
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}

export default function BalanceQuery() {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const [token, setToken] = useState('');
  const [usage, setUsage] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const requestVersion = useRef(0);

  const period = usage?.quota_period || '';
  const periodLimit = finiteNumber(usage?.period_limit);
  const periodUsed = finiteNumber(usage?.period_used) || 0;
  const periodRemaining =
    finiteNumber(usage?.period_remaining) ??
    (periodLimit === null ? null : Math.max(0, periodLimit - periodUsed));
  const hasPeriodQuota = Boolean(period && periodLimit && periodLimit > 0);
  const progress = hasPeriodQuota
    ? Math.min(100, Math.max(0, (periodUsed / periodLimit) * 100))
    : 0;
  const modelLimits = useMemo(
    () => Object.keys(usage?.model_limits || {}),
    [usage?.model_limits],
  );

  const balanceEnabled = useMemo(() => {
    const raw = statusState?.status?.HeaderNavModules;
    if (!raw) return true;
    try {
      const modules = JSON.parse(raw);
      return modules.balance !== false;
    } catch {
      return true;
    }
  }, [statusState?.status?.HeaderNavModules]);

  if (!balanceEnabled) return <Navigate to='/' replace />;

  async function queryBalance() {
    const trimmedToken = token.trim();
    if (!trimmedToken) {
      requestVersion.current += 1;
      setError(t('Please enter an API key'));
      setUsage(null);
      return;
    }

    setLoading(true);
    setError('');
    const version = ++requestVersion.current;
    try {
      const response = await fetch('/api/usage/token/', {
        method: 'GET',
        cache: 'no-store',
        headers: {
          Accept: 'application/json',
          Authorization: `Bearer ${trimmedToken}`,
        },
      });
      const body = await response.json().catch(() => null);
      const { code, message, data } = body || {};
      if (!response.ok || code === false || !data) {
        throw new Error(message || t('Unable to query balance'));
      }
      if (version === requestVersion.current) setUsage(data);
    } catch (queryError) {
      if (version === requestVersion.current) {
        setUsage(null);
        setError(queryError?.message || t('Unable to query balance'));
      }
    } finally {
      if (version === requestVersion.current) setLoading(false);
    }
  }

  return (
    <div className='mt-[60px] px-2'>
      <div style={{ maxWidth: 980, margin: '0 auto', padding: '36px 0 56px' }}>
        <Card
          style={{
            border: '1px solid var(--semi-color-border)',
            borderRadius: 12,
            background: 'var(--semi-color-bg-1)',
          }}
          bodyStyle={{ padding: 28 }}
        >
          <Title heading={1} style={{ margin: '0 0 20px' }}>
            {t('Balance Query')}
          </Title>

          <div
            style={{
              marginTop: 28,
              display: 'flex',
              gap: 10,
              alignItems: 'center',
            }}
          >
            <IconKey
              style={{ color: 'var(--semi-color-text-2)', flex: '0 0 auto' }}
            />
            <ApiKeyInput
              value={token}
              onChange={(value) => {
                requestVersion.current += 1;
                setToken(value);
                setUsage(null);
                setLoading(false);
              }}
              onEnterPress={queryBalance}
            />
            <Button
              theme='solid'
              type='primary'
              icon={<IconSearch />}
              loading={loading}
              onClick={queryBalance}
            >
              {loading ? t('Querying...') : t('Query')}
            </Button>
          </div>
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 10 }}
          >
            {t(
              'Your key is used only for this request and is not stored by the page.',
            )}
          </Text>
          {error && (
            <Text type='danger' style={{ display: 'block', marginTop: 12 }}>
              {error}
            </Text>
          )}
        </Card>

        {usage && (
          <>
            <Card
              style={{
                marginTop: 16,
                border: '1px solid var(--semi-color-border)',
                borderRadius: 12,
              }}
              bodyStyle={{ padding: 24 }}
            >
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  gap: 12,
                  alignItems: 'flex-start',
                  flexWrap: 'wrap',
                }}
              >
                <div>
                  <Title heading={3} style={{ margin: 0 }}>
                    {usage.name || t('Unnamed key')}
                  </Title>
                  <Text type='tertiary' size='small'>
                    {t('过期于')}:{' '}
                    {formatDate(usage.expires_at, t('No expiration'))}
                  </Text>
                </div>
                <Tag color={hasPeriodQuota ? 'blue' : 'green'}>
                  {hasPeriodQuota
                    ? t('Periodic quota')
                    : usage.unlimited_quota
                      ? t('Unlimited quota')
                      : t('Total quota')}
                </Tag>
              </div>

              {hasPeriodQuota ? (
                <div style={{ marginTop: 24 }}>
                  <div
                    style={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      marginBottom: 8,
                    }}
                  >
                    <Text strong>
                      {t(PERIOD_LABELS[period] || 'Quota period')}
                    </Text>
                    <Text type='tertiary'>
                      {renderQuota(periodRemaining)} /{' '}
                      {renderQuota(periodLimit)}
                    </Text>
                  </div>
                  <Progress percent={progress} showInfo={false} />
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns: 'repeat(auto-fit,minmax(160px,1fr))',
                      gap: 10,
                      marginTop: 16,
                    }}
                  >
                    <Metric
                      label={t('Period limit')}
                      value={renderQuota(periodLimit)}
                    />
                    <Metric
                      label={t('Period used')}
                      value={renderQuota(periodUsed)}
                    />
                    <Metric
                      label={t('Period remaining')}
                      value={renderQuota(periodRemaining)}
                    />
                  </div>
                  {usage.period_start ? (
                    <Text
                      type='tertiary'
                      size='small'
                      style={{ display: 'block', marginTop: 14 }}
                    >
                      {t('Current period started')}:{' '}
                      {formatDate(usage.period_start, t('No expiration'))}
                    </Text>
                  ) : null}
                </div>
              ) : (
                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(auto-fit,minmax(160px,1fr))',
                    gap: 10,
                    marginTop: 24,
                  }}
                >
                  <Metric
                    label={t('Total quota')}
                    value={
                      usage.unlimited_quota
                        ? t('Unlimited quota')
                        : renderQuota(usage.total_granted)
                    }
                  />
                  <Metric
                    label={t('Used quota')}
                    value={renderQuota(usage.total_used)}
                  />
                  <Metric
                    label={t('剩余额度')}
                    value={
                      usage.unlimited_quota
                        ? t('Unlimited quota')
                        : renderQuota(usage.total_available)
                    }
                  />
                </div>
              )}

              {modelLimits.length > 0 && (
                <div style={{ marginTop: 22 }}>
                  <Text strong>{t('Model limits')}</Text>
                  <div
                    style={{
                      display: 'flex',
                      flexWrap: 'wrap',
                      gap: 8,
                      marginTop: 10,
                    }}
                  >
                    {modelLimits.map((model) => (
                      <Tag key={model}>{model}</Tag>
                    ))}
                  </div>
                </div>
              )}
            </Card>
            <RecentRequests
              requests={usage.recent_requests}
              error={usage.recent_requests_error}
            />
          </>
        )}
      </div>
    </div>
  );
}
