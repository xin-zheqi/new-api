import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Card,
  Checkbox,
  Input,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, renderQuota, showError, showSuccess } from '../../helpers';
import { getUserIdFromLocalStorage } from '../../helpers/utils';

const InvoiceCenter = ({ admin = false }) => {
  const { t } = useTranslation();
  const [subscriptions, setSubscriptions] = useState([]);
  const [applications, setApplications] = useState([]);
  const [applicationDay, setApplicationDay] = useState(25);
  const [lookbackDays, setLookbackDays] = useState(null);
  const [title, setTitle] = useState('');
  const [selected, setSelected] = useState([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [uploadingId, setUploadingId] = useState(null);
  const fileInput = useRef(null);

  const loadData = async () => {
    setLoading(true);
    try {
      const response = await API.get(
        admin ? '/api/invoice/admin/applications' : '/api/user/invoice',
      );
      if (!response.data?.success) {
        showError(response.data?.message || t('Failed to load invoice data'));
        return;
      }
      if (admin) {
        setApplications(response.data.data || []);
      } else {
        setSubscriptions(response.data.data?.subscriptions || []);
        setApplications(response.data.data?.applications || []);
        setApplicationDay(response.data.data?.application_day || 25);
        setLookbackDays(response.data.data?.lookback_days ?? null);
      }
    } catch (error) {
      showError(t('Failed to load invoice data'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [admin]);

  const total = useMemo(
    () =>
      subscriptions
        .filter((item) => selected.includes(item.id))
        .reduce((sum, item) => sum + item.amount_total, 0),
    [subscriptions, selected],
  );

  const submitApplication = async () => {
    setSubmitting(true);
    try {
      const response = await API.post('/api/user/invoice/apply', {
        invoice_title: title,
        subscription_ids: selected,
      });
      if (!response.data?.success) {
        showError(
          response.data?.message || t('Failed to submit invoice application'),
        );
        return;
      }
      showSuccess(t('Invoice application submitted'));
      setSelected([]);
      setTitle('');
      await loadData();
    } catch (error) {
      showError(t('Failed to submit invoice application'));
    } finally {
      setSubmitting(false);
    }
  };

  const uploadPdf = async (id, file) => {
    const formData = new FormData();
    formData.append('file', file);
    setUploadingId(id);
    try {
      const response = await API.post(
        `/api/invoice/admin/applications/${id}/pdf`,
        formData,
      );
      if (!response.data?.success) {
        showError(response.data?.message || t('Failed to upload invoice'));
        return;
      }
      showSuccess(t('Invoice uploaded'));
      await loadData();
    } catch (error) {
      showError(t('Failed to upload invoice'));
    } finally {
      setUploadingId(null);
    }
  };

  const deletePdf = async (id) => {
    try {
      const response = await API.delete(
        `/api/invoice/admin/applications/${id}/pdf`,
      );
      if (!response.data?.success) {
        showError(response.data?.message || t('Failed to delete invoice'));
        return;
      }
      await loadData();
    } catch (error) {
      showError(t('Failed to delete invoice'));
    }
  };

  const completeApplication = async (id) => {
    try {
      const response = await API.post(
        `/api/invoice/admin/applications/${id}/complete`,
      );
      if (!response.data?.success) {
        showError(response.data?.message || t('Failed to complete invoice'));
        return;
      }
      showSuccess(t('Invoice completed'));
      await loadData();
    } catch (error) {
      showError(t('Failed to complete invoice'));
    }
  };

  const downloadInvoice = async (id) => {
    try {
      const response = await API.get(`/api/user/invoice/${id}/download`, {
        responseType: 'blob',
        headers: { 'New-Api-User': String(getUserIdFromLocalStorage()) },
      });
      const url = URL.createObjectURL(response.data);
      const link = document.createElement('a');
      link.href = url;
      link.download = 'invoice.pdf';
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (error) {
      showError(t('Failed to download invoice'));
    }
  };

  if (loading) return <Typography.Text>{t('Loading...')}</Typography.Text>;

  return (
    <div className='invoice-center-page mx-auto w-full max-w-6xl space-y-5 p-4 sm:p-6'>
      <Typography.Title heading={3}>
        {admin ? t('Invoice Management Center') : t('Invoice Center')}
      </Typography.Title>
      {!admin && (
        <Card>
          {lookbackDays !== null && (
            <Typography.Paragraph type='tertiary'>
              {t(
                'Applications open on day {{day}} of each month for eligible subscriptions from the past {{days}} days.',
                { day: applicationDay, days: lookbackDays },
              )}
            </Typography.Paragraph>
          )}
          <Typography.Paragraph type='danger'>
            {t(
              'Please verify the invoice title carefully. Issued invoices cannot be changed or reissued.',
            )}
          </Typography.Paragraph>
          <div className='space-y-2'>
            {subscriptions.map((subscription) => (
              <div
                key={subscription.id}
                className='flex items-center gap-3 border-b py-2'
              >
                <Checkbox
                  checked={selected.includes(subscription.id)}
                  onChange={(event) =>
                    setSelected((current) =>
                      event.target.checked
                        ? [...current, subscription.id]
                        : current.filter((id) => id !== subscription.id),
                    )
                  }
                />
                <span className='flex-1'>{subscription.plan_title}</span>
                <span>{renderQuota(subscription.amount_total)}</span>
              </div>
            ))}
            {!subscriptions.length && (
              <Typography.Text type='tertiary'>
                {t('No eligible subscriptions')}
              </Typography.Text>
            )}
          </div>
          <div className='mt-4 grid gap-2 md:grid-cols-[minmax(0,1fr)_auto] md:items-end'>
            <Input
              value={title}
              onChange={setTitle}
              placeholder={t('Enter the full invoice title')}
              maxLength={255}
            />
            <Button
              theme='solid'
              disabled={!selected.length || !title.trim() || submitting}
              loading={submitting}
              onClick={submitApplication}
            >
              {t('Apply for invoice')} ({renderQuota(total)})
            </Button>
          </div>
        </Card>
      )}
      <Card>
        {applications.map((application) => (
          <div
            key={application.id}
            className='flex flex-wrap items-center gap-3 border-b py-3'
          >
            <div className='min-w-0 flex-1'>
              <div className='font-medium'>
                {admin ? application.user?.username : application.invoice_title}
              </div>
              <div className='text-sm text-gray-500'>
                {admin
                  ? application.invoice_title
                  : renderQuota(application.total_amount)}
              </div>
              {admin && (
                <div className='text-sm text-gray-500'>
                  {t('Invoice total amount')}:{' '}
                  {renderQuota(application.total_amount)}
                </div>
              )}
            </div>
            <Tag
              color={application.status === 'completed' ? 'green' : 'orange'}
            >
              {application.status === 'completed'
                ? t('Completed')
                : t('Pending')}
            </Tag>
            {admin ? (
              <>
                <input
                  ref={fileInput}
                  type='file'
                  accept='.pdf,application/pdf'
                  className='hidden'
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    if (file && uploadingId) uploadPdf(uploadingId, file);
                    event.target.value = '';
                  }}
                />
                <Button
                  size='small'
                  onClick={() => {
                    setUploadingId(application.id);
                    fileInput.current?.click();
                  }}
                >
                  {application.pdf_name ? t('Replace PDF') : t('Upload PDF')}
                </Button>
                {application.pdf_name && (
                  <Button
                    size='small'
                    type='danger'
                    onClick={() => deletePdf(application.id)}
                  >
                    {t('Delete PDF')}
                  </Button>
                )}
                {application.status === 'pending' && (
                  <Button
                    size='small'
                    theme='solid'
                    disabled={!application.pdf_name}
                    onClick={() => completeApplication(application.id)}
                  >
                    {t('Complete invoice')}
                  </Button>
                )}
              </>
            ) : application.status === 'completed' && application.pdf_name ? (
              <Button
                size='small'
                onClick={() => downloadInvoice(application.id)}
              >
                {t('Download PDF')}
              </Button>
            ) : null}
          </div>
        ))}
        {!applications.length && (
          <Typography.Text type='tertiary'>
            {t('No invoice applications')}
          </Typography.Text>
        )}
      </Card>
    </div>
  );
};

export default InvoiceCenter;
