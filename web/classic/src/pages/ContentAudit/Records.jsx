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
import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  DatePicker,
  Input,
  Modal,
  Select,
  SideSheet,
  Space,
  Table,
  Tag,
} from '@douyinfe/semi-ui';
import { showError } from '../../helpers';
import { auditRequest } from './api';
import Matches from './Matches';
export default function Records() {
  const { t } = useTranslation();
  const [filters, setFilters] = useState({
    keyword: '',
    action: '',
    range: [],
  });
  const [page, setPage] = useState(1);
  const [query, setQuery] = useState('');
  const [rows, setRows] = useState({ items: [], total: 0 });
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState('');
  const [details, setDetails] = useState(null);
  const [detailError, setDetailError] = useState('');
  const [version, setVersion] = useState(0);
  useEffect(() => {
    let alive = true;
    setLoading(true);
    auditRequest('/records?' + query + '&page=' + page)
      .then((data) => {
        if (alive) setRows(data);
      })
      .catch((error) => {
        if (alive) showError(error.message);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [query, page, version]);
  useEffect(() => {
    let alive = true;
    setDetails(null);
    setDetailError('');
    if (selected)
      auditRequest('/records/' + selected)
        .then((data) => {
          if (alive) setDetails(data);
        })
        .catch((error) => {
          if (alive) setDetailError(error.message);
        });
    return () => {
      alive = false;
    };
  }, [selected]);
  const search = () => {
    const params = new URLSearchParams({
      keyword: filters.keyword,
      action: filters.action,
    });
    if (filters.range?.[0])
      params.set(
        'start_timestamp',
        String(Math.floor(new Date(filters.range[0]).getTime() / 1000)),
      );
    if (filters.range?.[1])
      params.set(
        'end_timestamp',
        String(Math.floor(new Date(filters.range[1]).getTime() / 1000)),
      );
    setPage(1);
    setQuery(params.toString());
    setVersion((value) => value + 1);
  };
  const remove = (id = '') =>
    Modal.confirm({
      title: t('Delete'),
      onOk: async () => {
        try {
          await auditRequest('/records' + (id ? '/' + id : ''), 'DELETE');
          setSelected('');
          setVersion((value) => value + 1);
        } catch (error) {
          showError(error.message);
          throw error;
        }
      },
    });
  const columns = [
    {
      title: t('Time'),
      dataIndex: 'created_at',
      render: (value) => new Date(value * 1000).toLocaleString(),
    },
    { title: t('API key name'), dataIndex: 'token_name', ellipsis: true },
    { title: t('Model'), dataIndex: 'model', ellipsis: true },
    {
      title: t('Action'),
      dataIndex: 'action',
      render: (value) => (
        <Tag color={value === 'block' ? 'red' : 'blue'}>{t(value)}</Tag>
      ),
    },
    { title: t('Matches'), dataIndex: 'match_count' },
    { title: t('Request ID'), dataIndex: 'request_id', ellipsis: true },
    {
      title: t('Actions'),
      render: (_, row) => (
        <Space>
          <Button onClick={() => setSelected(row.id)}>{t('Details')}</Button>
          <Button type='danger' onClick={() => remove(row.id)}>
            {t('Delete')}
          </Button>
        </Space>
      ),
    },
  ];
  return (
    <div className='flex flex-col gap-4'>
      <Space wrap>
        <Input
          aria-label={t('Request ID, key name or model')}
          placeholder={t('Request ID, key name or model')}
          maxLength={128}
          style={{ width: 200 }}
          value={filters.keyword}
          onChange={(keyword) => setFilters({ ...filters, keyword })}
        />
        <Select
          aria-label={t('Action')}
          value={filters.action}
          style={{ width: 120 }}
          onChange={(action) => setFilters({ ...filters, action })}
          optionList={[
            { value: '', label: t('All') },
            ...['warn', 'replace', 'block'].map((value) => ({
              value,
              label: t(value),
            })),
          ]}
        />
        <DatePicker
          type='dateTimeRange'
          value={filters.range}
          style={{ width: 280 }}
          onChange={(range) => setFilters({ ...filters, range: range || [] })}
        />
        <Button theme='solid' onClick={search}>
          {t('Search')}
        </Button>
        <Button type='danger' onClick={() => remove()}>
          {t('Delete all my records')}
        </Button>
      </Space>
      <Table
        rowKey='id'
        dataSource={rows.items}
        columns={columns}
        loading={loading}
        empty={t('No audit records')}
        scroll={{ x: 1000 }}
        pagination={{
          currentPage: page,
          pageSize: 20,
          total: rows.total,
          onPageChange: setPage,
        }}
      />
      <SideSheet
        title={t('Match details')}
        visible={!!selected}
        width={640}
        onCancel={() => setSelected('')}
        style={{ maxWidth: '100vw' }}
        keepDOM={false}
      >
        {details ? (
          <Matches result={details} />
        ) : (
          <p role='status'>{detailError || t('Loading...')}</p>
        )}
      </SideSheet>
    </div>
  );
}
/*
Copyright (C) 2023-2026 QuantumNous

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
