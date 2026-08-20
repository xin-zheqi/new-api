import React, { useEffect, useState } from 'react';
import { Badge, Button, Card, Empty, Spin, Typography } from '@douyinfe/semi-ui';
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
      showError(error.message || t('加载身份审核失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const review = async (id, action) => {
    try {
      const response = await API.post(`/api/user/identity-reviews/${id}/${action}`);
      if (!response.data?.success) throw new Error(response.data?.message);
      showSuccess(action === 'approve' ? t('身份审核已通过') : t('身份申请已拒绝'));
      load();
    } catch (error) {
      showError(error.message || t('身份审核失败'));
    }
  };

  const labels = { university: t('高校'), enterprise: t('企业') };
  return (
    <div className='w-full max-w-6xl mx-auto p-4 sm:p-6'>
      <Typography.Title heading={3}>{t('身份审核')}</Typography.Title>
      <Typography.Paragraph type='tertiary'>{t('审核高校和企业用户的身份申请')}</Typography.Paragraph>
      {loading ? <Spin /> : users.length === 0 ? <Empty description={t('暂无身份申请')} /> : users.map((user) => (
        <Card key={user.id} className='mb-3'>
          <div className='flex flex-wrap items-center gap-3'>
            <div className='flex-1 min-w-[180px]'><Typography.Text strong>{user.display_name || user.username}</Typography.Text><div className='text-xs text-gray-500'>{user.email || user.username}</div></div>
            <Badge>{labels[user.identity_requested] || user.identity_requested}</Badge>
            <Button theme='solid' type='primary' onClick={() => review(user.id, 'approve')}>{t('通过')}</Button>
            <Button type='danger' onClick={() => review(user.id, 'reject')}>{t('拒绝')}</Button>
          </div>
        </Card>
      ))}
    </div>
  );
};

export default IdentityReviews;
