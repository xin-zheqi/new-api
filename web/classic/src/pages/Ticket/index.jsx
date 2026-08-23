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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Empty,
  Input,
  Modal,
  Pagination,
  Select,
  SideSheet,
  Tag,
  TextArea,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Eye, Plus, RefreshCw, RotateCcw, Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import CardPro from '../../components/common/ui/CardPro';
import CardTable from '../../components/common/ui/CardTable';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { API, showError, showSuccess } from '../../helpers';
import { createCardProPagination } from '../../helpers/utils';
import TicketThread from './TicketThread';
import { TicketImagePicker } from './TicketImage';
import {
  TICKET_CONTENT_MAX_LENGTH,
  TICKET_STATUS,
  TICKET_TITLE_MAX_LENGTH,
  formatTicketTime,
  getTicketErrorMessage,
  getTicketStatusMeta,
  getTicketUserLabel,
  ticketTextLength,
  truncateTicketText,
} from './ticket';

const { Text, Title } = Typography;

const DEFAULT_ADMIN_FILTERS = {
  keyword: '',
  status: TICKET_STATUS.WAITING_ADMIN,
  userId: '',
};

const UserTicketCenter = () => {
  const { t } = useTranslation();
  const [tickets, setTickets] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [activeTicketId, setActiveTicketId] = useState(null);
  const [selectedId, setSelectedId] = useState(null);
  const [ticket, setTicket] = useState(null);
  const [listLoading, setListLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [listError, setListError] = useState('');
  const [replying, setReplying] = useState(false);
  const [createVisible, setCreateVisible] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createTitle, setCreateTitle] = useState('');
  const [createContent, setCreateContent] = useState('');
  const [createImage, setCreateImage] = useState(null);
  const listRequestRef = useRef(0);
  const detailRequestRef = useRef(0);
  const selectedIdRef = useRef(null);

  const loadTickets = async (
    nextPage = page,
    nextPageSize = pageSize,
    preferredId = null,
    silent = false,
  ) => {
    const requestId = ++listRequestRef.current;
    if (!silent) {
      setListLoading(true);
      setListError('');
    }
    try {
      const response = await API.get('/api/ticket/self', {
        params: { p: nextPage, page_size: nextPageSize },
        skipErrorHandler: true,
      });
      if (!response.data?.success) {
        const requestError = new Error('Ticket list request failed');
        requestError.code = response.data?.code;
        throw requestError;
      }
      if (requestId !== listRequestRef.current) return null;
      const data = response.data.data || {};
      const items = Array.isArray(data.items) ? data.items : [];
      const nextActiveId = data.active_ticket_id || null;
      setTickets(items);
      setTotal(Number(data.total || 0));
      setPage(Number(data.page || nextPage));
      setPageSize(Number(data.page_size || nextPageSize));
      setActiveTicketId(nextActiveId);
      setListError('');

      const preferredOnPage = items.some((item) => item.id === preferredId)
        ? preferredId
        : null;
      const activeOnPage = items.some((item) => item.id === nextActiveId)
        ? nextActiveId
        : null;
      const nextSelectedId =
        preferredOnPage || activeOnPage || items[0]?.id || null;
      selectedIdRef.current = nextSelectedId;
      setSelectedId(nextSelectedId);
      if (!nextSelectedId) setTicket(null);
      return nextSelectedId;
    } catch (error) {
      if (requestId !== listRequestRef.current) return null;
      const message = getTicketErrorMessage(error, t, t('加载工单列表失败'));
      if (!silent) {
        setListError(message);
        showError(message);
      }
      return null;
    } finally {
      if (!silent && requestId === listRequestRef.current) {
        setListLoading(false);
      }
    }
  };

  const loadDetail = async (id, silent = false) => {
    const requestId = ++detailRequestRef.current;
    if (!id) {
      setTicket(null);
      if (!silent) setDetailLoading(false);
      return;
    }
    if (!silent) setDetailLoading(true);
    try {
      const response = await API.get(`/api/ticket/${id}`, {
        skipErrorHandler: true,
      });
      if (!response.data?.success) {
        const requestError = new Error('Ticket detail request failed');
        requestError.code = response.data?.code;
        throw requestError;
      }
      if (requestId !== detailRequestRef.current) return;
      setTicket(response.data.data || null);
    } catch (error) {
      if (requestId !== detailRequestRef.current) return;
      if (!silent) {
        showError(getTicketErrorMessage(error, t, t('加载工单详情失败')));
        setTicket(null);
      }
    } finally {
      if (!silent && requestId === detailRequestRef.current) {
        setDetailLoading(false);
      }
    }
  };

  useEffect(() => {
    loadTickets(1, pageSize);
  }, []);

  useEffect(() => {
    loadDetail(selectedId);
  }, [selectedId]);

  useEffect(() => {
    selectedIdRef.current = selectedId;
  }, [selectedId]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (listLoading || detailLoading || replying || creating) return;
      const currentTicketId = selectedIdRef.current;
      void loadTickets(page, pageSize, currentTicketId, true);
      if (currentTicketId) void loadDetail(currentTicketId, true);
    }, 30000);
    return () => window.clearInterval(timer);
  }, [page, pageSize, listLoading, detailLoading, replying, creating]);

  const createTicket = async () => {
    const title = createTitle.trim();
    const content = createContent.trim();
    if (!title || !content || creating || activeTicketId) return;
    setCreating(true);
    try {
      const formData = new FormData();
      formData.append('title', title);
      formData.append('content', content);
      if (createImage) formData.append('image', createImage);
      const response = await API.post('/api/ticket', formData, {
        skipErrorHandler: true,
      });
      if (!response.data?.success) {
        const requestError = new Error('Ticket create request failed');
        requestError.code = response.data?.code;
        throw requestError;
      }
      const createdTicket = response.data.data;
      showSuccess(t('工单创建成功'));
      setCreateVisible(false);
      setCreateTitle('');
      setCreateContent('');
      setCreateImage(null);
      await loadTickets(1, pageSize, createdTicket?.id || null);
    } catch (error) {
      showError(getTicketErrorMessage(error, t, t('创建工单失败')));
    } finally {
      setCreating(false);
    }
  };

  const replyTicket = async (content, image) => {
    if (!ticket?.id) return false;
    const targetTicketId = ticket.id;
    setReplying(true);
    try {
      const formData = new FormData();
      formData.append('content', content);
      if (image) formData.append('image', image);
      const response = await API.post(
        `/api/ticket/${targetTicketId}/reply`,
        formData,
        { skipErrorHandler: true },
      );
      if (!response.data?.success) {
        const requestError = new Error('Ticket reply request failed');
        requestError.code = response.data?.code;
        throw requestError;
      }
      if (selectedIdRef.current === targetTicketId) {
        setTicket(response.data.data || null);
      }
      showSuccess(t('回复已发送'));
      await loadTickets(page, pageSize, selectedIdRef.current);
      return true;
    } catch (error) {
      showError(getTicketErrorMessage(error, t, t('回复工单失败')));
      return false;
    } finally {
      setReplying(false);
    }
  };

  const canCreate = !listLoading && !listError && !creating && !activeTicketId;
  const createTooltip = activeTicketId
    ? t('当前工单结束后才能创建新工单')
    : listLoading
      ? t('加载中...')
      : listError
        ? t('加载工单列表失败')
        : t('创建新工单');

  return (
    <div className='ticket-center-page mt-[60px] w-full px-2 pb-4'>
      <div className='mx-auto max-w-7xl space-y-3'>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <Title heading={3} className='m-0'>
              {t('工单中心')}
            </Title>
            <Text type='tertiary'>
              {t('查看历史问题，并与管理员继续沟通。')}
            </Text>
          </div>
          <Tooltip content={createTooltip}>
            <span>
              <Button
                theme='solid'
                type='primary'
                icon={<Plus size={16} />}
                disabled={!canCreate}
                onClick={() => setCreateVisible(true)}
              >
                {t('创建工单')}
              </Button>
            </span>
          </Tooltip>
        </div>

        {!!activeTicketId && (
          <Banner
            type='info'
            description={t(
              '每次只能有一个进行中的工单。当前工单由管理员结束后，您才能创建新工单。',
            )}
            closeIcon={null}
            className='!rounded-md'
          />
        )}
        {listError && (
          <Banner
            type='danger'
            description={listError}
            closeIcon={null}
            className='!rounded-md'
          />
        )}

        <div className='grid min-w-0 gap-3 lg:grid-cols-[320px_minmax(0,1fr)]'>
          <Card className='!rounded-lg' bodyStyle={{ padding: 0 }}>
            <div className='flex items-center justify-between border-b border-[var(--semi-color-border)] px-4 py-3'>
              <Text strong>{t('我的工单')}</Text>
              <Tooltip content={t('刷新列表')}>
                <Button
                  aria-label={t('刷新列表')}
                  icon={<RefreshCw size={15} />}
                  icononly
                  size='small'
                  type='tertiary'
                  theme='borderless'
                  loading={listLoading}
                  onClick={() => loadTickets(page, pageSize, selectedId)}
                />
              </Tooltip>
            </div>
            <div className='min-h-[240px]'>
              {!listLoading && tickets.length === 0 ? (
                <div className='flex min-h-[240px] items-center justify-center p-4'>
                  <Empty description={t('暂无工单记录')} />
                </div>
              ) : (
                tickets.map((item) => {
                  const meta = getTicketStatusMeta(item.status, false, t);
                  const selected = item.id === selectedId;
                  return (
                    <button
                      key={item.id}
                      type='button'
                      className={`block w-full border-b border-[var(--semi-color-border)] px-4 py-3 text-left transition-colors ${
                        selected
                          ? 'bg-[var(--semi-color-primary-light-default)]'
                          : 'hover:bg-[var(--semi-color-fill-0)]'
                      }`}
                      aria-pressed={selected}
                      disabled={replying}
                      onClick={() => {
                        selectedIdRef.current = item.id;
                        setSelectedId(item.id);
                      }}
                    >
                      <div className='flex items-start justify-between gap-2'>
                        <span
                          className='line-clamp-2 min-w-0 font-medium'
                          style={{ overflowWrap: 'anywhere' }}
                        >
                          {item.title}
                        </span>
                        <Tag color={meta.color} size='small' shape='circle'>
                          {meta.label}
                        </Tag>
                      </div>
                      <div className='mt-2 flex items-center justify-between gap-2 text-xs text-[var(--semi-color-text-2)]'>
                        <span>#{item.id}</span>
                        <span>
                          {formatTicketTime(
                            item.last_message_at || item.updated_at,
                          )}
                        </span>
                      </div>
                    </button>
                  );
                })
              )}
            </div>
            {total > 0 && (
              <div className='flex justify-center border-t border-[var(--semi-color-border)] p-3'>
                <Pagination
                  currentPage={page}
                  pageSize={pageSize}
                  total={total}
                  size='small'
                  showSizeChanger
                  pageSizeOpts={[10, 20, 50]}
                  onPageChange={(nextPage) => loadTickets(nextPage, pageSize)}
                  onPageSizeChange={(size) => loadTickets(1, size)}
                />
              </div>
            )}
          </Card>

          <Card className='min-w-0 !rounded-lg'>
            <TicketThread
              ticket={ticket}
              loading={detailLoading}
              replying={replying}
              onReply={replyTicket}
              onRefresh={() => loadDetail(ticket?.id)}
            />
          </Card>
        </div>
      </div>

      <Modal
        title={t('创建工单')}
        visible={createVisible}
        width={640}
        okText={t('提交工单')}
        cancelText={t('取消')}
        confirmLoading={creating}
        okButtonProps={{
          disabled: !createTitle.trim() || !createContent.trim() || !canCreate,
        }}
        onOk={createTicket}
        onCancel={() => {
          if (creating) return;
          setCreateVisible(false);
          setCreateTitle('');
          setCreateContent('');
          setCreateImage(null);
        }}
      >
        <div className='space-y-4'>
          <div>
            <div className='mb-1 flex items-center justify-between gap-3'>
              <label htmlFor='ticket-create-title' className='font-semibold'>
                {t('问题标题')}
              </label>
              <Text type='tertiary' size='small'>
                {ticketTextLength(createTitle)}/{TICKET_TITLE_MAX_LENGTH}
              </Text>
            </div>
            <Input
              id='ticket-create-title'
              value={createTitle}
              maxLength={TICKET_TITLE_MAX_LENGTH * 2}
              disabled={creating}
              placeholder={t('请简要描述问题')}
              onChange={(value) =>
                setCreateTitle(
                  truncateTicketText(value, TICKET_TITLE_MAX_LENGTH),
                )
              }
            />
          </div>
          <div>
            <div className='mb-1 flex items-center justify-between gap-3'>
              <label htmlFor='ticket-create-content' className='font-semibold'>
                {t('问题详情')}
              </label>
              <Text type='tertiary' size='small'>
                {ticketTextLength(createContent)}/{TICKET_CONTENT_MAX_LENGTH}
              </Text>
            </div>
            <TextArea
              id='ticket-create-content'
              value={createContent}
              maxLength={TICKET_CONTENT_MAX_LENGTH * 2}
              autosize={{ minRows: 5, maxRows: 10 }}
              disabled={creating}
              placeholder={t(
                '请描述遇到的问题和复现步骤，不要提交密码、密钥等敏感信息。',
              )}
              onChange={(value) =>
                setCreateContent(
                  truncateTicketText(value, TICKET_CONTENT_MAX_LENGTH),
                )
              }
            />
          </div>
          <TicketImagePicker
            file={createImage}
            onChange={setCreateImage}
            disabled={creating}
          />
        </div>
      </Modal>
    </div>
  );
};

