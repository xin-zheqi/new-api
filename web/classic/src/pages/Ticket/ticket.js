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

import dayjs from 'dayjs';

export const TICKET_STATUS = {
  WAITING_ADMIN: 'waiting_admin',
  WAITING_USER: 'waiting_user',
  CLOSED: 'closed',
};

export const TICKET_MAX_IMAGE_SIZE = 5 * 1024 * 1024;
export const TICKET_TITLE_MAX_LENGTH = 120;
export const TICKET_CONTENT_MAX_LENGTH = 4000;
export const TICKET_MESSAGE_LIMIT = 100;
export const TICKET_IMAGE_ACCEPT = 'image/jpeg,image/png,image/webp';
export const TICKET_ALLOWED_IMAGE_TYPES = new Set([
  'image/jpeg',
  'image/png',
  'image/webp',
]);

export function formatTicketTime(value) {
  if (!value) return '-';
  const numericValue = Number(value);
  const date = Number.isFinite(numericValue)
    ? dayjs(numericValue < 1000000000000 ? numericValue * 1000 : numericValue)
    : dayjs(value);
  return date.isValid() ? date.format('YYYY-MM-DD HH:mm') : '-';
}

export function ticketTextLength(value) {
  return Array.from(value || '').length;
}

export function truncateTicketText(value, maxLength) {
  return Array.from(value || '')
    .slice(0, maxLength)
    .join('');
}

export function getTicketStatusMeta(status, admin, t) {
  switch (status) {
    case TICKET_STATUS.WAITING_ADMIN:
      return {
        color: 'orange',
        label: admin ? t('待管理员处理') : t('等待管理员回复'),
        description: admin
          ? t('现在轮到管理员回复此工单。')
          : t('管理员正在处理，收到管理员回复前您暂时不能继续回复。'),
      };
    case TICKET_STATUS.WAITING_USER:
      return {
        color: 'blue',
        label: admin ? t('等待用户回复') : t('待您回复'),
        description: admin
          ? t('管理员已经回复，正在等待用户补充信息。')
          : t('管理员已回复，现在可以继续补充信息。'),
      };
    case TICKET_STATUS.CLOSED:
      return {
        color: 'grey',
        label: t('已结束'),
        description: admin
          ? t('此工单已经结束，不能继续回复。')
          : t('此工单已经结束。如仍有问题，可以创建新工单。'),
      };
    default:
      return {
        color: 'grey',
        label: status || t('未知状态'),
        description: t('工单状态异常，请刷新后重试。'),
      };
  }
}

export function canReplyToTicket(ticket, admin) {
  if (!ticket) return false;
  return admin
    ? ticket.status === TICKET_STATUS.WAITING_ADMIN
    : ticket.status === TICKET_STATUS.WAITING_USER;
}

export function getTicketUserLabel(ticket, t) {
  if (ticket?.display_name) return ticket.display_name;
  if (ticket?.username) return ticket.username;
  if (ticket?.email) return ticket.email;
  return ticket?.user_id ? `${t('用户')} #${ticket.user_id}` : t('未知用户');
}

export function validateTicketImage(file, t) {
  if (!file) return null;
  if (!TICKET_ALLOWED_IMAGE_TYPES.has(file.type)) {
    return t('仅支持 JPG、PNG 或 WebP 图片。');
  }
  if (file.size <= 0 || file.size > TICKET_MAX_IMAGE_SIZE) {
    return t('图片大小必须大于 0 且不能超过 5 MiB。');
  }
  return null;
}

export function getTicketErrorMessage(error, t, fallback) {
  if (error?.response?.status === 429) {
    return t('请求过于频繁，请稍后重试。');
  }
  const code = error?.code || error?.response?.data?.code;
  switch (code) {
    case 'ticket_not_found':
      return t('工单不存在或您无权访问。');
    case 'ticket_active_exists':
      return t('当前已有进行中的工单，结束后才能创建新工单。');
    case 'ticket_waiting_admin':
      return t('当前正在等待管理员回复，用户暂时不能继续回复。');
    case 'ticket_waiting_user':
      return t('当前正在等待用户回复，管理员暂时不能继续回复。');
    case 'ticket_closed':
      return t('工单已结束，不能继续操作。');
    case 'ticket_message_limit':
      return t('此工单已达到 100 条消息上限。');
    case 'ticket_state_changed':
      return t('工单状态已经变化，请刷新后重试。');
    case 'ticket_invalid_filter':
      return t('工单筛选条件无效。');
    case 'ticket_user_id_invalid':
      return t('请输入有效的用户 ID。');
    case 'ticket_request_too_large':
      return t('提交内容过大，请缩小图片后重试。');
    case 'ticket_invalid_multipart':
      return t('上传请求无效，请重新选择图片后重试。');
    case 'ticket_invalid_fields':
      return t('提交的工单字段无效。');
    case 'ticket_missing_fields':
      return t('请完整填写标题和内容。');
    case 'ticket_image_count_invalid':
      return t('每条消息最多只能上传一张图片。');
    case 'ticket_title_invalid':
      return t('标题必须为 1 到 120 个字符。');
    case 'ticket_content_invalid':
      return t('内容必须为 1 到 4000 个字符。');
    case 'ticket_image_size_invalid':
      return t('图片大小必须大于 0 且不能超过 5 MiB。');
    case 'ticket_image_name_invalid':
      return t('图片文件名无效，请重新选择图片。');
    case 'ticket_image_type_invalid':
      return t('仅支持 JPG、PNG 或 WebP 图片。');
    case 'ticket_image_dimensions_invalid':
      return t('图片尺寸无效或超出限制。');
    case 'ticket_image_extension_mismatch':
      return t('图片扩展名与实际格式不一致。');
    case 'ticket_image_mime_mismatch':
      return t('图片类型与实际格式不一致。');
    case 'ticket_image_busy':
      return t('图片正在处理中，请稍后重试。');
    case 'ticket_request_failed':
      return t('工单请求失败，请稍后重试。');
    case 'ticket_operation_failed':
      return t('工单操作失败，请稍后重试。');
    default:
      return fallback;
  }
}
