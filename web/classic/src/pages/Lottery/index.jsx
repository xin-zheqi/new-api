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
  Card,
  DatePicker,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  TabPane,
  Table,
  Tabs,
  Tag,
  Toast,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Award,
  CalendarClock,
  Clock3,
  Edit3,
  Eye,
  Gift,
  Mail,
  Trash2,
  Trophy,
  UserRound,
  Users,
  WalletCards,
} from 'lucide-react';
import { API, isAdmin, timestamp2string } from '../../helpers';

const { Text, Title } = Typography;

const WEEKDAYS = [0, 1, 2, 3, 4, 5, 6];

const defaultForm = {
  title: '',
  description: '',
  prize_name: '',
  mode: 'once',
  winner_count: 1,
  prize_per_winner: 1,
  require_recharge: false,
  min_recharge_amount: 0,
  recharge_window_days: 0,
  count_redemption_as_recharge: false,
  min_account_age_days: 0,
  min_request_count: 0,
  require_email_verified: false,
  registration_start: null,
  registration_end: null,
  draw_time: null,
  schedule_weekdays: [1, 3, 5],
  schedule_start_time: '09:00',
  schedule_end_time: '18:00',
  schedule_draw_time: '20:00',
  prize_codes: '',
};

function toUnixSeconds(value) {
  if (!value) return 0;
  const date = value instanceof Date ? value : new Date(value);
  const timestamp = date.getTime();
  if (!Number.isFinite(timestamp)) return 0;
  return Math.floor(timestamp / 1000);
}

function formatLotteryTime(value) {
  return value ? timestamp2string(value) : '-';
}

function formatLotteryRoundLabel(mode, round, t) {
  if (!round) return '-';
  if (mode === 'once') return t('一次性开奖');
  const key = String(round.round_key || '');
  if (/^\d{8}$/.test(key)) {
    return `${key.slice(0, 4)}-${key.slice(4, 6)}-${key.slice(6)}`;
  }
  if (round.draw_time) {
    return new Date(round.draw_time * 1000).toLocaleDateString();
  }
  return key || '-';
}

function dateFromUnix(value) {
  return value ? new Date(value * 1000) : null;
}

function parsePrizeCodes(value) {
  const seen = new Set();
  const codes = [];
  String(value || '')
    .split(/\r?\n/)
    .forEach((rawCode) => {
      const code = rawCode.trim();
      if (!code || seen.has(code)) return;
      seen.add(code);
      codes.push(code);
    });
  return codes;
}

function isRoundDrawn(status) {
  return status === 'finished' || status === 'insufficient_prizes';
}

function isRoundUndrawn(status) {
  return status === 'pending' || status === 'open' || status === 'drawing';
}

function roundStatusTag(status, t) {
  const config = {
    pending: ['grey', '待开始'],
    open: ['green', '报名中'],
    drawing: ['orange', '开奖中'],
    finished: ['blue', '已开奖'],
    cancelled: ['red', '已取消'],
    insufficient_prizes: ['red', '奖品不足'],
  }[status] || ['grey', status || '-'];
  return <Tag color={config[0]}>{t(config[1])}</Tag>;
}

function modeTag(mode, t) {
  return (
    <Tag color={mode === 'scheduled' ? 'purple' : 'blue'}>
      {mode === 'scheduled' ? t('定时抽奖') : t('一次性抽奖')}
    </Tag>
  );
}

function activityStatusTag(status, t) {
  if (status === 3) {
    return <Tag color='red'>{t('已删除')}</Tag>;
  }
  return (
    <Tag color={status === 1 ? 'green' : 'grey'}>
      {status === 1 ? t('启用') : t('停用')}
    </Tag>
  );
}

function formatLotteryAmount(value) {
  const amount = Number(value || 0);
  if (!Number.isFinite(amount)) return '$0';
  return `$${amount.toLocaleString(undefined, {
    minimumFractionDigits: amount % 1 === 0 ? 0 : 2,
    maximumFractionDigits: 2,
  })}`;
}

function getEligibilityIssueLabel(issue, t) {
  if (issue.code === 'email_required') {
    return t('需要绑定邮箱后才能参与抽奖');
  }
  if (issue.code === 'account_age_required') {
    return t('账号注册时间不满足参与条件');
  }
  if (issue.code === 'request_count_required') {
    return t('请求次数不满足参与条件');
  }
  if (issue.code === 'recharge_required') {
    return t('充值条件不满足，暂时不能参与抽奖');
  }
  return issue.message || t('当前暂不满足参与条件');
}

