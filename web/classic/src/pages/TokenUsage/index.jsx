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

import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Col,
  DatePicker,
  Empty,
  Input,
  Row,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { API, renderNumber, renderQuota, showError } from '../../helpers';
import CardPro from '../../components/common/ui/CardPro';
import CardTable from '../../components/common/ui/CardTable';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../helpers/utils';
import { IconSearch } from '@douyinfe/semi-icons';

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

function unix(date) {
  if (!date) return undefined;
  const value = date instanceof Date ? date.getTime() : new Date(date).getTime();
  if (!Number.isFinite(value)) return undefined;
  return Math.floor(value / 1000);
}

function getPresetRange(type) {
  const now = new Date();
  const start = new Date(now);
  const end = new Date(now);
  if (type === 'today') {
    start.setHours(0, 0, 0, 0);
    end.setHours(23, 59, 59, 999);
  } else if (type === 'yesterday') {
    start.setDate(start.getDate() - 1);
    start.setHours(0, 0, 0, 0);
    end.setDate(end.getDate() - 1);
    end.setHours(23, 59, 59, 999);
  } else if (type === '7d') {
    start.setDate(start.getDate() - 6);
    start.setHours(0, 0, 0, 0);
    end.setHours(23, 59, 59, 999);
  } else if (type === '30d') {
    start.setDate(start.getDate() - 29);
    start.setHours(0, 0, 0, 0);
    end.setHours(23, 59, 59, 999);
  } else if (type === 'month') {
    start.setDate(1);
    start.setHours(0, 0, 0, 0);
    end.setMonth(end.getMonth() + 1, 0);
    end.setHours(23, 59, 59, 999);
  } else {
    return [null, null];
  }
  return [start, end];
}

function formatTokens(value) {
  const n = Number(value || 0);
  if (n === 0) return '-';
  if (n < 1000) return String(n);
  if (n < 1000000) return `${(n / 1000).toFixed(1)}K`;
  return `${(n / 1000000).toFixed(2)}M`;
}

function statusTag(status, t) {
  if (status === 1) return <Tag color='green'>{t('已启用')}</Tag>;
  if (status === 2) return <Tag color='grey'>{t('已禁用')}</Tag>;
  if (status === 3) return <Tag color='orange'>{t('已过期')}</Tag>;
  if (status === 4) return <Tag color='red'>{t('已耗尽')}</Tag>;
  return <Tag>{t('未知状态')}</Tag>;
}

function formatDisplayKey(key) {
  if (!key) return 'sk-...';
  return key.startsWith('sk-') ? key : `sk-${key}`;
}

function UsagePie({ models }) {
  const { t } = useTranslation();
  const values = (models || [])
    .filter((model) => model.quota > 0)
    .slice(0, 12)
    .map((model) => ({
      model: model.model_name || t('未知模型'),
      quota: model.quota,
      quotaText: renderQuota(model.quota, 4),
    }));

  if (values.length === 0) {
    return (
      <div className='flex items-center justify-center h-48 border rounded-md text-semi-color-text-2'>
        {t('暂无用量数据')}
      </div>
    );
  }

  return (
    <div style={{ height: 240 }}>
      <VChart
        spec={{
          type: 'pie',
          data: [{ id: 'usage', values }],
          outerRadius: 0.78,
          innerRadius: 0.45,
          valueField: 'quota',
          categoryField: 'model',
          legends: { visible: true, orient: 'bottom' },
          tooltip: {
            mark: {
              content: [
                {
                  key: (datum) => datum.model,
                  value: (datum) => datum.quotaText,
                },
              ],
            },
          },
        }}
      />
    </div>
  );
}

