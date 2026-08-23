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

import React from 'react';
import { Banner, Descriptions, Tag, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  formatInvoiceMoney,
  formatInvoiceTime,
  getInvoiceStatusMeta,
} from './invoice';

const { Text } = Typography;

export const InvoiceStatusTag = ({ status, t }) => {
  const meta = getInvoiceStatusMeta(status, t);
  return (
    <Tag color={meta.color} size='small' shape='circle'>
      {meta.label}
    </Tag>
  );
};

const InvoiceDetails = ({ application, admin = false, t }) => {
  const { i18n } = useTranslation();
  const detailData = [
    { key: t('Application ID'), value: `#${application.id}` },
    ...(admin
      ? [
          {
            key: t('User'),
            value:
              application.user?.display_name ||
              application.user?.username ||
              `#${application.user_id}`,
          },
          { key: t('Email'), value: application.user?.email || '-' },
        ]
      : []),
    { key: t('Invoice title'), value: application.invoice_title || '-' },
    {
      key: t('Taxpayer ID'),
      value: application.taxpayer_id || t('Not provided'),
    },
    {
      key: t('Bank name'),
      value: application.bank_name || t('Not provided'),
    },
    {
      key: t('Invoice total amount'),
      value: formatInvoiceMoney(
        application.total_amount_micros,
        application.currency,
        i18n.resolvedLanguage,
      ),
    },
    {
      key: t('Submitted at'),
      value: formatInvoiceTime(application.created_at),
    },
    ...(application.status === 'completed'
      ? [
          {
            key: t('Completed at'),
            value: formatInvoiceTime(application.completed_at),
          },
        ]
      : []),
    ...(application.status === 'rejected'
      ? [
          {
            key: t('Rejected at'),
            value: formatInvoiceTime(application.rejected_at),
          },
        ]
      : []),
  ];

  return (
    <div className='space-y-4 py-2'>
      <Descriptions data={detailData} row />
      {application.remark ? (
        <div>
          <Text type='tertiary'>{t('Remark')}</Text>
          <div
            className='mt-1 whitespace-pre-wrap text-sm'
            style={{ overflowWrap: 'anywhere' }}
          >
            {application.remark}
          </div>
        </div>
      ) : null}
      {application.status === 'rejected' ? (
        <Banner
          type='warning'
          title={t('Rejection reason')}
          description={application.rejection_reason || t('No reason provided')}
          closeIcon={null}
        />
      ) : null}
      <div>
        <Text type='tertiary'>{t('Included subscriptions')}</Text>
        <div className='mt-2 divide-y divide-[var(--semi-color-border)] rounded border border-solid border-[var(--semi-color-border)] px-3'>
          {(application.items || []).map((item) => (
            <div
              key={item.id || item.user_subscription_id}
              className='flex flex-wrap items-center justify-between gap-2 py-2 text-sm'
            >
              <span className='min-w-0' style={{ overflowWrap: 'anywhere' }}>
                {item.item_type === 'redemption_recharge'
                  ? t('Redemption code balance recharge')
                  : item.item_type === 'top_up'
                    ? item.plan_title || t('Balance recharge')
                    : item.plan_title || t('Subscription')}
              </span>
              <Text strong>
                {formatInvoiceMoney(
                  item.paid_amount_micros,
                  item.currency,
                  i18n.resolvedLanguage,
                )}
              </Text>
            </div>
          ))}
          {(application.items || []).length === 0 ? (
            <div className='py-3 text-sm text-[var(--semi-color-text-2)]'>
              {t('No subscription details')}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
};

export default InvoiceDetails;
