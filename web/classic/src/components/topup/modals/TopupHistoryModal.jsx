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
import React, { useState, useEffect, useMemo } from 'react';
import {
  Modal,
  Table,
  Badge,
  Typography,
  Toast,
  Empty,
  Button,
  Input,
  Tag,
  Select,
  DatePicker,
  Switch,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Coins } from 'lucide-react';
import { IconSearch } from '@douyinfe/semi-icons';
import { API, timestamp2string } from '../../../helpers';
import { isAdmin, isRoot } from '../../../helpers/utils';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
const { Text } = Typography;

// 状态映射配置
const STATUS_CONFIG = {
  success: { type: 'success', key: '成功' },
  pending: { type: 'warning', key: '待支付' },
  failed: { type: 'danger', key: '失败' },
  expired: { type: 'danger', key: '已过期' },
};

// 支付方式映射
const PAYMENT_METHOD_MAP = {
  stripe: 'Stripe',
  creem: 'Creem',
  waffo: 'Waffo',
  alipay: '支付宝',
  wxpay: '微信',
};

const toUnixSeconds = (value) => {
  if (!value) return 0;
  const date = value instanceof Date ? value : new Date(value);
  const timestamp = date.getTime();
  if (!Number.isFinite(timestamp)) return 0;
  return Math.floor(timestamp / 1000);
};

