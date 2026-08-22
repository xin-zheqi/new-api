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
  Button,
  Empty,
  Input,
  Modal,
  Select,
  SideSheet,
  Space,
  Tag,
  TextArea,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  CheckCircle2,
  Eye,
  FileDown,
  FileUp,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  XCircle,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import CardPro from '../../components/common/ui/CardPro';
import CardTable from '../../components/common/ui/CardTable';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { API, showError, showSuccess } from '../../helpers';
import { createCardProPagination } from '../../helpers/utils';
import InvoiceDetails, { InvoiceStatusTag } from './InvoiceDetails';
import {
  DEFAULT_PAGE_SIZE,
  INVOICE_SEARCH_MAX_LENGTH,
  REJECTION_REASON_MAX_LENGTH,
  clampInvoiceText,
  formatInvoiceMoney,
  formatInvoiceTime,
  getInvoiceErrorMessage,
  validateInvoicePdf,
} from './invoice';

const { Text, Title } = Typography;

const AdminInvoiceCenter = () => {
  const { t, i18n } = useTranslation();
  const isMobile = useIsMobile();
  const [applications, setApplications] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [filters, setFilters] = useState({
    status: '',
    keyword: '',
    userId: '',
  });
  const [draftFilters, setDraftFilters] = useState(filters);
  const [loading, setLoading] = useState(true);
  const [actionKey, setActionKey] = useState('');
  const [detail, setDetail] = useState(null);
  const [rejectTarget, setRejectTarget] = useState(null);
  const [rejectionReason, setRejectionReason] = useState('');
  const uploadInputRef = useRef(null);
  const uploadTargetIdRef = useRef(null);
  const loadRequestRef = useRef(0);

  const loadApplications = async (
    nextPage = page,
    nextPageSize = pageSize,
    nextFilters = filters,
  ) => {
    const requestId = ++loadRequestRef.current;
    setLoading(true);
    try {
      const params = { p: nextPage, size: nextPageSize };
      if (nextFilters.status) params.status = nextFilters.status;
      if (nextFilters.keyword.trim()) {
        params.keyword = nextFilters.keyword.trim();
      }
      if (nextFilters.userId) params.user_id = Number(nextFilters.userId);

      const response = await API.get('/api/invoice/admin/applications', {
        params,
        skipErrorHandler: true,
      });
      if (requestId !== loadRequestRef.current) return;
      if (!response.data?.success) {
        showError(
          getInvoiceErrorMessage(response, t, t('Failed to load invoice data')),
        );
        return;
      }
      const records = response.data.data || [];
      setApplications(records);
      setTotal(Number(response.data.total || 0));
      setPage(Number(response.data.page || nextPage));
      setPageSize(Number(response.data.size || nextPageSize));
      setDetail((current) => {
        if (!current) return null;
        return records.find((item) => item.id === current.id) || current;
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
    loadApplications(1, DEFAULT_PAGE_SIZE, {
      status: '',
      keyword: '',
      userId: '',
    });
    return () => {
      loadRequestRef.current += 1;
    };
  }, []);

  const applyFilters = () => {
    const userId = draftFilters.userId.trim();
    const numericUserId = Number(userId);
    if (
      userId &&
      (!/^[1-9]\d*$/.test(userId) ||
        !Number.isSafeInteger(numericUserId) ||
        numericUserId <= 0)
    ) {
      showError(t('Enter a valid user ID.'));
      return;
    }
    const nextFilters = {
      status: draftFilters.status || '',
      keyword: draftFilters.keyword.trim(),
      userId,
    };
    setFilters(nextFilters);
    loadApplications(1, pageSize, nextFilters);
  };

  const resetFilters = () => {
    const emptyFilters = { status: '', keyword: '', userId: '' };
    setDraftFilters(emptyFilters);
    setFilters(emptyFilters);
    loadApplications(1, pageSize, emptyFilters);
  };

  const executeAction = async (id, action, request, successMessage) => {
    setActionKey(`${action}:${id}`);
    try {
      const response = await request();
      if (!response.data?.success) {
        showError(
          getInvoiceErrorMessage(response, t, t('Invoice operation failed')),
        );
        return false;
      }
      showSuccess(successMessage);
      await loadApplications(page, pageSize, filters);
      return true;
    } catch (error) {
      showError(
        getInvoiceErrorMessage(error, t, t('Invoice operation failed')),
      );
      return false;
    } finally {
      setActionKey('');
    }
  };

  const choosePdf = (id) => {
    uploadTargetIdRef.current = id;
    uploadInputRef.current?.click();
  };

  const uploadPdf = async (event) => {
    const file = event.target.files?.[0];
    const id = uploadTargetIdRef.current;
    event.target.value = '';
    uploadTargetIdRef.current = null;
    if (!file || !id) return;

    const validationError = await validateInvoicePdf(file, t);
    if (validationError) {
      showError(validationError);
      return;
    }
    const formData = new FormData();
    formData.append('file', file);
    await executeAction(
      id,
      'upload',
      () =>
        API.post(`/api/invoice/admin/applications/${id}/pdf`, formData, {
          skipErrorHandler: true,
        }),
      t('Invoice uploaded'),
    );
  };

  const downloadAdminInvoice = async (application) => {
    const id = application.id;
    setActionKey(`download:${id}`);
    try {
      const response = await API.get(
        `/api/invoice/admin/applications/${id}/download`,
        { responseType: 'blob', skipErrorHandler: true },
      );
      const url = URL.createObjectURL(response.data);
      const link = document.createElement('a');
      link.href = url;
      link.download = `invoice-${id}.pdf`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (error) {
      showError(
        getInvoiceErrorMessage(error, t, t('Failed to download invoice')),
      );
    } finally {
      setActionKey('');
    }
  };

  const deletePdf = (application) => {
    Modal.confirm({
      title: t('Delete uploaded PDF?'),
      content: t(
        'The administrator must upload a PDF again before completion.',
      ),
      okText: t('Delete PDF'),
      cancelText: t('Cancel'),
      okButtonProps: { type: 'danger' },
      onOk: () =>
        executeAction(
          application.id,
          'delete',
          () =>
            API.delete(
              `/api/invoice/admin/applications/${application.id}/pdf`,
              { skipErrorHandler: true },
            ),
          t('Invoice PDF deleted'),
        ),
    });
  };

  const completeApplication = (application) => {
    Modal.confirm({
      title: t('Complete this invoice application?'),
      content: t('The user will be able to download the uploaded PDF.'),
      okText: t('Complete invoice'),
      cancelText: t('Cancel'),
      onOk: () =>
        executeAction(
          application.id,
          'complete',
          () =>
            API.post(
              `/api/invoice/admin/applications/${application.id}/complete`,
              null,
              { skipErrorHandler: true },
            ),
          t('Invoice completed'),
        ),
    });
  };

  const rejectApplication = async () => {
    const reason = rejectionReason.trim();
    if (!reason) {
      showError(t('Enter a rejection reason.'));
      return;
    }
    const id = rejectTarget?.id;
    if (!id) return;
    const success = await executeAction(
      id,
      'reject',
      () =>
        API.post(
          `/api/invoice/admin/applications/${id}/reject`,
          { reason },
          { skipErrorHandler: true },
        ),
      t('Invoice application rejected'),
    );
    if (success) {
      setRejectTarget(null);
      setRejectionReason('');
    }
  };

  const columns = [
    {
      title: t('ID'),
      dataIndex: 'id',
      width: 80,
      render: (id) => <Text strong>#{id}</Text>,
    },
    {
      title: t('User'),
      render: (_, application) => (
        <div className='min-w-[180px]'>
          <Text strong>
            {application.user?.display_name ||
              application.user?.username ||
              `#${application.user_id}`}
          </Text>
          <div className='text-xs text-[var(--semi-color-text-2)]'>
            {application.user?.email || `ID: ${application.user_id}`}
          </div>
        </div>
      ),
    },
    {
      title: t('Invoice details'),
      render: (_, application) => (
        <div className='min-w-[220px]'>
          <Text
            ellipsis={{ showTooltip: true }}
            style={{ display: 'block', maxWidth: 280 }}
          >
            {application.invoice_title}
          </Text>
          <div className='text-xs text-[var(--semi-color-text-2)]'>
            {application.taxpayer_id || t('No taxpayer ID')}
          </div>
        </div>
      ),
    },
    {
      title: t('Amount'),
      dataIndex: 'total_amount_micros',
      align: 'right',
      render: (value, application) => (
        <Text strong>
          {formatInvoiceMoney(
            value,
            application.currency,
            i18n.resolvedLanguage,
          )}
        </Text>
      ),
    },
    {
      title: t('Status'),
      dataIndex: 'status',
      render: (status) => <InvoiceStatusTag status={status} t={t} />,
    },
    {
      title: t('Submitted at'),
      dataIndex: 'created_at',
      render: formatInvoiceTime,
    },
    {
      title: t('Actions'),
      fixed: 'right',
      render: (_, application) => {
        const pending = application.status === 'pending';
        return (
          <Space spacing={4} wrap>
            <Tooltip content={t('View details')}>
              <Button
                aria-label={t('View details')}
                icon={<Eye size={15} />}
                icononly
                size='small'
                type='tertiary'
                theme='outline'
                onClick={() => setDetail(application)}
              />
            </Tooltip>
            {application.pdf_name ? (
              <Tooltip content={t('Download PDF')}>
                <Button
                  aria-label={t('Download PDF')}
                  icon={<FileDown size={15} />}
                  icononly
                  size='small'
                  type='tertiary'
                  theme='outline'
                  loading={actionKey === `download:${application.id}`}
                  disabled={Boolean(actionKey)}
                  onClick={() => downloadAdminInvoice(application)}
                />
              </Tooltip>
            ) : null}
            {pending ? (
              <>
                <Button
                  size='small'
                  icon={<FileUp size={15} />}
                  loading={actionKey === `upload:${application.id}`}
                  disabled={Boolean(actionKey)}
                  onClick={() => choosePdf(application.id)}
                >
                  {application.pdf_name ? t('Replace PDF') : t('Upload PDF')}
                </Button>
                {application.pdf_name ? (
                  <Tooltip content={t('Delete PDF')}>
                    <Button
                      aria-label={t('Delete PDF')}
                      size='small'
                      type='danger'
                      icon={<Trash2 size={15} />}
                      icononly
                      disabled={Boolean(actionKey)}
                      onClick={() => deletePdf(application)}
                    />
                  </Tooltip>
                ) : null}
                <Button
                  size='small'
                  theme='solid'
                  icon={<CheckCircle2 size={15} />}
                  disabled={!application.pdf_name || Boolean(actionKey)}
                  loading={actionKey === `complete:${application.id}`}
                  onClick={() => completeApplication(application)}
                >
                  {t('Complete')}
                </Button>
                <Button
                  size='small'
                  type='danger'
                  icon={<XCircle size={15} />}
                  disabled={Boolean(actionKey)}
                  onClick={() => {
                    setRejectTarget(application);
                    setRejectionReason('');
                  }}
                >
                  {t('Reject')}
                </Button>
              </>
            ) : null}
          </Space>
        );
      },
    },
  ];

  const searchArea = (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-col gap-2 md:flex-row md:items-center'>
        <Input
          aria-label={t('Search invoice title, taxpayer ID, user, or email')}
          prefix={<Search size={15} />}
          value={draftFilters.keyword}
          onChange={(value) =>
            setDraftFilters((current) => ({
              ...current,
              keyword: clampInvoiceText(value, INVOICE_SEARCH_MAX_LENGTH),
            }))
          }
          onEnterPress={applyFilters}
          placeholder={t('Search invoice title, taxpayer ID, user, or email')}
          showClear
          style={{ minWidth: 260, flex: 1 }}
        />
        <Input
          aria-label={t('User ID')}
          value={draftFilters.userId}
          onChange={(value) =>
            setDraftFilters((current) => ({
              ...current,
              userId: String(value || '')
                .replace(/\D/g, '')
                .slice(0, 15),
            }))
          }
          onEnterPress={applyFilters}
          placeholder={t('User ID')}
          showClear
          style={{ width: isMobile ? '100%' : 150 }}
        />
        <Select
          aria-label={t('Status')}
          value={draftFilters.status}
          onChange={(value) =>
            setDraftFilters((current) => ({
              ...current,
              status: value || '',
            }))
          }
          style={{ width: isMobile ? '100%' : 160 }}
        >
          <Select.Option value=''>{t('All statuses')}</Select.Option>
          <Select.Option value='pending'>{t('Pending')}</Select.Option>
          <Select.Option value='completed'>{t('Completed')}</Select.Option>
          <Select.Option value='rejected'>{t('Rejected')}</Select.Option>
        </Select>
        <Button
          theme='solid'
          icon={<Search size={15} />}
          onClick={applyFilters}
        >
          {t('Search')}
        </Button>
        <Tooltip content={t('Reset filters')}>
          <Button
            aria-label={t('Reset filters')}
            icon={<RotateCcw size={15} />}
            icononly
            type='tertiary'
            theme='outline'
            onClick={resetFilters}
          />
        </Tooltip>
        <Tooltip content={t('Refresh')}>
          <Button
            aria-label={t('Refresh')}
            icon={<RefreshCw size={15} />}
            icononly
            type='tertiary'
            theme='outline'
            loading={loading}
            onClick={() => loadApplications(page, pageSize, filters)}
          />
        </Tooltip>
      </div>
    </div>
  );

  return (
    <div className='w-full px-2 pb-6'>
      <input
        ref={uploadInputRef}
        type='file'
        accept='.pdf,application/pdf'
        className='hidden'
        onChange={uploadPdf}
      />
      <CardPro
        type='type1'
        className='!rounded-lg'
        descriptionArea={
          <div className='flex flex-wrap items-start justify-between gap-2'>
            <div>
              <Title heading={4} className='m-0'>
                {t('Invoice Management')}
              </Title>
              <Text type='secondary'>
                {t(
                  'Search applications, verify billing details, and complete invoice processing from one workspace.',
                )}
              </Text>
            </div>
            <Tag color='blue' shape='circle'>
              {t('{{count}} applications', { count: total })}
            </Tag>
          </div>
        }
        searchArea={searchArea}
        paginationArea={createCardProPagination({
          currentPage: page,
          pageSize,
          total,
          onPageChange: (nextPage) =>
            loadApplications(nextPage, pageSize, filters),
          onPageSizeChange: (size) => loadApplications(1, size, filters),
          isMobile,
          pageSizeOpts: [10, 20, 50, 100],
          t,
        })}
        t={t}
      >
        <CardTable
          columns={columns}
          dataSource={applications}
          loading={loading}
          rowKey='id'
          scroll={{ x: 'max-content' }}
          pagination={false}
          hidePagination
          empty={<Empty description={t('No matching invoice applications')} />}
        />
      </CardPro>

      <SideSheet
        visible={Boolean(detail)}
        placement='right'
        width={isMobile ? '100%' : 720}
        title={t('Invoice application details')}
        onCancel={() => setDetail(null)}
      >
        {detail ? (
          <>
            <div className='mb-3 flex items-center justify-between gap-3'>
              <InvoiceStatusTag status={detail.status} t={t} />
              {detail.pdf_name ? (
                <Tag color='blue' shape='circle'>
                  {t('PDF uploaded')}
                </Tag>
              ) : (
                <Tag color='grey' shape='circle'>
                  {t('No PDF uploaded')}
                </Tag>
              )}
            </div>
            <InvoiceDetails application={detail} admin t={t} />
          </>
        ) : null}
      </SideSheet>

      <Modal
        visible={Boolean(rejectTarget)}
        title={t('Reject invoice application')}
        okText={t('Confirm rejection')}
        cancelText={t('Cancel')}
        okButtonProps={{ type: 'danger' }}
        confirmLoading={actionKey === `reject:${rejectTarget?.id}`}
        onOk={rejectApplication}
        onCancel={() => {
          if (!actionKey) {
            setRejectTarget(null);
            setRejectionReason('');
          }
        }}
      >
        <Text type='tertiary'>
          {t(
            'The reason is visible to the user. Rejected subscriptions become eligible for a new application.',
          )}
        </Text>
        <TextArea
          className='mt-3'
          aria-label={t('Rejection reason')}
          value={rejectionReason}
          onChange={(value) =>
            setRejectionReason(
              clampInvoiceText(value, REJECTION_REASON_MAX_LENGTH),
            )
          }
          placeholder={t('Enter a clear rejection reason')}
          autosize={{ minRows: 4, maxRows: 8 }}
          showClear
        />
        <div className='mt-1 text-right text-xs text-[var(--semi-color-text-2)]'>
          {Array.from(rejectionReason).length}/{REJECTION_REASON_MAX_LENGTH}
        </div>
      </Modal>
    </div>
  );
};

export default AdminInvoiceCenter;