function EligibilityPanel({ eligibility, t, compact = false }) {
  if (!eligibility) return null;
  const issues = eligibility.issues || [];
  const rechargeIssue = issues.find((issue) => issue.code === 'recharge_required');
  const otherIssues = issues.filter((issue) => issue.code !== 'recharge_required');

  return (
    <div className='rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-4'>
      <Space vertical align='start' spacing={compact ? 8 : 10} style={{ width: '100%' }}>
        <Text strong>
          {eligibility.eligible ? t('你当前满足参与条件') : t('你当前不满足参与条件')}
        </Text>
        {eligibility.eligible ? (
          <Text type='secondary' size='small'>
            {t('只要活动仍在报名中，你就可以参与抽奖。')}
          </Text>
        ) : (
          <>
            {rechargeIssue && (
              <div className='w-full rounded border border-[var(--semi-color-border)] bg-white p-3'>
                <Space vertical align='start' spacing={8} style={{ width: '100%' }}>
                  <div className='flex w-full items-center justify-between gap-3'>
                    <div>
                      <Text strong>{t('充值条件')}</Text>
                      <div>
                        <Text type='secondary' size='small'>
                          {rechargeIssue.window_days > 0
                            ? t('需要在最近 {{days}} 天内完成符合条件的充值', {
                              days: rechargeIssue.window_days,
                            })
                            : t('需要至少有一笔符合条件的充值记录')}
                        </Text>
                      </div>
                    </div>
                    {rechargeIssue.count_redemption_as_recharge && (
                      <Tag>{t('兑换码兑换计入')}</Tag>
                    )}
                  </div>
                  {Number(rechargeIssue.required_amount || 0) > 0 ? (
                    <>
                      <div className='h-2 w-full overflow-hidden rounded-full bg-[var(--semi-color-fill-1)]'>
                        <div
                          className='h-full rounded-full bg-[var(--semi-color-primary)]'
                          style={{
                            width: `${Math.min(
                              100,
                              Math.max(
                                0,
                                ((Number(rechargeIssue.current_amount || 0) /
                                  Number(rechargeIssue.required_amount || 1)) *
                                  100),
                              ),
                            )}%`,
                          }}
                        />
                      </div>
                      <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
                        <div>
                          <Text type='secondary' size='small'>{t('需要金额')}</Text>
                          <div className='font-medium'>
                            {formatLotteryAmount(rechargeIssue.required_amount)}
                          </div>
                        </div>
                        <div>
                          <Text type='secondary' size='small'>{t('当前计入')}</Text>
                          <div className='font-medium'>
                            {formatLotteryAmount(rechargeIssue.current_amount)}
                          </div>
                        </div>
                        <div>
                          <Text type='secondary' size='small'>{t('还差金额')}</Text>
                          <div className='font-medium text-[var(--semi-color-danger)]'>
                            {formatLotteryAmount(rechargeIssue.remaining_amount)}
                          </div>
                        </div>
                      </div>
                    </>
                  ) : (
                    <Text type='secondary' size='small'>
                      {t('当前需要至少存在一笔成功充值记录后才能参与抽奖')}
                    </Text>
                  )}
                </Space>
              </div>
            )}
            {otherIssues.map((issue) => (
              <Text key={issue.code} type='secondary' size='small'>
                • {getEligibilityIssueLabel(issue, t)}
              </Text>
            ))}
          </>
        )}
      </Space>
    </div>
  );
}

function ConditionInfoItem({ icon, label, value, strong = false }) {
  return (
    <div className='flex min-w-0 items-start gap-3 rounded border border-[var(--semi-color-border)] bg-white p-3'>
      <div className='mt-0.5 shrink-0 text-[var(--semi-color-text-2)]'>{icon}</div>
      <div className='min-w-0'>
        <Text type='secondary' size='small'>
          {label}
        </Text>
        <div className={`truncate ${strong ? 'font-semibold' : 'font-medium'}`}>
          {value}
        </div>
      </div>
    </div>
  );
}

function DrawConditions({ lottery, t }) {
  const hasRechargeRule =
    lottery.require_recharge ||
    lottery.min_recharge_amount > 0 ||
    lottery.recharge_window_days > 0 ||
    lottery.count_redemption_as_recharge;
  const rows = [];
  if (lottery.min_account_age_days > 0) {
    rows.push({
      label: t('账号天数'),
      value: t('至少 {{count}} 天', { count: lottery.min_account_age_days }),
      icon: <UserRound size={16} />,
    });
  }
  if (lottery.min_request_count > 0) {
    rows.push({
      label: t('请求次数'),
      value: t('至少 {{count}} 次', { count: lottery.min_request_count }),
      icon: <Users size={16} />,
    });
  }
  if (lottery.require_email_verified) {
    rows.push({ label: t('邮箱条件'), value: t('需要绑定邮箱'), icon: <Mail size={16} /> });
  }
  if (!hasRechargeRule && rows.length === 0) return null;
  return (
    <div className='rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-4 text-sm'>
      <div className='mb-3 flex items-center gap-2 font-medium'>
        <Gift size={16} />
        {t('本活动参与条件')}
      </div>
      {hasRechargeRule && (
        <div className='mb-3 rounded border border-[var(--semi-color-border)] bg-white p-3'>
          <div className='mb-3 flex flex-wrap items-start justify-between gap-2'>
            <div className='flex min-w-0 items-center gap-2'>
              <WalletCards size={16} className='shrink-0 text-[var(--semi-color-primary)]' />
              <div className='min-w-0'>
                <Text strong>{t('充值条件')}</Text>
                <div>
                  <Text type='secondary' size='small'>
                    {lottery.recharge_window_days > 0
                      ? t('需要在最近 {{days}} 天内完成符合条件的充值', {
                        days: lottery.recharge_window_days,
                      })
                      : t('允许任意时间内的符合条件充值')}
                  </Text>
                </div>
              </div>
            </div>
            {lottery.count_redemption_as_recharge && (
              <Tag>{t('兑换码兑换计入')}</Tag>
            )}
          </div>
          <div className='grid gap-2 md:grid-cols-3'>
            <ConditionInfoItem
              icon={<Gift size={16} />}
              label={t('需要金额')}
              value={
                lottery.min_recharge_amount > 0
                  ? formatLotteryAmount(lottery.min_recharge_amount)
                  : t('任意成功充值')
              }
              strong
            />
            <ConditionInfoItem
              icon={<Clock3 size={16} />}
              label={t('充值有效期')}
              value={
                lottery.recharge_window_days > 0
                  ? t('{{count}} 天', { count: lottery.recharge_window_days })
                  : t('不限制时间')
              }
            />
            <ConditionInfoItem
              icon={<CalendarClock size={16} />}
              label={t('适用范围')}
              value={t('仅本活动')}
            />
          </div>
        </div>
      )}
      {rows.length > 0 && (
        <div className='grid gap-2 md:grid-cols-3'>
        {rows.map((row) => (
          <ConditionInfoItem
            key={row.label}
            icon={row.icon}
            label={row.label}
            value={row.value}
          />
        ))}
        </div>
      )}
    </div>
  );
}

function hasLotteryConditions(lottery) {
  return Boolean(
    lottery.require_recharge ||
      lottery.min_recharge_amount > 0 ||
      lottery.recharge_window_days > 0 ||
      lottery.count_redemption_as_recharge ||
      lottery.min_account_age_days > 0 ||
      lottery.min_request_count > 0 ||
      lottery.require_email_verified,
  );
}