const TopupHistoryModal = ({ visible, onCancel, t }) => {
  const [loading, setLoading] = useState(false);
  const [topups, setTopups] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [keyword, setKeyword] = useState('');
  const [dateRange, setDateRange] = useState([]);
  const [manualModalVisible, setManualModalVisible] = useState(false);
  const [manualCreating, setManualCreating] = useState(false);
  const [manualUsersLoading, setManualUsersLoading] = useState(false);
  const [manualUsers, setManualUsers] = useState([]);
  const [manualUserKeyword, setManualUserKeyword] = useState('');
  const [manualForm, setManualForm] = useState({
    user_id: undefined,
    payment_method: 'bank_transfer',
    amount: 1,
    money: 0,
    create_time: new Date(),
    credit_balance: false,
  });
  const isMobile = useIsMobile();

  const loadTopups = async (currentPage, currentPageSize) => {
    setLoading(true);
    try {
      const base = isAdmin() ? '/api/user/topup' : '/api/user/topup/self';
      const startTime = toUnixSeconds(dateRange?.[0]);
      const endTime = toUnixSeconds(dateRange?.[1]);
      const qs =
        `p=${currentPage}&page_size=${currentPageSize}` +
        (keyword ? `&keyword=${encodeURIComponent(keyword)}` : '') +
        (startTime ? `&start_time=${startTime}` : '') +
        (endTime ? `&end_time=${endTime}` : '');
      const endpoint = `${base}?${qs}`;
      const res = await API.get(endpoint);
      const { success, message, data } = res.data;
      if (success) {
        setTopups(data.items || []);
        setTotal(data.total || 0);
      } else {
        Toast.error({ content: message || t('加载失败') });
      }
    } catch (error) {
      Toast.error({ content: t('加载账单失败') });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible) {
      loadTopups(page, pageSize);
    }
  }, [visible, page, pageSize, keyword, dateRange]);

  const handlePageChange = (currentPage) => {
    setPage(currentPage);
  };

  const handlePageSizeChange = (currentPageSize) => {
    setPageSize(currentPageSize);
    setPage(1);
  };

  const handleKeywordChange = (value) => {
    setKeyword(value);
    setPage(1);
  };

  const handleDateRangeChange = (value) => {
    setDateRange(value || []);
    setPage(1);
  };

  // 管理员补单
  const handleAdminComplete = async (tradeNo) => {
    try {
      const res = await API.post('/api/user/topup/complete', {
        trade_no: tradeNo,
      });
      const { success, message } = res.data;
      if (success) {
        Toast.success({ content: t('补单成功') });
        await loadTopups(page, pageSize);
      } else {
        Toast.error({ content: message || t('补单失败') });
      }
    } catch (e) {
      Toast.error({ content: t('补单失败') });
    }
  };

  const confirmAdminComplete = (tradeNo) => {
    Modal.confirm({
      title: t('确认补单'),
      content: t('是否将该订单标记为成功并为用户入账？'),
      onOk: () => handleAdminComplete(tradeNo),
    });
  };

  const resetManualForm = () => {
    setManualForm({
      user_id: undefined,
      payment_method: 'bank_transfer',
      amount: 1,
      money: 0,
      create_time: new Date(),
      credit_balance: false,
    });
    setManualUserKeyword('');
  };

  const openManualModal = () => {
    resetManualForm();
    setManualModalVisible(true);
  };

  const closeManualModal = () => {
    setManualModalVisible(false);
    resetManualForm();
  };

  const loadManualUsers = async () => {
    setManualUsersLoading(true);
    try {
      const keyword = manualUserKeyword.trim();
      const endpoint = keyword
        ? `/api/user/search?keyword=${encodeURIComponent(keyword)}&p=1&page_size=20`
        : '/api/user/?p=1&page_size=20';
      const res = await API.get(endpoint);
      const { success, data } = res.data;
      setManualUsers(success ? data.items || [] : []);
    } catch (e) {
      setManualUsers([]);
    } finally {
      setManualUsersLoading(false);
    }
  };

  useEffect(() => {
    if (!manualModalVisible) return;
    const timer = setTimeout(loadManualUsers, 250);
    return () => clearTimeout(timer);
  }, [manualModalVisible, manualUserKeyword]);

  const getManualCreateTimestamp = () => {
    const value = manualForm.create_time;
    const date = value instanceof Date ? value : new Date(value);
    const timestamp = Math.floor(date.getTime() / 1000);
    return Number.isFinite(timestamp) ? timestamp : 0;
  };

  const handleCreateManualTopup = async () => {
    const payload = {
      user_id: Number(manualForm.user_id || 0),
      payment_method: String(manualForm.payment_method || '').trim(),
      amount: Number(manualForm.amount || 0),
      money: Number(manualForm.money || 0),
      create_time: getManualCreateTimestamp(),
      credit_balance: !!manualForm.credit_balance,
    };
    if (
      payload.user_id <= 0 ||
      !payload.payment_method ||
      payload.amount <= 0 ||
      payload.money < 0 ||
      payload.create_time <= 0
    ) {
      Toast.error({ content: t('请完整填写充值记录信息') });
      return;
    }
    if (payload.payment_method.length > 50) {
      Toast.error({ content: t('支付方式不能超过 50 个字符') });
      return;
    }

    setManualCreating(true);
    try {
      const res = await API.post('/api/user/topup/manual', payload);
      const { success, message } = res.data;
      if (success) {
        Toast.success({ content: t('创建充值记录成功') });
        closeManualModal();
        await loadTopups(page, pageSize);
      } else {
        Toast.error({ content: message || t('创建充值记录失败') });
      }
    } catch (e) {
      Toast.error({ content: t('创建充值记录失败') });
    } finally {
      setManualCreating(false);
    }
  };

  // 渲染状态徽章
  const renderStatusBadge = (status) => {
    const config = STATUS_CONFIG[status] || { type: 'primary', key: status };
    return (
      <span className='flex items-center gap-2'>
        <Badge dot type={config.type} />
        <span>{t(config.key)}</span>
      </span>
    );
  };

  // 渲染支付方式
  const renderPaymentMethod = (pm) => {
    const displayName = PAYMENT_METHOD_MAP[pm];
    return <Text>{displayName ? t(displayName) : pm || '-'}</Text>;
  };

  const isSubscriptionTopup = (record) => {
    const tradeNo = (record?.trade_no || '').toLowerCase();
    return Number(record?.amount || 0) === 0 && tradeNo.startsWith('sub');
  };

  // 检查是否为管理员
  const userIsAdmin = useMemo(() => isAdmin(), []);
  const userIsRoot = useMemo(() => isRoot(), []);

  const columns = useMemo(() => {
    const baseColumns = [
      ...(userIsAdmin
        ? [
            {
              title: t('用户ID'),
              dataIndex: 'user_id',
              key: 'user_id',
              render: (userId) => <Text>{userId ?? '-'}</Text>,
            },
          ]
        : []),
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
        render: (text) => <Text copyable>{text}</Text>,
      },
      {
        title: t('支付方式'),
        dataIndex: 'payment_method',
        key: 'payment_method',
        render: renderPaymentMethod,
      },
      {
        title: t('充值额度'),
        dataIndex: 'amount',
        key: 'amount',
        render: (amount, record) => {
          if (isSubscriptionTopup(record)) {
            return (
              <Tag color='purple' shape='circle' size='small'>
                {t('订阅套餐')}
              </Tag>
            );
          }
          return (
            <span className='flex items-center gap-1'>
              <Coins size={16} />
              <Text>{amount}</Text>
            </span>
          );
        },
      },
      {
        title: t('支付金额'),
        dataIndex: 'money',
        key: 'money',
        render: (money) => <Text type='danger'>¥{money.toFixed(2)}</Text>,
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        key: 'status',
        render: renderStatusBadge,
      },
    ];

    // 管理员才显示操作列
    if (userIsAdmin) {
      baseColumns.push({
        title: t('操作'),
        key: 'action',
        render: (_, record) => {
          const actions = [];
          if (record.status === 'pending') {
            actions.push(
              <Button
                key="complete"
                size='small'
                type='primary'
                theme='outline'
                onClick={() => confirmAdminComplete(record.trade_no)}
              >
                {t('补单')}
              </Button>
            );
          }
          return actions.length > 0 ? <>{actions}</> : null;
        },
      });
    }

    baseColumns.push({
      title: t('创建时间'),
      dataIndex: 'create_time',
      key: 'create_time',
      render: (time) => timestamp2string(time),
    });

    return baseColumns;
  }, [t, userIsAdmin]);

  return (
    <>
      <Modal
        title={t('充值账单')}
        visible={visible}
        onCancel={onCancel}
        footer={null}
        size={isMobile ? 'full-width' : 'large'}
      >
        <div className='mb-3 flex flex-col gap-2 md:flex-row'>
          <Input
            prefix={<IconSearch />}
            placeholder={t('订单号')}
            value={keyword}
            onChange={handleKeywordChange}
            showClear
            style={{ flex: 1 }}
          />
          <DatePicker
            type='dateTimeRange'
            value={dateRange}
            onChange={handleDateRangeChange}
            showClear
            placeholder={[t('开始时间'), t('结束时间')]}
            style={{ width: isMobile ? '100%' : 320 }}
          />
          {userIsRoot && (
            <Button type='primary' onClick={openManualModal}>
              {t('创建充值记录')}
            </Button>
          )}
        </div>
        <Table
          columns={columns}
          dataSource={topups}
          loading={loading}
          rowKey='id'
          pagination={{
            currentPage: page,
            pageSize: pageSize,
            total: total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: handlePageChange,
            onPageSizeChange: handlePageSizeChange,
          }}
          size='small'
          empty={
            <Empty
              image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('暂无充值记录')}
              style={{ padding: 30 }}
            />
          }
        />
      </Modal>

      {userIsRoot && (
        <Modal
          title={t('创建充值记录')}
          visible={manualModalVisible}
          onCancel={closeManualModal}
          onOk={handleCreateManualTopup}
          confirmLoading={manualCreating}
          okText={t('创建')}
          cancelText={t('取消')}
          size={isMobile ? 'full-width' : 'medium'}
        >
          <div className='flex flex-col gap-4'>
            <div>
              <Text strong>{t('用户')}</Text>
              <Input
                className='mt-2'
                prefix={<IconSearch />}
                placeholder={t('搜索用户ID或用户名')}
                value={manualUserKeyword}
                onChange={setManualUserKeyword}
                showClear
              />
              <Select
                className='mt-2 w-full'
                placeholder={manualUsersLoading ? t('加载中') : t('选择用户')}
                value={manualForm.user_id}
                onChange={(value) =>
                  setManualForm({ ...manualForm, user_id: value })
                }
                loading={manualUsersLoading}
                filter
              >
                {manualUsers.map((user) => (
                  <Select.Option key={user.id} value={user.id}>
                    {`ID ${user.id} · ${user.username}`}
                  </Select.Option>
                ))}
              </Select>
            </div>

            <div>
              <Text strong>{t('支付方式')}</Text>
              <Input
                className='mt-2'
                value={manualForm.payment_method}
                onChange={(value) =>
                  setManualForm({ ...manualForm, payment_method: value })
                }
                placeholder='bank_transfer'
              />
            </div>

            <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
              <div>
                <Text strong>{t('充值额度')}</Text>
                <Input
                  className='mt-2'
                  type='number'
                  min={1}
                  value={manualForm.amount}
                  onChange={(value) =>
                    setManualForm({ ...manualForm, amount: value })
                  }
                />
              </div>
              <div>
                <Text strong>{t('支付金额')}</Text>
                <Input
                  className='mt-2'
                  type='number'
                  min={0}
                  value={manualForm.money}
                  onChange={(value) =>
                    setManualForm({ ...manualForm, money: value })
                  }
                />
              </div>
            </div>

            <div>
              <Text strong>{t('创建时间')}</Text>
              <DatePicker
                className='mt-2 w-full'
                type='dateTime'
                value={manualForm.create_time}
                onChange={(value) =>
                  setManualForm({
                    ...manualForm,
                    create_time: value || new Date(),
                  })
                }
              />
            </div>

            <div className='flex items-center justify-between rounded border p-3'>
              <div className='flex flex-col'>
                <Text strong>{t('同步增加余额')}</Text>
                <Text type='tertiary' size='small'>
                  {t('开启后会立即为用户增加余额')}
                </Text>
              </div>
              <Switch
                checked={manualForm.credit_balance}
                onChange={(checked) =>
                  setManualForm({ ...manualForm, credit_balance: checked })
                }
              />
            </div>
          </div>
        </Modal>
      )}
    </>
  );
};

export default TopupHistoryModal;
