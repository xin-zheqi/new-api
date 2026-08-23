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
  Spin,
  Table,
  TextArea,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import { getUserIdFromLocalStorage } from '../../helpers/utils';
import InvoiceDetails, { InvoiceStatusTag } from './InvoiceDetails';
import {
  BANK_NAME_MAX_LENGTH,
  INVOICE_REMARK_MAX_LENGTH,
  INVOICE_TITLE_MAX_LENGTH,
  TAXPAYER_ID_MAX_LENGTH,
  clampInvoiceText,
  DEFAULT_PAGE_SIZE,
  formatInvoiceMoney,
  formatInvoiceTime,
  getInvoiceErrorMessage,
} from './invoice';

const { Text, Title } = Typography;

const UserInvoiceCenter = () => {
  const { t, i18n } = useTranslation();
  const [subscriptions, setSubscriptions] = useState([]);
  const [applications, setApplications] = useState([]);
  const [applicationsTotal, setApplicationsTotal] = useState(0);
  const [applicationPage, setApplicationPage] = useState(1);
  const [applicationPageSize, setApplicationPageSize] =
    useState(DEFAULT_PAGE_SIZE);
  const [applicationDay, setApplicationDay] = useState(25);
  const [applicationOpen, setApplicationOpen] = useState(false);
  const [identityEligible, setIdentityEligible] = useState(true);
  const [lookbackDays, setLookbackDays] = useState(null);
  const [monthlyLimit, setMonthlyLimit] = useState(1);
  const [remainingApplications, setRemainingApplications] = useState(0);
  const [selected, setSelected] = useState([]);
  const [form, setForm] = useState({
    invoice_title: '',
    taxpayer_id: '',
    bank_name: '',
    remark: '',
  });
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [downloadingId, setDownloadingId] = useState(null);
  const loadRequestRef = useRef(0);
  const invoiceDefaultsKey = `invoice-form-defaults-${getUserIdFromLocalStorage()}`;

  const loadData = async (
    nextPage = applicationPage,
    nextPageSize = applicationPageSize,
  ) => {
    const requestId = ++loadRequestRef.current;
    setLoading(true);
    try {
      const response = await API.get('/api/user/invoice', {
        params: { p: nextPage, size: nextPageSize },
        skipErrorHandler: true,
      });
      if (requestId !== loadRequestRef.current) return;
      if (!response.data?.success) {
        showError(
          getInvoiceErrorMessage(response, t, t('Failed to load invoice data')),
        );
        return;
      }
      const data = response.data.data || {};
      setSubscriptions(data.subscriptions || []);
      setApplications(data.applications || []);
      setApplicationsTotal(
        Number(data.applications_total ?? data.applications?.length ?? 0),
      );
      setApplicationPage(Number(data.page || nextPage));
      setApplicationPageSize(Number(data.size || nextPageSize));
      setApplicationDay(Number(data.application_day || 25));
      setApplicationOpen(data.application_open === true);
      setIdentityEligible(data.identity_eligible !== false);
      setLookbackDays(data.lookback_days ?? null);
      setMonthlyLimit(Number(data.monthly_limit || 1));
      setRemainingApplications(
        Math.max(0, Number(data.remaining_applications || 0)),
      );
      setSelected((current) => {
        const eligibleSubscriptions = data.subscriptions || [];
        const eligibleSelected = current.filter((id) =>
          eligibleSubscriptions.some((item) => item.id === id),
        );
        const currency = eligibleSubscriptions.find((item) =>
          eligibleSelected.includes(item.id),
        )?.paid_currency;
        return currency
          ? eligibleSelected.filter(
              (id) =>
                eligibleSubscriptions.find((item) => item.id === id)
                  ?.paid_currency === currency,
            )
          : [];
      });
    } catch (error) {
      if (requestId === loadRequestRef.current) {
        showError(
          getInvoiceErrorMessage(error, t, t('Failed to load invoice data')),
        );
      }
    } finally {
      if (requestId === loadRequestRef.current) setLoading(false);
    }
  };

  useEffect(() => {
    try {
      const saved = localStorage.getItem(invoiceDefaultsKey);
      if (saved) {
        const parsed = JSON.parse(saved);
        setForm((current) => ({
          ...current,
          invoice_title: clampInvoiceText(parsed.invoice_title, INVOICE_TITLE_MAX_LENGTH),
          taxpayer_id: String(parsed.taxpayer_id || '').toUpperCase().slice(0, TAXPAYER_ID_MAX_LENGTH),
          bank_name: clampInvoiceText(parsed.bank_name, BANK_NAME_MAX_LENGTH),
          remark: clampInvoiceText(parsed.remark, INVOICE_REMARK_MAX_LENGTH),
        }));
      }
    } catch (_) {
      localStorage.removeItem(invoiceDefaultsKey);
    }
    loadData(1, DEFAULT_PAGE_SIZE);
    return () => {
      loadRequestRef.current += 1;
    };
  }, []);

  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const selectedSubscriptions = useMemo(
    () => subscriptions.filter((item) => selectedSet.has(item.id)),
    [selectedSet, subscriptions],
  );
  const selectedCurrency = selectedSubscriptions[0]?.paid_currency || '';
  const availableCurrencyCount = useMemo(
    () => new Set(subscriptions.map((item) => item.paid_currency)).size,
    [subscriptions],
  );
  const totalMicros = useMemo(
    () =>
      selectedSubscriptions.reduce(
        (sum, item) => sum + Number(item.paid_amount_micros || 0),
        0,
      ),
    [selectedSubscriptions],
  );
  let disabledReason = '';
  if (!identityEligible) {
    disabledReason = t(
      'Invoice center is only available for university or enterprise users.',
    );
  } else if (!applicationOpen) {
    disabledReason = t(
      'Invoice applications are accepted only on day {{day}} of each month.',
      { day: applicationDay },
    );
  } else if (remainingApplications === 0) {
    disabledReason = t(
      "You have reached this month's invoice application limit.",
    );
  }

  const updateTextField = (field, maxLength) => (value) => {
    setForm((current) => ({
      ...current,
      [field]: clampInvoiceText(value, maxLength),
    }));
  };

  const updateTaxpayerId = (value) => {
    setForm((current) => ({
      ...current,
      taxpayer_id: String(value || '')
        .replace(/[^a-zA-Z0-9]/g, '')
        .toUpperCase()
        .slice(0, TAXPAYER_ID_MAX_LENGTH),
    }));
  };

  const submitApplication = async () => {
    const invoiceTitle = form.invoice_title.trim();
    if (!invoiceTitle) {
      showError(t('Enter the full invoice title'));
      return;
    }
    const taxpayerId = form.taxpayer_id.trim();
    if (!taxpayerId) {
      showError(t('Enter the taxpayer ID'));
      return;
    }
    if (selected.length === 0) {
      showError(t('Select at least one eligible subscription.'));
      return;
    }
    if (
      selectedSubscriptions.length !== selected.length ||
      new Set(selectedSubscriptions.map((item) => item.paid_currency)).size !==
        1
    ) {
      showError(t('Each invoice application can include only one currency.'));
      return;
    }
    setSubmitting(true);
    try {
      const selectedItems = selectedSubscriptions;
      const response = await API.post(
        '/api/user/invoice/apply',
        {
          invoice_title: invoiceTitle,
          taxpayer_id: taxpayerId,
          bank_name: form.bank_name.trim(),
          remark: form.remark.trim(),
          subscription_ids: selectedItems
            .filter((item) => item.item_type !== 'redemption_recharge')
            .map((item) => item.id),
          redemption_ids: selectedItems
            .filter((item) => item.item_type === 'redemption_recharge')
            .map((item) => item.redemption_id)
            .filter((id) => Number.isInteger(id) && id > 0),
        },
        { skipErrorHandler: true },
      );
      if (!response.data?.success) {
        showError(
          getInvoiceErrorMessage(
            response,
            t,
            t('Failed to submit invoice application'),
          ),
        );
        return;
      }
      showSuccess(t('Invoice application submitted'));
      localStorage.setItem(
        invoiceDefaultsKey,
        JSON.stringify({ ...form, invoice_title: invoiceTitle, taxpayer_id: taxpayerId }),
      );
      setSelected([]);
      setForm({
        invoice_title: '',
        taxpayer_id: '',
        bank_name: '',
        remark: '',
      });
      await loadData(1, applicationPageSize);
    } catch (error) {
      showError(
        getInvoiceErrorMessage(
          error,
          t,
          t('Failed to submit invoice application'),
        ),
      );
    } finally {
      setSubmitting(false);
    }
  };

  const downloadInvoice = async (application) => {
    setDownloadingId(application.id);
    try {
      const response = await API.get(
        `/api/user/invoice/${application.id}/download`,
        {
          responseType: 'blob',
          headers: { 'New-Api-User': String(getUserIdFromLocalStorage()) },
          skipErrorHandler: true,
        },
      );
      const url = URL.createObjectURL(response.data);
      const link = document.createElement('a');
      link.href = url;
      link.download = `invoice-${application.id}.pdf`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (error) {
      showError(
        getInvoiceErrorMessage(error, t, t('Failed to download invoice')),
      );
    } finally {
      setDownloadingId(null);
    }
  };

  const subscriptionColumns = [
    {
      title: t('Subscription plan'),
      dataIndex: 'plan_title',
      render: (value, record) => {
        const title = record?.item_type === 'redemption_recharge'
            ? t('Redemption code balance recharge')
            : record?.item_type === 'top_up'
              ? value || t('Balance recharge')
              : value || t('Subscription');
        return (
          <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 280 }}>
            {title}
          </Text>
        );
      },
    },
    {
      title: t('Purchased at'),
      dataIndex: 'created_at',
      render: formatInvoiceTime,
    },
    {
      title: t('Invoiceable amount'),
      dataIndex: 'paid_amount_micros',
      align: 'right',
      render: (value, subscription) => (
        <Text strong>
          {formatInvoiceMoney(
            value,
            subscription.paid_currency,
            i18n.resolvedLanguage,
          )}
        </Text>
      ),
    },
  ];

  const applicationColumns = [
    {
      title: t('Application'),
      render: (_, application) => (
        <div className='min-w-[180px]'>
          <Text strong>#{application.id}</Text>
          <div className='text-xs text-[var(--semi-color-text-2)]'>
            {formatInvoiceTime(application.created_at)}
          </div>
        </div>
      ),
    },
    {
      title: t('Invoice title'),
      dataIndex: 'invoice_title',
      render: (value) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 260 }}>
          {value}
        </Text>
      ),
    },
    {
      title: t('Amount'),
      dataIndex: 'total_amount_micros',
      align: 'right',
      render: (value, application) =>
        formatInvoiceMoney(value, application.currency, i18n.resolvedLanguage),
    },
    {
      title: t('Status'),
      dataIndex: 'status',
      render: (status) => <InvoiceStatusTag status={status} t={t} />,
    },
    {
      title: t('Actions'),
      fixed: 'right',
      render: (_, application) =>
        application.status === 'completed' && application.pdf_name ? (
          <Button
            size='small'
            loading={downloadingId === application.id}
            onClick={() => downloadInvoice(application)}
          >
            {t('Download PDF')}
          </Button>
        ) : (
          <Text type='tertiary'>-</Text>
        ),
    },
  ];

  return (
    <div className='mx-auto w-full max-w-7xl space-y-4 px-2 pb-6'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <Title heading={3} className='m-0'>
            {t('Invoice Center')}
          </Title>
          <Text type='tertiary'>
            {t('Submit invoice details and track processing results.')}
          </Text>
        </div>
        <Tooltip content={t('Refresh')}>
          <Button
            aria-label={t('Refresh')}
            icon={<RefreshCw size={16} />}
            icononly
            type='tertiary'
            theme='outline'
            loading={loading}
            onClick={() => loadData(applicationPage, applicationPageSize)}
          />
        </Tooltip>
      </div>

      <Banner
        type='info'
        description={t(
          'Applications open on day {{day}} of each month for eligible subscriptions from the past {{days}} days. Each user may submit up to {{limit}} applications per month.',
          {
            day: applicationDay,
            days: lookbackDays ?? '-',
            limit: monthlyLimit,
          },
        )}
        closeIcon={null}
      />

      <Spin spinning={loading}>
        <Card className='!rounded-lg'>
          <div className='mb-4'>
            <Title heading={5} className='m-0'>
              {t('New invoice application')}
            </Title>
            <Text type='tertiary'>
              {t(
                'Verify all invoice information carefully. Completed invoices cannot be changed or reissued.',
              )}
            </Text>
          </div>

          <div className='grid gap-4 md:grid-cols-2'>
            <label className='space-y-1'>
              <Text strong>{t('Invoice title')}</Text>
              <Input
                aria-label={t('Invoice title')}
                value={form.invoice_title}
                onChange={updateTextField(
                  'invoice_title',
                  INVOICE_TITLE_MAX_LENGTH,
                )}
                placeholder={t('Enter the full invoice title')}
                showClear
              />
            </label>
            <label className='space-y-1'>
              <Text strong>{t('Taxpayer ID')}</Text>
              <Input
                aria-label={t('Taxpayer ID')}
                value={form.taxpayer_id}
                onChange={updateTaxpayerId}
                placeholder={t('Letters and numbers only')}
                showClear
              />
            </label>
            <label className='space-y-1 md:col-span-2'>
              <Text strong>{t('Bank name')}</Text>
              <Input
                aria-label={t('Bank name')}
                value={form.bank_name}
                onChange={updateTextField('bank_name', BANK_NAME_MAX_LENGTH)}
                placeholder={t('Optional bank account name')}
                showClear
              />
            </label>
            <label className='space-y-1 md:col-span-2'>
              <Text strong>{t('Remark')}</Text>
              <TextArea
                aria-label={t('Remark')}
                value={form.remark}
                onChange={updateTextField('remark', INVOICE_REMARK_MAX_LENGTH)}
                placeholder={t('Optional invoice notes')}
                autosize={{ minRows: 3, maxRows: 6 }}
                showClear
              />
              <div className='text-right text-xs text-[var(--semi-color-text-2)]'>
                {Array.from(form.remark).length}/{INVOICE_REMARK_MAX_LENGTH}
              </div>
            </label>
          </div>

          <div className='mb-2 mt-5 flex flex-wrap items-center justify-between gap-2'>
            <div>
              <Text strong>{t('Eligible subscriptions')}</Text>
              <div className='text-xs text-[var(--semi-color-text-2)]'>
                {t('{{count}} selected, total {{amount}}', {
                  count: selected.length,
                  amount: formatInvoiceMoney(
                    totalMicros,
                    selectedCurrency,
                    i18n.resolvedLanguage,
                  ),
                })}
                {availableCurrencyCount > 1
                  ? ` ${t('Each invoice application can include only one currency.')}`
                  : ''}
              </div>
            </div>
            <Button
              theme='solid'
              loading={submitting}
              disabled={
                Boolean(disabledReason) ||
                selected.length === 0 ||
                !form.invoice_title.trim() ||
                submitting
              }
              onClick={submitApplication}
            >
              {t('Apply for invoice')}
            </Button>
          </div>
          {disabledReason ? (
            <Banner
              type='warning'
              description={disabledReason}
              closeIcon={null}
              className='mb-3'
            />
          ) : null}
          <Table
            columns={subscriptionColumns}
            dataSource={subscriptions}
            rowKey='id'
            rowSelection={{
              selectedRowKeys: selected,
              getCheckboxProps: (subscription) => ({
                disabled: Boolean(
                  selectedCurrency &&
                  subscription.paid_currency !== selectedCurrency,
                ),
                title:
                  selectedCurrency &&
                  subscription.paid_currency !== selectedCurrency
                    ? t(
                        'Each invoice application can include only one currency.',
                      )
                    : undefined,
              }),
              onChange: (keys) => {
                const nextKeys = keys.map(Number);
                const nextSubscriptions = subscriptions.filter((item) =>
                  nextKeys.includes(item.id),
                );
                const nextCurrencies = new Set(
                  nextSubscriptions.map((item) => item.paid_currency),
                );
                if (nextCurrencies.size <= 1) {
                  setSelected(nextKeys);
                  return;
                }
                const currency =
                  selectedCurrency || nextSubscriptions[0]?.paid_currency;
                setSelected(
                  nextSubscriptions
                    .filter((item) => item.paid_currency === currency)
                    .map((item) => item.id),
                );
                showError(
                  t('Each invoice application can include only one currency.'),
                );
              },
            }}
            pagination={false}
            style={{ width: '100%' }}
            scroll={{ x: '100%' }}
            empty={<Empty description={t('No eligible subscriptions')} />}
          />
        </Card>

        <Card className='mt-4 !rounded-lg'>
          <Title heading={5} className='mb-3 mt-0'>
            {t('Application history')}
          </Title>
          <Table
            columns={applicationColumns}
            dataSource={applications}
            rowKey='id'
            expandedRowRender={(application) => (
              <InvoiceDetails application={application} t={t} />
            )}
            pagination={{
              currentPage: applicationPage,
              pageSize: applicationPageSize,
              total: applicationsTotal,
              showSizeChanger: true,
              pageSizeOpts: [10, 20, 50, 100],
              onPageChange: (nextPage) =>
                loadData(nextPage, applicationPageSize),
              onPageSizeChange: (nextPageSize) => loadData(1, nextPageSize),
            }}
            style={{ width: '100%' }}
            scroll={{ x: '100%' }}
            empty={<Empty description={t('No invoice applications')} />}
          />
        </Card>
      </Spin>
    </div>
  );
};

export default UserInvoiceCenter;