function ExpandedRow({ record }) {
  const { t } = useTranslation();
  const columns = [
    {
      title: t('模型'),
      dataIndex: 'model_name',
      render: (text) => text || t('未知模型'),
    },
    {
      title: t('用量'),
      dataIndex: 'quota',
      align: 'right',
      render: (text) => renderQuota(text, 4),
    },
    {
      title: t('总 Tokens'),
      dataIndex: 'total_tokens',
      align: 'right',
      render: formatTokens,
    },
    {
      title: t('提示 Tokens'),
      dataIndex: 'prompt_tokens',
      align: 'right',
      render: formatTokens,
    },
    {
      title: t('补全 Tokens'),
      dataIndex: 'completion_tokens',
      align: 'right',
      render: formatTokens,
    },
    {
      title: t('请求次数'),
      dataIndex: 'requests',
      align: 'right',
      render: renderNumber,
    },
  ];

  return (
    <div className='p-3'>
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={8}>
          <div className='space-y-3'>
            <Typography.Text strong>{t('模型分布')}</Typography.Text>
            <UsagePie models={record.models} />
          </div>
        </Col>
        <Col xs={24} lg={16}>
          <div className='space-y-3'>
            <Typography.Text strong>{t('模型明细')}</Typography.Text>
            <Table
              columns={columns}
              dataSource={record.models || []}
              pagination={false}
              rowKey='model_name'
              size='small'
            />
          </div>
        </Col>
      </Row>
    </div>
  );
}

