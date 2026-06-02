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
import {
  Banner,
  Button,
  Empty,
  Input,
  Modal,
  Popover,
  Progress,
  Select,
  SideSheet,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconPlusCircle, IconSearch } from '@douyinfe/semi-icons';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import CardPro from '../../common/ui/CardPro';
import CardTable from '../../common/ui/CardTable';
import { API, renderQuota, showError, showSuccess } from '../../../helpers';
import { normalizePlanRecords } from '../../../helpers/subscription';
import { convertUSDToCurrency } from '../../../helpers/render';
import { createCardProPagination } from '../../../helpers/utils';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { useTranslation } from 'react-i18next';

const { Paragraph, Text, Title } = Typography;

const STATUS_OPTIONS = [
  { label: '可用中', value: 'active' },
  { label: '已用完', value: 'exhausted' },
  { label: '已过期', value: 'expired' },
  { label: '已作废', value: 'cancelled' },
];

const SOURCE_OPTIONS = [
  { label: '订单购买', value: 'order' },
  { label: '余额开通', value: 'balance' },
  { label: '兑换码', value: 'redemption' },
  { label: '管理员', value: 'admin' },
];

function formatTs(ts) {
  if (!ts) return '-';
  return new Date(ts * 1000).toLocaleString();
}

const getQuotaProgressColor = (pct) => {
  if (pct === 100) return 'var(--semi-color-success)';
  if (pct <= 10) return 'var(--semi-color-danger)';
  if (pct <= 30) return 'var(--semi-color-warning)';
  return undefined;
};

function toFiniteNumber(value, fallback = 0) {
  const num = Number(value);
  return Number.isFinite(num) ? num : fallback;
}

function renderQuotaUsage(record, t) {
  const totalAmount = toFiniteNumber(record?.amount_total);
  const usedAmount = toFiniteNumber(record?.amount_used);
  const remainAmount =
    totalAmount > 0
      ? Math.max(
          0,
          toFiniteNumber(record?.amount_remaining, totalAmount - usedAmount),
        )
      : 0;

  if (totalAmount <= 0) {
    const popoverContent = (
      <div className='text-xs p-2'>
        <Paragraph copyable={{ content: renderQuota(usedAmount, 2) }}>
          {t('已用额度')}: {renderQuota(usedAmount, 2)}
        </Paragraph>
      </div>
    );
    return (
      <Popover content={popoverContent} position='top'>
        <Tag color='white' shape='circle'>
          {t('无限额度')}
        </Tag>
      </Popover>
    );
  }

  const percent = Math.min(
    100,
    Math.max(0, (remainAmount / totalAmount) * 100),
  );
  const popoverContent = (
    <div className='text-xs p-2'>
      <Paragraph copyable={{ content: renderQuota(usedAmount, 2) }}>
        {t('已用额度')}: {renderQuota(usedAmount, 2)}
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(remainAmount, 2) }}>
        {t('剩余额度')}: {renderQuota(remainAmount, 2)} ({percent.toFixed(0)}%)
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(totalAmount, 2) }}>
        {t('总额度')}: {renderQuota(totalAmount, 2)}
      </Paragraph>
    </div>
  );

  return (
    <Popover content={popoverContent} position='top'>
      <Tag color='white' shape='circle'>
        <div className='flex flex-col items-end'>
          <span className='text-xs leading-none'>
            {`${renderQuota(remainAmount, 2)} / ${renderQuota(totalAmount, 2)}`}
          </span>
          <Progress
            percent={percent}
            stroke={getQuotaProgressColor(percent)}
            aria-label='subscription quota usage'
            format={() => `${percent.toFixed(0)}%`}
            style={{ width: '100%', marginTop: '1px', marginBottom: 0 }}
          />
        </div>
      </Tag>
    </Popover>
  );
}

function getStatusMeta(status) {
  switch (status) {
    case 'active':
      return { color: 'green', text: '可用中' };
    case 'exhausted':
      return { color: 'orange', text: '已用完' };
    case 'expired':
      return { color: 'grey', text: '已过期' };
    case 'cancelled':
      return { color: 'grey', text: '已作废' };
    default:
      return { color: 'blue', text: status || '-' };
  }
}

function getSourceText(source) {
  return (
    SOURCE_OPTIONS.find((item) => item.value === source)?.label || source || '-'
  );
}

function getUserLabel(record) {
  const name = record?.display_name || record?.username || '-';
  return `${name} (ID: ${record?.user_id || '-'})`;
}