function LotteryCard({ lottery, onJoin, joining, t }) {
  const round = lottery.round;
  const drawn = isRoundDrawn(round?.status);
  const canJoin =
    round?.status === 'open' &&
    !lottery.joined &&
    !drawn &&
    lottery.eligibility?.eligible !== false &&
    Date.now() / 1000 < round.registration_end;
  let actionLabel = t('参与抽奖');
  if (drawn) {
    actionLabel = t('已开奖');
  } else if (lottery.joined) {
    actionLabel = t('已参与');
  }
  return (
    <Card
      shadows='hover'
      bodyStyle={{ padding: 20 }}
      style={{ height: '100%' }}
    >
      <div className='flex flex-col gap-4'>
        <div className='flex items-start justify-between gap-3'>
          <div>
            <Space wrap>
              {modeTag(lottery.mode, t)}
              {roundStatusTag(round?.status, t)}
              {lottery.won && (
                <Tag color='amber' prefixIcon={<Award size={14} />}>
                  {t('你中奖了')}
                </Tag>
              )}
            </Space>
            <Title heading={5} style={{ margin: '10px 0 4px' }}>
              {lottery.title}
            </Title>
            <Text type='secondary'>{lottery.description || t('暂无描述')}</Text>
          </div>
          <Trophy
            size={28}
            strokeWidth={2.4}
            fill='none'
            color={
              lottery.won
                ? 'var(--semi-color-warning)'
                : 'var(--semi-color-text-1)'
            }
          />
        </div>

        <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
          <div>
            <Text type='secondary' size='small'>
              {t('奖品')}
            </Text>
            <Tooltip content={lottery.prize_name}>
              <div className='max-w-full truncate font-medium'>
                {lottery.prize_name}
              </div>
            </Tooltip>
          </div>
          <div>
            <Text type='secondary' size='small'>
              {t('中奖人数')}
            </Text>
            <div className='font-medium'>{lottery.winner_count}</div>
          </div>
          <div>
            <Text type='secondary' size='small'>
              {t('参与人数')}
            </Text>
            <div className='font-medium'>{lottery.participant_count || 0}</div>
          </div>
        </div>

        <DrawConditions lottery={lottery} t={t} />

        {round && (
          <div className='rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3'>
            <div className='grid grid-cols-1 gap-2 md:grid-cols-3'>
              <Text size='small'>
                {t('报名开始')}：{formatLotteryTime(round.registration_start)}
              </Text>
              <Text size='small'>
                {t('报名结束')}：{formatLotteryTime(round.registration_end)}
              </Text>
              <Text size='small'>
                {t('开奖时间')}：{formatLotteryTime(round.draw_time)}
              </Text>
            </div>
          </div>
        )}

        {lottery.winners?.length > 0 && (
          <div className='rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3'>
            <Text strong>{t('中奖用户')}</Text>
            <div className='mt-2 flex flex-wrap gap-2'>
              {lottery.winners.map((winner, index) => (
                <Tag key={`${winner.masked_name}-${index}`} color='amber'>
                  {winner.masked_name}
                </Tag>
              ))}
            </div>
          </div>
        )}

        {drawn && (!lottery.winners || lottery.winners.length === 0) && (
          <Text type='secondary'>{t('本次抽奖已开奖，暂无中奖用户。')}</Text>
        )}

        {lottery.eligibility && hasLotteryConditions(lottery) && !drawn && (
          <EligibilityPanel eligibility={lottery.eligibility} t={t} compact />
        )}

        <div className='flex items-center justify-between'>
          {lottery.joined ? (
            <Text type='tertiary' size='small'>
              {t('你已参与本轮抽奖')}
            </Text>
          ) : (
            <span />
          )}
          <Button
            type='primary'
            disabled={!canJoin || lottery.joined}
            loading={joining === lottery.id}
            onClick={() => onJoin(lottery.id)}
          >
            {actionLabel}
          </Button>
        </div>
      </div>
    </Card>
  );
}

