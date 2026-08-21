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

import React, { useEffect, useRef, useState } from 'react';
import {
  Avatar,
  Banner,
  Button,
  Divider,
  Empty,
  Spin,
  Tag,
  TextArea,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { CheckCircle2, RefreshCw, Send, ShieldCheck, User } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { TicketAttachment, TicketImagePicker } from './TicketImage';
import {
  TICKET_CONTENT_MAX_LENGTH,
  TICKET_MESSAGE_LIMIT,
  TICKET_STATUS,
  canReplyToTicket,
  formatTicketTime,
  getTicketStatusMeta,
  getTicketUserLabel,
  ticketTextLength,
  truncateTicketText,
} from './ticket';

const { Text, Title } = Typography;

const TicketThread = ({
  ticket,
  admin = false,
  loading = false,
  replying = false,
  closing = false,
  onReply,
  onClose,
  onRefresh,
}) => {
  const { t } = useTranslation();
  const [content, setContent] = useState('');
  const [image, setImage] = useState(null);
  const messagesEndRef = useRef(null);

  useEffect(() => {
    setContent('');
    setImage(null);
  }, [ticket?.id]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ block: 'nearest' });
  }, [ticket?.messages?.length]);

  if (loading) {
    return (
      <div className='flex min-h-[360px] items-center justify-center'>
        <Spin size='large' />
      </div>
    );
  }

  if (!ticket) {
    return (
      <div className='flex min-h-[360px] items-center justify-center'>
        <Empty description={t('请选择一条工单查看详情')} />
      </div>
    );
  }

  const statusMeta = getTicketStatusMeta(ticket.status, admin, t);
  const isClosed = ticket.status === TICKET_STATUS.CLOSED;
  const messages = Array.isArray(ticket.messages) ? ticket.messages : [];
  const messageLimitReached =
    Number(ticket.message_count ?? messages.length) >= TICKET_MESSAGE_LIMIT;
  const canReply = canReplyToTicket(ticket, admin) && !messageLimitReached;
  const replyPlaceholder = canReply
    ? t('请输入回复内容')
    : isClosed
      ? t('工单已结束，无法继续回复')
      : admin
        ? t('正在等待用户回复')
        : t('正在等待管理员回复');

  const submitReply = async () => {
    const trimmedContent = content.trim();
    if (!canReply || !trimmedContent || replying) return;
    const success = await onReply(trimmedContent, image);
    if (success) {
      setContent('');
      setImage(null);
    }
  };

  return (
    <div className='flex min-h-0 flex-col'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0'>
          <div className='flex flex-wrap items-center gap-2'>
            <Title heading={4} className='m-0 break-words'>
              {ticket.title}
            </Title>
            <Tag color={statusMeta.color} shape='circle'>
              {statusMeta.label}
            </Tag>
          </div>
          <div className='mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-[var(--semi-color-text-2)]'>
            <span>
              {t('工单编号')}: #{ticket.id}
            </span>
            {admin && <span>{getTicketUserLabel(ticket, t)}</span>}
            <span>
              {t('创建时间')}: {formatTicketTime(ticket.created_at)}
            </span>
            <span>
              {t('最近更新')}:{' '}
              {formatTicketTime(ticket.last_message_at || ticket.updated_at)}
            </span>
          </div>
        </div>
        <div className='flex items-center gap-1'>
          <Tooltip content={t('刷新工单')}>
            <Button
              aria-label={t('刷新工单')}
              icon={<RefreshCw size={16} />}
              icononly
              size='small'
              type='tertiary'
              theme='borderless'
              disabled={loading || replying || closing}
              onClick={onRefresh}
            />
          </Tooltip>
          {admin && !isClosed && (
            <Button
              size='small'
              type='danger'
              theme='light'
              icon={<CheckCircle2 size={15} />}
              loading={closing}
              disabled={replying}
              onClick={onClose}
            >
              {t('结束工单')}
            </Button>
          )}
        </div>
      </div>

      <Banner
        type={isClosed ? 'info' : canReply ? 'success' : 'warning'}
        description={
          messageLimitReached
            ? t(
                '此工单已达到 100 条消息上限，不能继续回复。管理员可以结束工单。',
              )
            : statusMeta.description
        }
        closeIcon={null}
        className='mt-3 !rounded-md'
      />

      <Divider margin='16px' />

      <div
        className='min-h-[260px] flex-1 space-y-4 overflow-y-auto pr-1'
        style={{ maxHeight: admin ? 'calc(100vh - 390px)' : '58vh' }}
        role='log'
        aria-label={`${t('工单编号')} #${ticket.id}`}
        aria-live='polite'
        aria-relevant='additions text'
      >
        {messages.length === 0 ? (
          <Empty description={t('暂无工单消息')} />
        ) : (
          messages.map((message) => {
            const fromAdmin = message.sender_role === 'admin';
            const ownMessage = admin ? fromAdmin : !fromAdmin;
            return (
              <div
                key={message.id}
                className={`flex items-start gap-2 ${ownMessage ? 'flex-row-reverse' : ''}`}
              >
                <Avatar
                  size='small'
                  color={fromAdmin ? 'blue' : 'grey'}
                  style={{ flexShrink: 0 }}
                >
                  {fromAdmin ? <ShieldCheck size={15} /> : <User size={15} />}
                </Avatar>
                <div
                  className={`min-w-0 max-w-[min(82%,720px)] ${ownMessage ? 'text-right' : 'text-left'}`}
                >
                  <div className='mb-1 flex items-center gap-2 text-xs text-[var(--semi-color-text-2)]'>
                    <span className={ownMessage ? 'ml-auto' : ''}>
                      {fromAdmin
                        ? t('管理员')
                        : admin
                          ? getTicketUserLabel(ticket, t)
                          : t('我')}
                    </span>
                    <span>{formatTicketTime(message.created_at)}</span>
                  </div>
                  <div
                    className={`inline-block max-w-full rounded-md px-3 py-2 text-left ${
                      ownMessage
                        ? 'bg-[var(--semi-color-primary)] text-white'
                        : 'bg-[var(--semi-color-fill-0)] text-[var(--semi-color-text-0)]'
                    }`}
                  >
                    <div
                      className='whitespace-pre-wrap break-words text-sm leading-6'
                      style={{ overflowWrap: 'anywhere' }}
                    >
                      {message.content}
                    </div>
                    {message.attachment && (
                      <div className='mt-2'>
                        <TicketAttachment
                          ticketId={ticket.id}
                          attachment={message.attachment}
                        />
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })
        )}
        <div ref={messagesEndRef} />
      </div>

      <Divider margin='16px' />

      <div className='space-y-2'>
        <div className='flex items-center justify-between gap-3'>
          <label
            htmlFor={`ticket-reply-${ticket.id}`}
            className='font-semibold'
          >
            {t('回复工单')}
          </label>
          <Text type='tertiary' size='small'>
            {ticketTextLength(content)}/{TICKET_CONTENT_MAX_LENGTH}
          </Text>
        </div>
        <TextArea
          id={`ticket-reply-${ticket.id}`}
          value={content}
          maxLength={TICKET_CONTENT_MAX_LENGTH * 2}
          autosize={{ minRows: 3, maxRows: 8 }}
          disabled={!canReply || replying || closing}
          placeholder={replyPlaceholder}
          onChange={(value) =>
            setContent(truncateTicketText(value, TICKET_CONTENT_MAX_LENGTH))
          }
          onKeyDown={(event) => {
            if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
              event.preventDefault();
              submitReply();
            }
          }}
        />
        <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
          <TicketImagePicker
            file={image}
            onChange={setImage}
            disabled={!canReply || replying || closing}
          />
          <Button
            theme='solid'
            type='primary'
            icon={<Send size={16} />}
            loading={replying}
            disabled={!canReply || !content.trim() || closing}
            onClick={submitReply}
          >
            {t('发送回复')}
          </Button>
        </div>
      </div>
    </div>
  );
};

export default TicketThread;
