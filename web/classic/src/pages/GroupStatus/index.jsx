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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Spin,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { API, showError } from '../../helpers';

const { Text, Title } = Typography;

const RATE_COLORS = {
  red: [239, 68, 68],
  yellow: [245, 158, 11],
  green: [34, 197, 94],
};

function interpolateColor(start, end, ratio) {
  const clampedRatio = Math.max(0, Math.min(1, ratio));
  const values = start.map((value, index) =>
    Math.round(value + (end[index] - value) * clampedRatio),
  );
  return `rgb(${values.join(', ')})`;
}

function getRateColor(rate) {
  const safeRate = Number.isFinite(rate)
    ? Math.max(0, Math.min(100, rate))
    : 100;
  if (safeRate <= 50) {
    return interpolateColor(RATE_COLORS.red, RATE_COLORS.yellow, safeRate / 50);
  }
  return interpolateColor(
    RATE_COLORS.yellow,
    RATE_COLORS.green,
    (safeRate - 50) / 50,
  );
}

function formatRate(rate) {
  const safeRate = Number.isFinite(rate) ? rate : 100;
  return `${safeRate.toFixed(safeRate === 100 || safeRate === 0 ? 0 : 1)}%`;
}

function formatTimeSpan(bucket) {
  return `${dayjs.unix(bucket.start_ts).format('HH:mm')} - ${dayjs
    .unix(bucket.end_ts)
    .format('HH:mm')}`;
}

function TimelineBar({ bucket }) {
  const { t } = useTranslation();
  const rate = bucket?.success_rate ?? 100;
  const color = getRateColor(rate);

  return (
    <Tooltip
      content={
        <div style={{ minWidth: 160 }}>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>
            {formatTimeSpan(bucket)}
          </div>
          <div>
            {t('成功率')}: {formatRate(rate)}
          </div>
          <div>
            {t('成功请求')}: {bucket.success} / {bucket.total}
          </div>
        </div>
      }
      position='top'
    >
      <div
        aria-label={`${formatTimeSpan(bucket)} ${formatRate(rate)}`}
        style={{
          width: '100%',
          minWidth: 6,
          height: 34,
          borderRadius: 4,
          background: color,
        }}
      />
    </Tooltip>
  );
}

function GroupStatusCard({ group }) {
  const { t } = useTranslation();
  const currentRate = group.current_rate ?? 100;
  const buckets = group.buckets || [];
  const timelineGridColumns = `repeat(${Math.max(buckets.length, 1)}, minmax(6px, 1fr))`;

  return (
    <Card
      style={{
        border: '1px solid var(--semi-color-border)',
        borderRadius: 8,
        background: 'var(--semi-color-bg-1)',
      }}
      bodyStyle={{ padding: 20 }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
          gap: 20,
          flexWrap: 'wrap',
        }}
      >
        <div
          style={{
            minWidth: 210,
            flex: '0 0 210px',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'flex-start',
          }}
        >
          <Title
            heading={2}
            style={{
              margin: 0,
              lineHeight: 1.15,
              textAlign: 'left',
              width: '100%',
            }}
          >
            {group.group}
          </Title>
          <div
            style={{
              marginTop: 12,
              textAlign: 'left',
              width: '100%',
            }}
          >
            <Text type='tertiary' size='small'>
              {t('当前成功率')}
            </Text>
            <div
              style={{
                marginTop: 3,
                color: getRateColor(currentRate),
                fontSize: 22,
                fontWeight: 700,
                lineHeight: 1.15,
              }}
            >
              {formatRate(currentRate)}
            </div>
          </div>
        </div>

        <div
          style={{
            flex: '1 1 320px',
            minWidth: 0,
          }}
        >
          <div
            style={{
              overflowX: 'auto',
              padding: '4px 0',
            }}
          >
            <div
              style={{
                minWidth: buckets.length * 9,
                width: '100%',
              }}
            >
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: timelineGridColumns,
                  alignItems: 'end',
                  gap: 3,
                }}
              >
                {buckets.map((bucket) => (
                  <TimelineBar
                    key={`${bucket.start_ts}_${bucket.end_ts}`}
                    bucket={bucket}
                  />
                ))}
              </div>
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: timelineGridColumns,
                  gap: 3,
                  color: 'var(--semi-color-text-2)',
                  fontSize: 12,
                  paddingTop: 4,
                  width: '100%',
                }}
              >
                <span style={{ gridColumn: 1, justifySelf: 'start' }}>
                  {buckets[0]
                    ? dayjs.unix(buckets[0].start_ts).format('HH:mm')
                    : ''}
                </span>
                <span
                  style={{
                    gridColumn: buckets.length || 1,
                    justifySelf: 'end',
                  }}
                >
                  {buckets.length
                    ? dayjs
                        .unix(buckets[buckets.length - 1].end_ts)
                        .format('HH:mm')
                    : ''}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Card>
  );
}

export default function GroupStatus() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [statusData, setStatusData] = useState(null);

  const loadStatus = useCallback(
    async (manual = false) => {
      if (manual) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      try {
        const res = await API.get('/api/group/status', {
          disableDuplicate: true,
        });
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('状态数据加载失败'));
          return;
        }
        setStatusData(data);
      } catch (error) {
        showError(t('状态数据加载失败'));
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [t],
  );

  useEffect(() => {
    loadStatus();
    const timer = setInterval(() => loadStatus(true), 60000);
    return () => clearInterval(timer);
  }, [loadStatus]);

  const groups = statusData?.groups || [];
  const lastUpdatedAt = useMemo(() => dayjs().format('HH:mm:ss'), [statusData]);

  return (
    <div className='mt-[60px] px-2'>
      <div
        style={{
          maxWidth: 1180,
          margin: '0 auto',
          padding: '24px 0 40px',
        }}
      >
        <Card
          style={{
            marginBottom: 16,
            borderRadius: 8,
            border: '1px solid var(--semi-color-border)',
          }}
          bodyStyle={{ padding: 20 }}
        >
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: 16,
              flexWrap: 'wrap',
            }}
          >
            <div>
              <Title heading={3} style={{ margin: 0 }}>
                {t('分组状态')}
              </Title>
            </div>
            <Button
              icon={<IconRefresh />}
              type='tertiary'
              onClick={() => loadStatus(true)}
              loading={refreshing}
            >
              {t('刷新')}
            </Button>
          </div>
        </Card>

        <Spin spinning={loading}>
          {groups.length === 0 ? (
            <Card
              style={{
                borderRadius: 8,
                border: '1px solid var(--semi-color-border)',
              }}
            >
              <Empty
                title={t('暂无启用状态展示的分组')}
                description={t('请在分组设置中为需要展示的分组开启状态展示')}
              />
            </Card>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {groups.map((group) => (
                <GroupStatusCard key={group.group} group={group} />
              ))}
              <Text type='tertiary' size='small' style={{ textAlign: 'right' }}>
                {t('更新时间')}: {lastUpdatedAt}
              </Text>
            </div>
          )}
        </Spin>
      </div>
    </div>
  );
}