export default function TokenUsage() {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState([]);
  const [summary, setSummary] = useState({});
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [range, setRange] = useState(getPresetRange('7d'));

  const loadData = async (nextPage = page, nextSize = pageSize) => {
    setPage(nextPage);
    setPageSize(nextSize);
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('p', String(nextPage));
      params.set('page_size', String(nextSize));
      if (keyword.trim()) params.set('keyword', keyword.trim());
      const start = unix(range?.[0]);
      const end = unix(range?.[1]);
      if (start) params.set('start_timestamp', String(start));
      if (end) params.set('end_timestamp', String(end));
      const res = await API.get(`/api/token/usage/models?${params}`);
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('加载令牌用量失败'));
        return;
      }
      setItems(data?.page?.items || []);
      setTotal(data?.page?.total || 0);
      setSummary(data?.summary || {});
    } catch (error) {
      showError(error.message || t('加载令牌用量失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData(1, pageSize);
  }, [range]);

  const columns = useMemo(
    () => [
      {
        title: t('API Key'),
        dataIndex: 'key',
        render: (text, record) => (
          <div>
            <div className='font-medium'>
              {record.token_name || t('令牌名称')}
            </div>
            <Typography.Text type='tertiary' size='small' className='font-mono'>
              {formatDisplayKey(text)}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        render: (status) => statusTag(status, t),
      },
      {
        title: t('用量'),
        dataIndex: 'quota',
        align: 'right',
        render: (text) => renderQuota(text, 4),
      },
      {
        title: t('模型'),
        dataIndex: 'model_count',
        align: 'right',
      },
      {
        title: t('总 Tokens'),
        dataIndex: 'total_tokens',
        align: 'right',
        render: formatTokens,
      },
      {
        title: t('请求次数'),
        dataIndex: 'requests',
        align: 'right',
        render: renderNumber,
      },
    ],
    [t],
  );

  const statsArea = (
    <>
      <div className='flex flex-col md:flex-row justify-between items-start md:items-center gap-3 mb-3'>
        <div>
          <Typography.Title heading={5} className='!mb-0'>
            {t('令牌用量')}
          </Typography.Title>
          <Typography.Text type='tertiary'>
            {t('按令牌查看不同模型的用量分布')}
          </Typography.Text>
        </div>
        <Button
          theme='solid'
          loading={loading}
          onClick={() => loadData(1, pageSize)}
        >
          {t('刷新')}
        </Button>
      </div>
      <Row gutter={[12, 12]}>
        <Col xs={24} md={6}>
          <div className='rounded-xl border p-4 h-full'>
            <Typography.Text type='secondary'>{t('总用量')}</Typography.Text>
            <div className='text-xl font-semibold mt-1'>
              {renderQuota(summary.quota || 0, 4)}
            </div>
          </div>
        </Col>
        <Col xs={24} md={6}>
          <div className='rounded-xl border p-4 h-full'>
            <Typography.Text type='secondary'>{t('请求次数')}</Typography.Text>
            <div className='text-xl font-semibold mt-1'>
              {renderNumber(summary.requests || 0)}
            </div>
          </div>
        </Col>
        <Col xs={24} md={6}>
          <div className='rounded-xl border p-4 h-full'>
            <Typography.Text type='secondary'>{t('总 Tokens')}</Typography.Text>
            <div className='text-xl font-semibold mt-1'>
              {formatTokens(summary.total_tokens || 0)}
            </div>
          </div>
        </Col>
        <Col xs={24} md={6}>
          <div className='rounded-xl border p-4 h-full'>
            <Typography.Text type='secondary'>{t('活跃令牌')}</Typography.Text>
            <div className='text-xl font-semibold mt-1'>
              {summary.active_key_count || 0}/{summary.total_key_count || 0}
            </div>
          </div>
        </Col>
      </Row>
    </>
  );

  const searchArea = (
    <Space wrap>
      <Input
        prefix={<IconSearch />}
        placeholder={t('按令牌名称筛选')}
        value={keyword}
        onChange={setKeyword}
        style={{ width: 220 }}
        onEnterPress={() => {
          setPage(1);
          loadData(1, pageSize);
        }}
      />
      <DatePicker
        type='dateTimeRange'
        value={range}
        onChange={(value) => setRange(value || [null, null])}
        style={{ width: 360, maxWidth: '100%' }}
        showClear
      />
      <Button onClick={() => setRange(getPresetRange('today'))}>
        {t('今天')}
      </Button>
      <Button onClick={() => setRange(getPresetRange('yesterday'))}>
        {t('昨天')}
      </Button>
      <Button onClick={() => setRange(getPresetRange('7d'))}>
        {t('7天')}
      </Button>
      <Button onClick={() => setRange(getPresetRange('30d'))}>
        {t('30天')}
      </Button>
      <Button onClick={() => setRange(getPresetRange('month'))}>
        {t('本月')}
      </Button>
      <Button onClick={() => setRange(getPresetRange('all'))}>
        {t('全部时间')}
      </Button>
      <Button
        theme='solid'
        onClick={() => {
          setPage(1);
          loadData(1, pageSize);
        }}
      >
        {t('查询')}
      </Button>
    </Space>
  );

  return (
    <div className='mt-[60px] px-2'>
      <CardPro
        type='type2'
        statsArea={statsArea}
        searchArea={searchArea}
        paginationArea={createCardProPagination({
          currentPage: page,
          pageSize,
          total,
          onPageChange: (nextPage) => {
            setPage(nextPage);
            loadData(nextPage, pageSize);
          },
          onPageSizeChange: (nextSize) => {
            setPage(1);
            setPageSize(nextSize);
            loadData(1, nextSize);
          },
          isMobile,
          t,
        })}
        t={t}
      >
        <CardTable
          columns={columns}
          dataSource={items}
          loading={loading}
          rowKey='token_id'
          empty={<Empty description={t('暂无令牌用量')} />}
          expandedRowRender={(record) => <ExpandedRow record={record} />}
          hideExpandedColumn={false}
          expandRowByClick
          pagination={{
            currentPage: page,
            pageSize,
            total,
            pageSizeOpts: PAGE_SIZE_OPTIONS,
            showSizeChanger: true,
            onPageChange: (nextPage) => {
              setPage(nextPage);
              loadData(nextPage, pageSize);
            },
            onPageSizeChange: (nextSize) => {
              setPage(1);
              setPageSize(nextSize);
              loadData(1, nextSize);
            },
          }}
          hidePagination={true}
          className='rounded-xl overflow-hidden'
          size='small'
        />
      </CardPro>
    </div>
  );
}
