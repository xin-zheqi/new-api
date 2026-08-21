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
import {
  Badge,
  Button,
  Card,
  Empty,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../helpers';

const IdentityReviews = () => {
  const { t } = useTranslation();
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);

  const load = async () => {
    setLoading(true);
    try {
      const response = await API.get('/api/user/identity-reviews');
      if (!response.data?.success) throw new Error(response.data?.message);
      setUsers(response.data.data || []);
    } catch (error) {
      showError(error.message || t('Failed to load identity applications'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const review = async (id, action) => {
    try {
      const response = await API.post(
        `/api/user/identity-reviews/${id}/${action}`,
      );
      if (!response.data?.success) throw new Error(response.data?.message);
      showSuccess(
        action === 'approve'
          ? t('Identity approved')
          : t('Identity application rejected'),
      );
      load();
    } catch (error) {
      showError(error.message || t('Identity review failed'));
    }
  };

  const labels = { university: t('高校'), enterprise: t('企业') };
  return (
    <div className='w-full max-w-6xl mx-auto p-4 sm:p-6'>
      <Typography.Title heading={3} className='mb-4'>
        {t('Identity Review')}
      </Typography.Title>
      <Typography.Paragraph type='tertiary' className='mb-6'>
        {t('University and enterprise identity applications')}
      </Typography.Paragraph>
      {loading ? (
        <Spin />
      ) : users.length === 0 ? (
        <Empty description={t('No identity applications')} />
      ) : (
        users.map((user) => (
          <Card key={user.id} className='mb-3'>
            <div className='flex flex-wrap items-center gap-3'>
              <div className='flex-1 min-w-[180px]'>
                <Typography.Text strong>
                  {user.display_name || user.username}
                </Typography.Text>
                <div className='text-xs text-gray-500'>
                  {user.email || user.username}
                </div>
              </div>
              <Badge>
                {labels[user.identity_requested] || user.identity_requested}
              </Badge>
              <Button
                theme='solid'
                type='primary'
                onClick={() => review(user.id, 'approve')}
              >
                {t('Approve')}
              </Button>
              <Button type='danger' onClick={() => review(user.id, 'reject')}>
                {t('Reject')}
              </Button>
            </div>
          </Card>
        ))
      )}
    </div>
  );
};

export default IdentityReviews;