export default function Lottery() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [settings, setSettings] = useState(null);
  const [lotteries, setLotteries] = useState([]);
  const [publicDrawStatus, setPublicDrawStatus] = useState('all');
  const [myPrizes, setMyPrizes] = useState([]);
  const [adminLotteries, setAdminLotteries] = useState([]);
  const [adminTotal, setAdminTotal] = useState(0);
  const [adminPage, setAdminPage] = useState(1);
  const [adminPageSize, setAdminPageSize] = useState(10);
  const [adminKeyword, setAdminKeyword] = useState('');
  const [adminMode, setAdminMode] = useState('');
  const [adminStatus, setAdminStatus] = useState('');
  const [adminDrawStatus, setAdminDrawStatus] = useState('all');
  const [resultsVisible, setResultsVisible] = useState(false);
  const [resultsLottery, setResultsLottery] = useState(null);
  const [resultsLoading, setResultsLoading] = useState(false);
  const [resultsRounds, setResultsRounds] = useState([]);
  const [resultsTotal, setResultsTotal] = useState(0);
  const [resultsPage, setResultsPage] = useState(1);
  const [resultsPageSize, setResultsPageSize] = useState(5);
  const [resultsKeyword, setResultsKeyword] = useState('');
  const [resultsStatus, setResultsStatus] = useState('all');
  const [joining, setJoining] = useState(0);
  const [createVisible, setCreateVisible] = useState(false);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState(defaultForm);
  const [editingLottery, setEditingLottery] = useState(null);
  const [detailOnly, setDetailOnly] = useState(false);
  const userIsAdmin = useMemo(() => isAdmin(), []);

  const loadPublicData = async () => {
    setLoading(true);
    try {
      const settingsRes = await API.get('/api/lottery/settings');
      if (settingsRes.data.success) {
        setSettings(settingsRes.data.data);
      }
      if (!settingsRes.data.data?.enabled) {
        setLotteries([]);
        setMyPrizes([]);
        return;
      }
      const [lotteriesRes, prizesRes] = await Promise.all([
        API.get('/api/lottery/', {
          params: { draw_status: publicDrawStatus },
        }),
        API.get('/api/lottery/my-prizes?p=1&page_size=20'),
      ]);
      if (lotteriesRes.data.success) {
        setLotteries(lotteriesRes.data.data || []);
      }
      if (prizesRes.data.success) {
        setMyPrizes(prizesRes.data.data?.items || []);
      }
    } catch (error) {
      Toast.error({ content: t('加载抽奖信息失败') });
    } finally {
      setLoading(false);
    }
  };

  const loadAdminData = async () => {
    if (!userIsAdmin) return;
    try {
      const params = new URLSearchParams({
        p: String(adminPage),
        page_size: String(adminPageSize),
      });
      if (adminKeyword) params.append('keyword', adminKeyword);
      if (adminMode) params.append('mode', adminMode);
      if (adminStatus) params.append('status', adminStatus);
      if (adminDrawStatus) params.append('draw_status', adminDrawStatus);
      const res = await API.get(`/api/lottery/admin/?${params.toString()}`);
      if (res.data.success) {
        setAdminLotteries(res.data.data?.items || []);
        setAdminTotal(res.data.data?.total || 0);
      } else {
        Toast.error({ content: res.data.message || t('加载失败') });
      }
    } catch (error) {
      Toast.error({ content: t('加载抽奖管理失败') });
    }
  };

  useEffect(() => {
    loadPublicData();
  }, [publicDrawStatus]);

  useEffect(() => {
    loadAdminData();
  }, [adminPage, adminPageSize, adminKeyword, adminMode, adminStatus, adminDrawStatus]);

  useEffect(() => {
    if (!resultsVisible || !resultsLottery) return;
    const loadResults = async () => {
      setResultsLoading(true);
      try {
        const params = new URLSearchParams({
          p: String(resultsPage),
          page_size: String(resultsPageSize),
        });
        if (resultsKeyword) params.append('keyword', resultsKeyword);
        if (resultsStatus) params.append('status', resultsStatus);
        const res = await API.get(
          `/api/lottery/admin/${resultsLottery.id}/rounds?${params.toString()}`
        );
        if (res.data.success) {
          setResultsRounds(res.data.data?.items || []);
          setResultsTotal(res.data.data?.total || 0);
        } else {
          Toast.error({ content: res.data.message || t('加载失败') });
        }
      } catch (error) {
        Toast.error({ content: t('加载失败') });
      } finally {
        setResultsLoading(false);
      }
    };
    loadResults();
  }, [resultsVisible, resultsLottery, resultsPage, resultsPageSize, resultsKeyword, resultsStatus, t]);

  useEffect(() => {
    setResultsPage(1);
  }, [resultsKeyword, resultsStatus, resultsPageSize, resultsLottery?.id]);

  const handleJoin = async (id) => {
    setJoining(id);
    try {
      const res = await API.post(`/api/lottery/${id}/join`);
      if (res.data.success) {
        Toast.success({ content: t('参与成功') });
        await loadPublicData();
      } else {
        Toast.error({ content: res.data.message || t('参与失败') });
      }
    } catch (error) {
      Toast.error({ content: t('参与失败') });
    } finally {
      setJoining(0);
    }
  };

  const handleStatusChange = async (lottery, status) => {
    try {
      const res = await API.patch(`/api/lottery/admin/${lottery.id}/status`, {
        status,
      });
      if (res.data.success) {
        Toast.success({ content: t('保存成功') });
        await Promise.all([loadAdminData(), loadPublicData()]);
      } else {
        Toast.error({ content: res.data.message || t('保存失败') });
      }
    } catch (error) {
      Toast.error({ content: t('保存失败') });
    }
  };

  const handleDrawRound = async (roundId) => {
    try {
      const res = await API.post(`/api/lottery/admin/rounds/${roundId}/draw`);
      if (res.data.success) {
        Toast.success({ content: t('开奖完成') });
        await Promise.all([loadAdminData(), loadPublicData()]);
      } else {
        Toast.error({ content: res.data.message || t('开奖失败') });
      }
    } catch (error) {
      Toast.error({ content: t('开奖失败') });
    }
  };

  const confirmDrawRound = (roundId) => {
    Modal.confirm({
      title: t('确认手动开奖'),
      content: t('这会立即对当前轮次执行开奖，操作后不能撤销。'),
      okText: t('手动开奖'),
      cancelText: t('取消'),
      onOk: async () => {
        await handleDrawRound(roundId);
      },
    });
  };

  const handleDeleteLottery = (lottery) => {
    Modal.confirm({
      title: t('删除抽奖'),
      content: t('删除后用户不可见且不能再参与，抽奖管理中会保留已删除状态。'),
      okText: t('删除'),
      cancelText: t('取消'),
      okType: 'danger',
      onOk: async () => {
        try {
          const res = await API.delete(`/api/lottery/admin/${lottery.id}`);
          if (res.data.success) {
            Toast.success({ content: t('删除成功') });
            await Promise.all([loadAdminData(), loadPublicData()]);
          } else {
            Toast.error({ content: res.data.message || t('删除失败') });
          }
        } catch (error) {
          Toast.error({ content: t('删除失败') });
        }
      },
    });
  };

  const openResultsModal = (lottery) => {
    setResultsLottery(lottery);
    setResultsVisible(true);
    setResultsPage(1);
    setResultsPageSize(5);
    setResultsKeyword('');
    setResultsStatus('all');
  };

  const openCreateModal = () => {
    setEditingLottery(null);
    setDetailOnly(false);
    setForm({ ...defaultForm });
    setCreateVisible(true);
  };

  const openEditModal = (lottery, readOnly = false) => {
    setEditingLottery(lottery);
    setDetailOnly(readOnly);
    setForm({
      ...defaultForm,
      title: lottery.title,
      description: lottery.description || '',
      prize_name: lottery.prize_name,
      mode: lottery.mode,
      winner_count: lottery.winner_count,
      prize_per_winner: lottery.prize_per_winner,
      require_recharge: lottery.require_recharge,
      min_recharge_amount: lottery.min_recharge_amount,
      recharge_window_days: lottery.recharge_window_days,
      count_redemption_as_recharge: lottery.count_redemption_as_recharge,
      min_account_age_days: lottery.min_account_age_days,
      min_request_count: lottery.min_request_count,
      require_email_verified: lottery.require_email_verified,
      registration_start: dateFromUnix(lottery.round?.registration_start),
      registration_end: dateFromUnix(lottery.round?.registration_end),
      draw_time: dateFromUnix(lottery.round?.draw_time),
      schedule_weekdays: lottery.schedule_weekdays || [1, 3, 5],
      schedule_start_time: lottery.schedule_start_time || '09:00',
      schedule_end_time: lottery.schedule_end_time || '18:00',
      schedule_draw_time: lottery.schedule_draw_time || '20:00',
      prize_codes: (lottery.prize_codes || []).join('\n'),
    });
    setCreateVisible(true);
  };

  const handleFormValuesChange = (values) => {
    setForm((current) => {
      const next = { ...current, ...values };
      for (const field of ['registration_start', 'registration_end', 'draw_time']) {
        if (!Object.prototype.hasOwnProperty.call(values, field)) continue;
        next[field] = current[field];
      }
      return next;
    });
  };

  const handleSave = async () => {
    const codes = parsePrizeCodes(form.prize_codes);
    if (form.mode === 'once') {
      const registrationStart = toUnixSeconds(form.registration_start);
      const registrationEnd = toUnixSeconds(form.registration_end);
      const drawTime = toUnixSeconds(form.draw_time);
      const now = Math.floor(Date.now() / 1000);
      if (!registrationStart) {
        Toast.error({ content: t('请填写报名开始时间') });
        return;
      }
      if (registrationEnd <= registrationStart || registrationEnd <= now) {
        Toast.error({ content: t('报名结束时间必须晚于当前时间和报名开始时间') });
        return;
      }
      if (drawTime < registrationEnd) {
        Toast.error({ content: t('开奖时间必须晚于或等于报名结束时间') });
        return;
      }
    }
    const payload = {
      title: String(form.title || '').trim(),
      description: String(form.description || '').trim(),
      prize_name: String(form.prize_name || '').trim(),
      mode: form.mode,
      winner_count: Number(form.winner_count || 1),
      prize_per_winner: Number(form.prize_per_winner || 1),
      require_recharge: Boolean(form.require_recharge),
      min_recharge_amount: Number(form.min_recharge_amount || 0),
      recharge_window_days: Number(form.recharge_window_days || 0),
      count_redemption_as_recharge: Boolean(form.count_redemption_as_recharge),
      min_account_age_days: Number(form.min_account_age_days || 0),
      min_request_count: Number(form.min_request_count || 0),
      require_email_verified: Boolean(form.require_email_verified),
      registration_start:
        form.mode === 'once' ? toUnixSeconds(form.registration_start) : 0,
      registration_end:
        form.mode === 'once' ? toUnixSeconds(form.registration_end) : 0,
      draw_time: form.mode === 'once' ? toUnixSeconds(form.draw_time) : 0,
      schedule_weekdays:
        form.mode === 'scheduled' ? form.schedule_weekdays || [] : [],
      schedule_start_time:
        form.mode === 'scheduled' ? form.schedule_start_time : '',
      schedule_end_time:
        form.mode === 'scheduled' ? form.schedule_end_time : '',
      schedule_draw_time:
        form.mode === 'scheduled' ? form.schedule_draw_time : '',
      prize_codes: codes,
    };
    if (!payload.title || !payload.prize_name || codes.length === 0) {
      Toast.error({ content: t('请完整填写抽奖信息') });
      return;
    }
    setCreating(true);
    try {
      const res = editingLottery
        ? await API.put(`/api/lottery/admin/${editingLottery.id}`, payload)
        : await API.post('/api/lottery/admin/', payload);
      if (res.data.success) {
        Toast.success({ content: editingLottery ? t('保存成功') : t('创建成功') });
        setCreateVisible(false);
        setEditingLottery(null);
        setForm({ ...defaultForm });
        await Promise.all([loadAdminData(), loadPublicData()]);
      } else {
        Toast.error({ content: res.data.message || t('保存失败') });
      }
    } catch (error) {
      Toast.error({ content: t('保存失败') });
    } finally {
      setCreating(false);
    }
  };

  const adminColumns = [
    {
      title: t('活动'),
      dataIndex: 'title',
      render: (_, record) => (
        <div className='min-w-0'>
          <div className='font-medium'>{record.title}</div>
          <Tooltip content={record.prize_name}>
            <Text
              type='secondary'
              size='small'
              ellipsis={{ showTooltip: false }}
              style={{ maxWidth: 220 }}
            >
              {record.prize_name}
            </Text>
          </Tooltip>
        </div>
      ),
    },
    {
      title: t('模式'),
      dataIndex: 'mode',
      render: (mode) => modeTag(mode, t),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (status) => activityStatusTag(status, t),
    },
    {
      title: t('当前轮次'),
      dataIndex: 'round',
      render: (round) =>
        round ? (
          <Space vertical align='start'>
            {roundStatusTag(round.status, t)}
            <Text size='small'>
              {t('开奖时间')}：{formatLotteryTime(round.draw_time)}
            </Text>
          </Space>
        ) : (
          '-'
        ),
    },
    {
      title: t('参与人数'),
      dataIndex: 'participant_count',
      render: (count) => count || 0,
    },
    {
      title: t('已分配奖品'),
      dataIndex: 'assigned_prize_count',
      render: (count) => count ?? 0,
    },
    {
      title: t('未分配奖品'),
      dataIndex: 'available_prize_count',
      render: (count) => count ?? 0,
    },
    {
      title: t('操作'),
      key: 'action',
      render: (_, record) => (
        <Space wrap>
          <Button
            size='small'
            theme='outline'
            disabled={record.status === 3}
            onClick={() =>
              handleStatusChange(record, record.status === 1 ? 2 : 1)
            }
          >
            {record.status === 1 ? t('停用') : t('启用')}
          </Button>
          {record.round &&
            ['pending', 'open'].includes(record.round.status) && (
                <Button
                  size='small'
                  type='primary'
                  theme='outline'
                  disabled={record.status === 3}
                  onClick={() => confirmDrawRound(record.round.id)}
                >
                  {t('手动开奖')}
                </Button>
            )}
          <Button
            size='small'
            theme='outline'
            icon={<Trophy size={14} />}
            onClick={() => openResultsModal(record)}
          >
            {t('开奖结果')}
          </Button>
          <Button
            size='small'
            theme='outline'
            icon={
              record.status === 3 || !record.can_edit || !isRoundUndrawn(record.round?.status)
                ? <Eye size={14} />
                : <Edit3 size={14} />
            }
            onClick={() =>
              openEditModal(
                record,
                record.status === 3 || !record.can_edit || !isRoundUndrawn(record.round?.status),
              )
            }
          >
            {record.status === 3 || !record.can_edit || !isRoundUndrawn(record.round?.status)
              ? t('详情')
              : t('编辑')}
          </Button>
          <Button
            size='small'
            theme='outline'
            type='danger'
            disabled={record.status === 3}
            icon={<Trash2 size={14} />}
            onClick={() => handleDeleteLottery(record)}
          >
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  const renderResultsExpandedRow = (record) => {
    const round = record.round || {};
    const winners = record.winners || [];
    const drawn = isRoundDrawn(round.status);
    return (
      <div className='rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-4'>
        <div className='mb-3 grid grid-cols-1 gap-3 md:grid-cols-3'>
          <div>
            <Text type='secondary' size='small'>
              {t('报名开始')}
            </Text>
            <div>{formatLotteryTime(round.registration_start)}</div>
          </div>
          <div>
            <Text type='secondary' size='small'>
              {t('报名结束')}
            </Text>
            <div>{formatLotteryTime(round.registration_end)}</div>
          </div>
          <div>
            <Text type='secondary' size='small'>
              {t('内部轮次标识')}
            </Text>
            <div className='font-mono'>{round.round_key || '-'}</div>
          </div>
        </div>
        {drawn && winners.length > 0 ? (
          <div className='flex flex-col gap-3'>
            {winners.map((winner, winnerIndex) => {
              const prizes =
                winner.prize_details && winner.prize_details.length > 0
                  ? winner.prize_details
                  : (winner.prizes || []).map((code, prizeIndex) => ({
                      id: prizeIndex,
                      prize_name: '',
                      code,
                    }));
              return (
                <div
                  key={winner.user_id || `${winner.masked_name}-${winnerIndex}`}
                  className='rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3'
                >
                  <div className='mb-2 flex flex-col gap-1 md:flex-row md:items-start md:justify-between'>
                    <div>
                      <Text strong>{winner.username || winner.masked_name}</Text>
                      <div className='text-xs text-[var(--semi-color-text-2)]'>
                        {t('用户ID')}：{winner.user_id || '-'} · {t('中奖时间')}：
                        {formatLotteryTime(winner.won_at)}
                      </div>
                    </div>
                    <Tag color='blue'>{t('{{count}} 个奖品', { count: prizes.length })}</Tag>
                  </div>
                  <div className='grid grid-cols-1 gap-2 md:grid-cols-2'>
                    {prizes.map((prize) => (
                      <div
                        key={`${prize.id}-${prize.code}`}
                        className='min-w-0 rounded bg-[var(--semi-color-fill-1)] px-3 py-2'
                      >
                        <Tooltip content={prize.prize_name || t('兑换码')}>
                          <div className='truncate text-xs text-[var(--semi-color-text-2)]'>
                            {prize.prize_name || t('兑换码')}
                          </div>
                        </Tooltip>
                        <Text copyable ellipsis={{ showTooltip: true }}>
                          {prize.code}
                        </Text>
                      </div>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <Text type='secondary'>
            {drawn ? t('本轮已开奖，暂无中奖用户。') : t('本轮尚未开奖。')}
          </Text>
        )}
      </div>
    );
  };

  const resultsColumns = [
    {
      title: t('状态'),
      dataIndex: 'round',
      render: (round) => roundStatusTag(round?.status, t),
    },
    {
      title: t('轮次'),
      dataIndex: 'round',
      render: (round) => formatLotteryRoundLabel(resultsLottery?.mode, round, t),
    },
    {
      title: t('开奖时间'),
      dataIndex: 'round',
      render: (round) => formatLotteryTime(round?.draw_time),
    },
    {
      title: t('参与人数'),
      dataIndex: 'participant_count',
      render: (count) => count || 0,
    },
    {
      title: t('中奖人数'),
      dataIndex: 'winners',
      render: (winners) => (winners?.length || 0),
    },
  ];

  const prizeColumns = [
    { title: t('活动'), dataIndex: 'title' },
    {
      title: t('奖品'),
      dataIndex: 'prize_name',
      render: (name) => (
        <Tooltip content={name}>
          <Text ellipsis={{ showTooltip: false }} style={{ maxWidth: 220 }}>
            {name}
          </Text>
        </Tooltip>
      ),
    },
    { title: t('兑换码'), dataIndex: 'code', render: (code) => <Text copyable>{code}</Text> },
    {
      title: t('中奖时间'),
      dataIndex: 'won_at',
      render: (time) => formatLotteryTime(time),
    },
  ];

  return (
    <div className='mt-[60px] px-2'>
      <Spin spinning={loading}>
        <div className='mb-4 flex flex-col gap-2 md:flex-row md:items-center md:justify-between'>
          <div>
            <Title heading={3} style={{ margin: 0 }}>
              {t('抽奖活动')}
            </Title>
            <Text type='secondary'>
              {settings?.enabled
                ? t('参与活动并查看自己的中奖兑换码')
                : t('抽奖功能当前未开启')}
            </Text>
          </div>
          <Space>
            <Tag color={settings?.enabled ? 'green' : 'grey'}>
              {settings?.enabled ? t('已开启') : t('未开启')}
            </Tag>
          </Space>
        </div>

        {settings && !settings.enabled ? (
          <Card>
            <Empty description={t('抽奖功能当前未开启')} />
          </Card>
        ) : (
        <Tabs type='card'>
          <TabPane
            itemKey='activities'
            tab={
              <span className='inline-flex items-center gap-1'>
                <Trophy size={16} />
                {t('抽奖活动')}
              </span>
            }
          >
            <div className='mb-3 flex flex-col gap-2 md:flex-row md:items-center md:justify-between'>
              <Select
                value={publicDrawStatus}
                onChange={(value) => setPublicDrawStatus(value || 'all')}
                style={{ width: 160 }}
              >
                <Select.Option value='all'>{t('全部抽奖')}</Select.Option>
                <Select.Option value='undrawn'>{t('未开奖')}</Select.Option>
                <Select.Option value='drawn'>{t('已开奖')}</Select.Option>
              </Select>
            </div>
            {lotteries.length === 0 ? (
              <Empty description={t('暂无可参与的抽奖活动')} />
            ) : (
              <div className='grid grid-cols-1 gap-4 lg:grid-cols-2 2xl:grid-cols-3'>
                {lotteries.map((lottery) => (
                  <LotteryCard
                    key={lottery.id}
                    lottery={lottery}
                    joining={joining}
                    onJoin={handleJoin}
                    t={t}
                  />
                ))}
              </div>
            )}
          </TabPane>
          <TabPane
            itemKey='prizes'
            tab={
              <span className='inline-flex items-center gap-1'>
                <Gift size={16} />
                {t('我的奖品')}
              </span>
            }
          >
            <Table
              columns={prizeColumns}
              dataSource={myPrizes}
              rowKey='id'
              pagination={false}
              empty={<Empty description={t('暂无中奖记录')} />}
            />
          </TabPane>
          {userIsAdmin && (
            <TabPane
              itemKey='admin'
              tab={
                <span className='inline-flex items-center gap-1'>
                  <CalendarClock size={16} />
                  {t('抽奖管理')}
                </span>
              }
            >
              <div className='mb-3 flex flex-col gap-2 md:flex-row'>
                <Input
                  value={adminKeyword}
                  onChange={(value) => {
                    setAdminKeyword(value);
                    setAdminPage(1);
                  }}
                  showClear
                  placeholder={t('搜索活动或奖品')}
                  style={{ flex: 1 }}
                />
                <Select
                  value={adminMode}
                  onChange={(value) => {
                    setAdminMode(value || '');
                    setAdminPage(1);
                  }}
                  placeholder={t('模式')}
                  style={{ width: 140 }}
                >
                  <Select.Option value=''>{t('全部模式')}</Select.Option>
                  <Select.Option value='once'>{t('一次性抽奖')}</Select.Option>
                  <Select.Option value='scheduled'>
                    {t('定时抽奖')}
                  </Select.Option>
                </Select>
                <Select
                  value={adminStatus}
                  onChange={(value) => {
                    setAdminStatus(value || '');
                    setAdminPage(1);
                  }}
                  placeholder={t('状态')}
                  style={{ width: 140 }}
                >
                  <Select.Option value=''>{t('全部状态')}</Select.Option>
                  <Select.Option value='1'>{t('启用')}</Select.Option>
                  <Select.Option value='2'>{t('停用')}</Select.Option>
                  <Select.Option value='3'>{t('已删除')}</Select.Option>
                </Select>
                <Select
                  value={adminDrawStatus}
                  onChange={(value) => {
                    setAdminDrawStatus(value || 'all');
                    setAdminPage(1);
                  }}
                  placeholder={t('开奖状态')}
                  style={{ width: 140 }}
                >
                  <Select.Option value='all'>{t('全部开奖')}</Select.Option>
                  <Select.Option value='undrawn'>{t('未开奖')}</Select.Option>
                  <Select.Option value='drawn'>{t('已开奖')}</Select.Option>
                </Select>
                <Button type='primary' onClick={openCreateModal}>
                  {t('新建抽奖')}
                </Button>
              </div>
              <Table
                columns={adminColumns}
                dataSource={adminLotteries}
                rowKey='id'
                pagination={{
                  currentPage: adminPage,
                  pageSize: adminPageSize,
                  total: adminTotal,
                  showSizeChanger: true,
                  onPageChange: setAdminPage,
                  onPageSizeChange: (size) => {
                    setAdminPageSize(size);
                    setAdminPage(1);
                  },
                }}
              />
            </TabPane>
          )}
        </Tabs>
        )}
      </Spin>

      <Modal
        title={t('开奖结果')}
        visible={resultsVisible}
        onCancel={() => {
          setResultsVisible(false);
          setResultsLottery(null);
        }}
        footer={null}
        width={960}
      >
        {resultsLottery && (
          <div className='mb-4 rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-4'>
            <div className='mb-3 flex flex-col gap-1 md:flex-row md:items-start md:justify-between'>
              <div className='min-w-0'>
                <Text strong>{resultsLottery.title}</Text>
                <Tooltip content={resultsLottery.prize_name}>
                  <div className='max-w-[360px] truncate text-[var(--semi-color-text-2)]'>
                    {resultsLottery.prize_name}
                  </div>
                </Tooltip>
              </div>
              <Space>
                <Tag color='green'>
                  {t('已分配奖品')}：{resultsLottery.assigned_prize_count || 0}
                </Tag>
                <Tag color='grey'>
                  {t('未分配奖品')}：{resultsLottery.available_prize_count || 0}
                </Tag>
              </Space>
            </div>
            <div className='flex flex-col gap-2 md:flex-row md:items-center'>
              <Input
                value={resultsKeyword}
                onChange={setResultsKeyword}
                placeholder={t('搜索轮次、中奖用户或兑换码')}
                style={{ maxWidth: 280 }}
              />
              <Select
                value={resultsStatus}
                onChange={(value) => setResultsStatus(value || 'all')}
                style={{ width: 140 }}
              >
                <Select.Option value='all'>{t('全部轮次')}</Select.Option>
                <Select.Option value='drawn'>{t('已开奖')}</Select.Option>
                <Select.Option value='undrawn'>{t('未开奖')}</Select.Option>
              </Select>
            </div>
          </div>
        )}
        <Table
          columns={resultsColumns}
          dataSource={resultsRounds}
          rowKey={(record) => record.round?.id}
          expandedRowRender={renderResultsExpandedRow}
          loading={resultsLoading}
          empty={<Empty description={t('暂无开奖结果')} />}
          pagination={{
            currentPage: resultsPage,
            pageSize: resultsPageSize,
            total: resultsTotal,
            showSizeChanger: true,
            onPageChange: setResultsPage,
            onPageSizeChange: (size) => {
              setResultsPageSize(size);
              setResultsPage(1);
            },
          }}
        />
      </Modal>

      <Modal
        title={
          detailOnly
            ? t('详情')
            : editingLottery
              ? t('编辑抽奖')
              : t('新建抽奖')
        }
        visible={createVisible}
        onCancel={() => {
          setCreateVisible(false);
          setEditingLottery(null);
          setDetailOnly(false);
        }}
        onOk={detailOnly ? undefined : handleSave}
        confirmLoading={detailOnly ? false : creating}
        okText={detailOnly ? t('关闭') : editingLottery ? t('保存') : t('创建')}
        cancelText={t('取消')}
        width={860}
        okButtonProps={detailOnly ? { style: { display: 'none' } } : undefined}
      >
        <Form
          key={editingLottery?.id || 'create'}
          labelPosition='top'
          onValueChange={handleFormValuesChange}
          initValues={form}
        >
          <Form.Input field='title' label={t('活动标题')} disabled={detailOnly} />
          <Form.Input field='prize_name' label={t('奖品名称')} disabled={detailOnly} />
          <Form.TextArea field='description' label={t('活动说明')} rows={3} disabled={detailOnly} />
          <Form.Select field='mode' label={t('抽奖模式')} disabled={detailOnly}>
            <Form.Select.Option value='once'>{t('一次性抽奖')}</Form.Select.Option>
            <Form.Select.Option value='scheduled'>{t('定时抽奖')}</Form.Select.Option>
          </Form.Select>
          <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
            <Form.InputNumber
              field='winner_count'
              label={t('中奖人数')}
              min={1}
              max={100}
              disabled={detailOnly}
            />
            <Form.InputNumber
              field='prize_per_winner'
              label={t('每人奖品数')}
              min={1}
              max={100}
              disabled={detailOnly}
            />
          </div>
          <div className='rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3'>
            <div className='mb-3 font-medium'>{t('参与条件')}</div>
            <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
              <Form.Switch
                field='require_recharge'
                label={t('参与前必须完成充值')}
                checkedText='｜'
                uncheckedText='〇'
                disabled={detailOnly}
              />
              <Form.Switch
                field='require_email_verified'
                label={t('参与前必须绑定邮箱')}
                checkedText='｜'
                uncheckedText='〇'
                disabled={detailOnly}
              />
              <Form.InputNumber
                field='min_recharge_amount'
                label={t('最低充值金额')}
                min={0}
                step={1}
                disabled={detailOnly}
                extraText={t('0 表示任意成功充值即可满足金额条件')}
              />
              <Form.InputNumber
                field='recharge_window_days'
                label={t('充值有效期天数')}
                min={0}
                step={1}
                disabled={detailOnly}
                extraText={t('0 表示不限制充值或兑换发生时间')}
              />
              <Form.InputNumber
                field='min_account_age_days'
                label={t('账号最小天数')}
                min={0}
                step={1}
                disabled={detailOnly}
              />
              <Form.InputNumber
                field='min_request_count'
                label={t('最小请求次数')}
                min={0}
                step={1}
                disabled={detailOnly}
              />
            </div>
            <Form.Switch
              field='count_redemption_as_recharge'
              label={t('兑换码兑换计入充值条件')}
              checkedText='｜'
              uncheckedText='〇'
              disabled={detailOnly}
            />
          </div>
          {form.mode === 'once' ? (
            <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
              <Form.Slot label={t('报名开始')}>
                <DatePicker
                  type='dateTime'
                  value={form.registration_start}
                  disabled={detailOnly}
                  onChange={(value) =>
                    setForm((current) => ({ ...current, registration_start: value }))
                  }
                  style={{ width: '100%' }}
                />
              </Form.Slot>
              <Form.Slot label={t('报名结束')}>
                <DatePicker
                  type='dateTime'
                  value={form.registration_end}
                  disabled={detailOnly}
                  onChange={(value) =>
                    setForm((current) => ({ ...current, registration_end: value }))
                  }
                  style={{ width: '100%' }}
                />
              </Form.Slot>
              <Form.Slot label={t('开奖时间')}>
                <DatePicker
                  type='dateTime'
                  value={form.draw_time}
                  disabled={detailOnly}
                  onChange={(value) =>
                    setForm((current) => ({ ...current, draw_time: value }))
                  }
                  style={{ width: '100%' }}
                />
              </Form.Slot>
            </div>
          ) : (
            <>
              <Form.Select
                field='schedule_weekdays'
                label={t('开奖星期')}
                multiple
                disabled={detailOnly}
              >
                {WEEKDAYS.map((day) => (
                  <Form.Select.Option key={day} value={day}>
                    {t(['周日', '周一', '周二', '周三', '周四', '周五', '周六'][day])}
                  </Form.Select.Option>
                ))}
              </Form.Select>
              <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
                <Form.Input
                  field='schedule_start_time'
                  label={t('报名开始')}
                  placeholder='09:00'
                  disabled={detailOnly}
                />
                <Form.Input
                  field='schedule_end_time'
                  label={t('报名结束')}
                  placeholder='18:00'
                  disabled={detailOnly}
                />
                <Form.Input
                  field='schedule_draw_time'
                  label={t('开奖时间')}
                  placeholder='20:00'
                  disabled={detailOnly}
                />
              </div>
            </>
          )}
          <Form.TextArea
            field='prize_codes'
            label={t('奖品兑换码')}
            rows={6}
            placeholder={t('每行一个兑换码')}
            disabled={detailOnly}
          />
          <div className='rounded border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3'>
            <Space>
              <Users size={16} />
              <Text type='secondary' size='small'>
                {t('兑换码数量需要大于中奖人数，并满足中奖人数乘以每人奖品数。')}
              </Text>
            </Space>
          </div>
        </Form>
      </Modal>
    </div>
  );
}