function canInvalidate(record) {
  return ['active', 'exhausted'].includes(record?.effective_status);
}

const UserSubscriptionsPage = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();

  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [plansLoading, setPlansLoading] = useState(false);

  const [records, setRecords] = useState([]);
  const [total, setTotal] = useState(0);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState();
  const [source, setSource] = useState();
  const [planId, setPlanId] = useState();

  const [plans, setPlans] = useState([]);
  const [createUserId, setCreateUserId] = useState('');
  const [createPlanId, setCreatePlanId] = useState();

  const [detailRecord, setDetailRecord] = useState(null);

  const planOptions = useMemo(() => {
    return (plans || []).map((item) => {
      const plan = item?.plan || item;
      const id = plan?.id;
      const title = plan?.title || `#${id}`;
      const price = convertUSDToCurrency(Number(plan?.price_amount || 0), 2);
      return {
        label: `${title} (${price})`,
        value: id,
      };
    });
  }, [plans]);

  const translatedStatusOptions = useMemo(
    () => STATUS_OPTIONS.map((item) => ({ ...item, label: t(item.label) })),
    [t],
  );

  const translatedSourceOptions = useMemo(
    () => SOURCE_OPTIONS.map((item) => ({ ...item, label: t(item.label) })),
    [t],
  );

  const loadPlans = async () => {
    setPlansLoading(true);
    try {
      const res = await API.get('/api/subscription/admin/plans');
      if (res.data?.success) {
        setPlans(normalizePlanRecords(res.data.data));
      } else {
        showError(res.data?.message || t('加载失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setPlansLoading(false);
    }
  };

  const loadRecords = async (page = activePage, size = pageSize, filters) => {
    setLoading(true);
    try {
      const currentKeyword = filters ? filters.keyword : keyword;
      const currentStatus = filters ? filters.status : status;
      const currentSource = filters ? filters.source : source;
      const currentPlanId = filters ? filters.planId : planId;
      const params = {
        p: page,
        page_size: size,
      };
      if (currentKeyword?.trim()) params.keyword = currentKeyword.trim();
      if (currentStatus) params.status = currentStatus;
      if (currentSource) params.source = currentSource;
      if (currentPlanId) params.plan_id = currentPlanId;

      const res = await API.get('/api/subscription/admin/user_subscriptions', {
        params,
      });
      if (res.data?.success) {
        const data = res.data.data || {};
        setRecords(Array.isArray(data.items) ? data.items : []);
        setTotal(Number(data.total || 0));
        setActivePage(Number(data.page || page));
        setPageSize(Number(data.page_size || size));
      } else {
        showError(res.data?.message || t('加载失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadPlans();
  }, []);

  useEffect(() => {
    loadRecords(1, pageSize);
  }, []);

  const handleSearch = () => {
    loadRecords(1, pageSize);
  };

  const handleReset = () => {
    setKeyword('');
    setStatus(undefined);
    setSource(undefined);
    setPlanId(undefined);
    loadRecords(1, pageSize, {
      keyword: '',
      status: undefined,
      source: undefined,
      planId: undefined,
    });
  };

  const handlePageChange = (page) => {
    loadRecords(page, pageSize);
  };

  const handlePageSizeChange = (size) => {
    loadRecords(1, size);
  };

  const createSubscription = async () => {
    const userId = Number(createUserId);
    if (!Number.isInteger(userId) || userId <= 0) {
      showError(t('请输入有效的用户 ID'));
      return;
    }
    if (!createPlanId) {
      showError(t('请选择订阅套餐'));
      return;
    }

    setCreating(true);
    try {
      const res = await API.post(
        `/api/subscription/admin/users/${userId}/subscriptions`,
        {
          plan_id: createPlanId,
        },
      );
      if (res.data?.success) {
        const msg = res.data?.data?.message;
        showSuccess(msg || t('新增成功'));
        setCreateUserId('');
        setCreatePlanId(undefined);
        await loadRecords(1, pageSize);
      } else {
        showError(res.data?.message || t('新增失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setCreating(false);
    }
  };

  const invalidateSubscription = (record) => {
    if (!canInvalidate(record)) return;
    Modal.confirm({
      title: t('确认作废'),
      content: t('作废后该订阅将立即失效，历史记录仍会保留。是否继续？'),
      centered: true,
      okType: 'danger',
      onOk: async () => {
        try {
          const res = await API.post(
            `/api/subscription/admin/user_subscriptions/${record.id}/invalidate`,
          );
          if (res.data?.success) {
            const msg = res.data?.data?.message;
            showSuccess(msg || t('已作废'));
            await loadRecords(activePage, pageSize);
          } else {
            showError(res.data?.message || t('操作失败'));
          }
        } catch (error) {
          showError(t('请求失败'));
        }
      },
    });
  };

  const columns = useMemo(
    () => [
      {
        title: 'ID',
        dataIndex: 'id',
        key: 'id',
        width: 80,
      },
      {
        title: t('用户'),
        key: 'user',
        width: 220,
        render: (_, record) => (
          <div className='min-w-0'>
            <div className='font-medium truncate'>{getUserLabel(record)}</div>
            <div className='text-xs text-gray-500 truncate'>
              {record?.email || record?.username || '-'}
            </div>
          </div>
        ),
      },
      {
        title: t('套餐'),
        key: 'plan',
        width: 190,
        render: (_, record) => (
          <div className='min-w-0'>
            <div className='font-medium truncate'>
              {record?.plan_title || '-'}
            </div>
            <div className='text-xs text-gray-500'>
              {t('套餐 ID')}: {record?.plan_id || '-'}
            </div>
          </div>
        ),
      },
      {
        title: t('状态'),
        key: 'status',
        width: 100,
        render: (_, record) => {
          const meta = getStatusMeta(record?.effective_status);
          return (
            <Tag color={meta.color} shape='circle' size='small'>
              {t(meta.text)}
            </Tag>
          );
        },
      },
      {
        title: t('剩余额度/总额度'),
        key: 'usage',
        width: 190,
        render: (_, record) => renderQuotaUsage(record, t),
      },
      {
        title: t('有效期'),
        key: 'validity',
        width: 220,
        render: (_, record) => (
          <div className='text-xs text-gray-600'>
            <div>
              {t('开始')}: {formatTs(record?.start_time)}
            </div>
            <div>
              {t('结束')}: {formatTs(record?.end_time)}
            </div>
          </div>
        ),
      },
      {
        title: t('重置时间'),
        key: 'reset',
        width: 180,
        render: (_, record) => (
          <div className='text-xs text-gray-600'>
            <div>
              {t('上次')}: {formatTs(record?.last_reset_time)}
            </div>
            <div>
              {t('下次')}: {formatTs(record?.next_reset_time)}
            </div>
          </div>
        ),
      },
      {
        title: t('来源'),
        key: 'source',
        width: 100,
        render: (_, record) => t(getSourceText(record?.source)),
      },
      {
        title: t('创建时间'),
        key: 'created_at',
        width: 170,
        render: (_, record) => formatTs(record?.created_at),
      },
      {
        title: '',
        key: 'operate',
        width: 150,
        fixed: 'right',
        render: (_, record) => (
          <Space>
            <Button
              size='small'
              type='tertiary'
              theme='light'
              onClick={() => setDetailRecord(record)}
            >
              {t('详情')}
            </Button>
            {canInvalidate(record) ? (
              <Button
                size='small'
                type='danger'
                theme='light'
                onClick={() => invalidateSubscription(record)}
              >
                {t('作废')}
              </Button>
            ) : null}
          </Space>
        ),
      },
    ],
    [t, activePage, pageSize],
  );

  const descriptionArea = (
    <div className='flex flex-col gap-2'>
      <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-2'>
        <div>
          <Title heading={4} className='m-0'>
            {t('订阅管理')}
          </Title>
          <Text type='secondary'>
            {t('集中查看和作废用户已购买/已开通的订阅记录')}
          </Text>
        </div>
        <Tag color='blue' shape='circle'>
          {t('用户订阅')}
        </Tag>
      </div>
    </div>
  );

  const actionsArea = (
    <div className='flex flex-col md:flex-row md:items-center gap-2 w-full'>
      <Input
        value={createUserId}
        onChange={setCreateUserId}
        placeholder={t('用户 ID')}
        style={{ width: isMobile ? '100%' : 150 }}
      />
      <Select
        value={createPlanId}
        onChange={setCreatePlanId}
        optionList={planOptions}
        placeholder={t('选择订阅套餐')}
        loading={plansLoading}
        filter
        showClear
        style={{ width: isMobile ? '100%' : 320 }}
      />
      <Button
        type='primary'
        theme='solid'
        icon={<IconPlusCircle />}
        loading={creating}
        onClick={createSubscription}
      >
        {t('开通订阅')}
      </Button>
    </div>
  );

  const searchArea = (
    <div className='flex flex-col md:flex-row md:items-center gap-2 w-full'>
      <Input
        value={keyword}
        onChange={setKeyword}
        prefix={<IconSearch />}
        placeholder={t('搜索用户、邮箱、套餐、ID')}
        showClear
        style={{ width: isMobile ? '100%' : 280 }}
        onEnterPress={handleSearch}
      />
      <Select
        value={status}
        onChange={setStatus}
        optionList={translatedStatusOptions}
        placeholder={t('状态')}
        showClear
        style={{ width: isMobile ? '100%' : 150 }}
      />
      <Select
        value={source}
        onChange={setSource}
        optionList={translatedSourceOptions}
        placeholder={t('来源')}
        showClear
        style={{ width: isMobile ? '100%' : 150 }}
      />
      <Select
        value={planId}
        onChange={setPlanId}
        optionList={planOptions}
        placeholder={t('套餐')}
        loading={plansLoading}
        filter
        showClear
        style={{ width: isMobile ? '100%' : 240 }}
      />
      <div className='flex gap-2 w-full md:w-auto'>
        <Button
          type='tertiary'
          loading={loading}
          onClick={handleSearch}
          className='flex-1 md:flex-initial'
        >
          {t('查询')}
        </Button>
        <Button
          type='tertiary'
          onClick={handleReset}
          className='flex-1 md:flex-initial'
        >
          {t('重置')}
        </Button>
      </div>
    </div>
  );

  const detailItems = detailRecord
    ? [
        [t('订阅 ID'), detailRecord.id],
        [t('用户'), getUserLabel(detailRecord)],
        [t('邮箱'), detailRecord.email || '-'],
        [t('套餐'), detailRecord.plan_title || '-'],
        [t('套餐 ID'), detailRecord.plan_id || '-'],
        [t('状态'), t(getStatusMeta(detailRecord.effective_status).text)],
        [t('来源'), t(getSourceText(detailRecord.source))],
        [
          t('总额度'),
          detailRecord.amount_total > 0
            ? renderQuota(detailRecord.amount_total, 2)
            : t('不限'),
        ],
        [t('已用额度'), renderQuota(detailRecord.amount_used || 0, 2)],
        [t('剩余额度'), renderQuota(detailRecord.amount_remaining || 0, 2)],
        [t('开始时间'), formatTs(detailRecord.start_time)],
        [t('结束时间'), formatTs(detailRecord.end_time)],
        [t('上次重置'), formatTs(detailRecord.last_reset_time)],
        [t('下次重置'), formatTs(detailRecord.next_reset_time)],
        [t('升级分组'), detailRecord.upgrade_group || '-'],
        [t('原用户分组'), detailRecord.prev_user_group || '-'],
        [t('创建时间'), formatTs(detailRecord.created_at)],
        [t('更新时间'), formatTs(detailRecord.updated_at)],
      ]
    : [];

  return (
    <>
      <CardPro
        type='type1'
        descriptionArea={descriptionArea}
        actionsArea={actionsArea}
        searchArea={searchArea}
        paginationArea={createCardProPagination({
          currentPage: activePage,
          pageSize,
          total,
          onPageChange: handlePageChange,
          onPageSizeChange: handlePageSizeChange,
          isMobile,
          t,
        })}
        t={t}
      >
        <CardTable
          columns={columns}
          dataSource={records}
          loading={loading}
          rowKey={(row) => row?.id}
          scroll={{ x: 'max-content' }}
          pagination={false}
          hidePagination={true}
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('暂无订阅记录')}
              style={{ padding: 30 }}
            />
          }
          size='middle'
        />
      </CardPro>

      <SideSheet
        visible={!!detailRecord}
        placement='right'
        width={isMobile ? '100%' : 560}
        title={t('订阅详情')}
        onCancel={() => setDetailRecord(null)}
      >
        <div className='flex flex-col gap-3'>
          {detailItems.map(([label, value]) => (
            <div
              key={label}
              className='flex justify-between gap-4 py-2 border-b border-dashed'
              style={{ borderColor: 'var(--semi-color-border)' }}
            >
              <Text type='secondary'>{label}</Text>
              <Text strong className='text-right break-all'>
                {value}
              </Text>
            </div>
          ))}
          {detailRecord && !canInvalidate(detailRecord) ? (
            <Banner
              type='info'
              description={t('该订阅已过期或已作废，仅保留记录，不可操作。')}
              closeIcon={null}
              className='!rounded-lg mt-2'
            />
          ) : null}
        </div>
      </SideSheet>
    </>
  );
};

export default UserSubscriptionsPage;