const AdminTicketCenter = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [tickets, setTickets] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(true);
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState(TICKET_STATUS.WAITING_ADMIN);
  const [userId, setUserId] = useState('');
  const [appliedFilters, setAppliedFilters] = useState(DEFAULT_ADMIN_FILTERS);
  const [detail, setDetail] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [replying, setReplying] = useState(false);
  const [closing, setClosing] = useState(false);
  const listRequestRef = useRef(0);
  const detailRequestRef = useRef(0);
  const detailIdRef = useRef(null);

  const statusOptions = useMemo(
    () => [
      { value: TICKET_STATUS.WAITING_ADMIN, label: t('待管理员处理') },
      { value: TICKET_STATUS.WAITING_USER, label: t('等待用户回复') },
      { value: TICKET_STATUS.CLOSED, label: t('已结束') },
    ],
    [t],
  );

  const loadTickets = async (
    nextPage = page,
    nextPageSize = pageSize,
    filters = appliedFilters,
    silent = false,
  ) => {
    const nextKeyword = filters.keyword;
    const nextStatus = filters.status;
    const nextUserId = filters.userId;
    const trimmedUserId = nextUserId.trim();
    if (trimmedUserId && !/^[1-9]\d*$/.test(trimmedUserId)) {
      showError(t('请输入有效的用户 ID'));
      return false;
    }

    const requestId = ++listRequestRef.current;
    if (!silent) setLoading(true);
    try {
      const params = { p: nextPage, page_size: nextPageSize };
      if (nextKeyword.trim()) params.keyword = nextKeyword.trim();
      if (nextStatus) params.status = nextStatus;
      if (trimmedUserId) params.user_id = trimmedUserId;
      const response = await API.get('/api/ticket/admin', {
        params,
        skipErrorHandler: true,
      });
      if (!response.data?.success) {
        const requestError = new Error('Ticket list request failed');
        requestError.code = response.data?.code;
        throw requestError;
      }
      if (requestId !== listRequestRef.current) return false;
      const data = response.data.data || {};
      setTickets(Array.isArray(data.items) ? data.items : []);
      setTotal(Number(data.total || 0));
      setPage(Number(data.page || nextPage));
      setPageSize(Number(data.page_size || nextPageSize));
      return true;
    } catch (error) {
      if (requestId !== listRequestRef.current) return false;
      if (!silent) {
        showError(getTicketErrorMessage(error, t, t('加载工单列表失败')));
      }
      return false;
    } finally {
      if (!silent && requestId === listRequestRef.current) setLoading(false);
    }
  };

  const loadDetail = async (id, silent = false) => {
    const requestId = ++detailRequestRef.current;
    if (!id) {
      if (!silent) setDetailLoading(false);
      return;
    }
    if (!silent) setDetailLoading(true);
    try {
      const response = await API.get(`/api/ticket/admin/${id}`, {
        skipErrorHandler: true,
      });
      if (!response.data?.success) {
        const requestError = new Error('Ticket detail request failed');
        requestError.code = response.data?.code;
        throw requestError;
      }
      if (
        requestId !== detailRequestRef.current ||
        detailIdRef.current !== id
      ) {
        return;
      }
      setDetail(response.data.data || null);
    } catch (error) {
      if (requestId !== detailRequestRef.current) return;
      if (!silent) {
        showError(getTicketErrorMessage(error, t, t('加载工单详情失败')));
        setDetail(null);
      }
    } finally {
      if (!silent && requestId === detailRequestRef.current) {
        setDetailLoading(false);
      }
    }
  };

  useEffect(() => {
    loadTickets(1, pageSize, DEFAULT_ADMIN_FILTERS);
  }, []);

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (loading || detailLoading || replying || closing) return;
      void loadTickets(page, pageSize, appliedFilters, true);
      const currentTicketId = detailIdRef.current;
      if (currentTicketId) void loadDetail(currentTicketId, true);
    }, 30000);
    return () => window.clearInterval(timer);
  }, [
    page,
    pageSize,
    appliedFilters,
    loading,
    detailLoading,
    replying,
    closing,
  ]);

  const applyFilters = async () => {
    const nextFilters = { keyword, status, userId };
    const loaded = await loadTickets(1, pageSize, nextFilters);
    if (loaded) setAppliedFilters(nextFilters);
  };

  const openDetail = (record) => {
    detailIdRef.current = record.id;
    setDetail(record);
    loadDetail(record.id);
  };

  const replyTicket = async (content, image) => {
    if (!detail?.id) return false;
    const targetTicketId = detail.id;
    setReplying(true);
    try {
      const formData = new FormData();
      formData.append('content', content);
      if (image) formData.append('image', image);
      const response = await API.post(
        `/api/ticket/admin/${targetTicketId}/reply`,
        formData,
        { skipErrorHandler: true },
      );
      if (!response.data?.success) {
        const requestError = new Error('Ticket reply request failed');
        requestError.code = response.data?.code;
        throw requestError;
      }
      if (detailIdRef.current === targetTicketId) {
        setDetail(response.data.data || null);
      }
      showSuccess(t('回复已发送'));
      await loadTickets(page, pageSize, appliedFilters);
      return true;
    } catch (error) {
      showError(getTicketErrorMessage(error, t, t('回复工单失败')));
      return false;
    } finally {
      setReplying(false);
    }
  };

  const closeTicket = () => {
    if (
      !detail?.id ||
      detail.status === TICKET_STATUS.CLOSED ||
      replying ||
      closing
    ) {
      return;
    }
    const targetTicketId = detail.id;
    Modal.confirm({
      title: t('确认结束工单'),
      content: t(
        '结束后用户和管理员都不能继续回复，且只有管理员可以执行此操作。是否继续？',
      ),
      okText: t('结束工单'),
      cancelText: t('取消'),
      okType: 'danger',
      centered: true,
      onOk: async () => {
        setClosing(true);
        try {
          const response = await API.post(
            `/api/ticket/admin/${targetTicketId}/close`,
            undefined,
            { skipErrorHandler: true },
          );
          if (!response.data?.success) {
            const requestError = new Error('Ticket close request failed');
            requestError.code = response.data?.code;
            throw requestError;
          }
          if (detailIdRef.current === targetTicketId) {
            setDetail(response.data.data || null);
          }
          showSuccess(t('工单已结束'));
          await loadTickets(page, pageSize, appliedFilters);
        } catch (error) {
          showError(getTicketErrorMessage(error, t, t('结束工单失败')));
          throw error;
        } finally {
          setClosing(false);
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
        render: (value) => `#${value}`,
      },
      {
        title: t('用户'),
        key: 'user',
        width: 190,
        render: (_, record) => (
          <div className='min-w-0'>
            <div className='truncate font-medium'>
              {getTicketUserLabel(record, t)}
            </div>
            <div className='truncate text-xs text-[var(--semi-color-text-2)]'>
              {record.email || `${t('用户 ID')}: ${record.user_id}`}
            </div>
          </div>
        ),
      },
      {
        title: t('问题标题'),
        dataIndex: 'title',
        key: 'title',
        width: 300,
        render: (value) => (
          <div className='line-clamp-2' style={{ overflowWrap: 'anywhere' }}>
            {value}
          </div>
        ),
      },
      {
        title: t('状态'),
        key: 'status',
        width: 130,
        render: (_, record) => {
          const meta = getTicketStatusMeta(record.status, true, t);
          return (
            <Tag color={meta.color} shape='circle' size='small'>
              {meta.label}
            </Tag>
          );
        },
      },
      {
        title: t('消息数'),
        dataIndex: 'message_count',
        key: 'message_count',
        width: 90,
      },
      {
        title: t('最近更新'),
        key: 'last_message_at',
        width: 170,
        render: (_, record) =>
          formatTicketTime(record.last_message_at || record.updated_at),
      },
      {
        title: '',
        key: 'action',
        width: 105,
        fixed: 'right',
        render: (_, record) => (
          <Button
            size='small'
            type='tertiary'
            theme='light'
            icon={<Eye size={15} />}
            onClick={() => openDetail(record)}
          >
            {t('处理')}
          </Button>
        ),
      },
    ],
    [t],
  );

  const searchArea = (
    <div className='flex w-full flex-col gap-2 md:flex-row md:items-center'>
      <Input
        value={keyword}
        prefix={<Search size={15} />}
        showClear
        aria-label={t('搜索工单编号、标题、用户或邮箱')}
        placeholder={t('搜索工单编号、标题、用户或邮箱')}
        style={{ width: isMobile ? '100%' : 280 }}
        onChange={(value) => setKeyword(truncateTicketText(value, 100))}
        onEnterPress={applyFilters}
      />
      <Select
        value={status}
        optionList={statusOptions}
        showClear
        aria-label={t('工单状态')}
        placeholder={t('工单状态')}
        style={{ width: isMobile ? '100%' : 170 }}
        onChange={(nextStatus) => {
          setStatus(nextStatus);
          const nextFilters = { keyword, status: nextStatus || '', userId };
          void loadTickets(1, pageSize, nextFilters).then((loaded) => {
            if (loaded) setAppliedFilters(nextFilters);
          });
        }}
      />
      <Input
        value={userId}
        showClear
        aria-label={t('用户 ID')}
        placeholder={t('用户 ID')}
        style={{ width: isMobile ? '100%' : 150 }}
        onChange={(value) =>
          setUserId(value.replace(/\D/g, '').replace(/^0+/, '').slice(0, 19))
        }
        onEnterPress={applyFilters}
      />
      <div className='flex gap-2'>
        <Button
          type='primary'
          theme='solid'
          icon={<Search size={15} />}
          loading={loading}
          onClick={applyFilters}
        >
          {t('查询')}
        </Button>
        <Tooltip content={t('重置筛选')}>
          <Button
            aria-label={t('重置筛选')}
            icon={<RotateCcw size={15} />}
            icononly
            type='tertiary'
            theme='outline'
            onClick={() => {
              setKeyword('');
              setStatus(TICKET_STATUS.WAITING_ADMIN);
              setUserId('');
              setAppliedFilters(DEFAULT_ADMIN_FILTERS);
              loadTickets(1, pageSize, DEFAULT_ADMIN_FILTERS);
            }}
          />
        </Tooltip>
        <Tooltip content={t('刷新列表')}>
          <Button
            aria-label={t('刷新列表')}
            icon={<RefreshCw size={15} />}
            icononly
            type='tertiary'
            theme='outline'
            onClick={() => loadTickets(page, pageSize, appliedFilters)}
          />
        </Tooltip>
      </div>
    </div>
  );

  return (
    <div className='ticket-admin-page mt-[60px] w-full px-2 pb-4'>
      <CardPro
        type='type1'
        className='!rounded-lg'
        descriptionArea={
          <div className='flex flex-wrap items-start justify-between gap-2'>
            <div>
              <Title heading={4} className='m-0'>
                {t('工单管理')}
              </Title>
              <Text type='secondary'>
                {t('按处理状态、用户和关键词筛选工单，并在同一工作台中回复。')}
              </Text>
            </div>
            <Tag color='blue' shape='circle'>
              {t('共 {{count}} 条', { count: total })}
            </Tag>
          </div>
        }
        searchArea={searchArea}
        paginationArea={createCardProPagination({
          currentPage: page,
          pageSize,
          total,
          onPageChange: (nextPage) =>
            loadTickets(nextPage, pageSize, appliedFilters),
          onPageSizeChange: (size) => loadTickets(1, size, appliedFilters),
          isMobile,
          pageSizeOpts: [10, 20, 50],
          t,
        })}
        t={t}
      >
        <CardTable
          columns={columns}
          dataSource={tickets}
          loading={loading}
          rowKey='id'
          scroll={{ x: 'max-content' }}
          pagination={false}
          hidePagination
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('没有符合条件的工单')}
              style={{ padding: 30 }}
            />
          }
        />
      </CardPro>

      <SideSheet
        visible={!!detail}
        placement='right'
        width={isMobile ? '100%' : 760}
        title={t('工单处理')}
        onCancel={() => {
          if (!replying && !closing) {
            detailRequestRef.current += 1;
            detailIdRef.current = null;
            setDetail(null);
          }
        }}
      >
        <TicketThread
          ticket={detail}
          admin
          loading={detailLoading}
          replying={replying}
          closing={closing}
          onReply={replyTicket}
          onClose={closeTicket}
          onRefresh={() => loadDetail(detail?.id)}
        />
      </SideSheet>
    </div>
  );
};

const TicketCenter = ({ admin = false }) =>
  admin ? <AdminTicketCenter /> : <UserTicketCenter />;

export default TicketCenter;
