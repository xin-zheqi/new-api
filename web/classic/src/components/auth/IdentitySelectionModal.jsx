import React, { useContext, useState } from 'react';
import { Button, Modal, Space, Typography } from '@douyinfe/semi-ui';
import { Building2, GraduationCap, UserRound, UsersRound } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';
import { UserContext } from '../../context/User';

const identities = [
  { value: 'personal', label: '个人' },
  { value: 'student', label: '学生' },
  { value: 'university', label: '高校' },
  { value: 'enterprise', label: '企业' },
];
const identityIcons = { personal: UserRound, student: GraduationCap, university: Building2, enterprise: UsersRound };

const IdentitySelectionModal = () => {
  const { t } = useTranslation();
  const [userState, userDispatch] = useContext(UserContext);
  const [saving, setSaving] = useState(false);
  const user = userState?.user;
  const hasPendingIdentityReview =
    user?.identity_review_status === 'pending' ||
    user?.identity_requested === 'university' ||
    user?.identity_requested === 'enterprise';
  const shouldSelectIdentity = Boolean(user && !user.identity && !hasPendingIdentityReview);

  const chooseIdentity = async (identity) => {
    if (!user) return;
    setSaving(true);
    try {
      const response = await API.put('/api/user/self', { identity });
      if (!response.data?.success) {
        showError(response.data?.message || t('身份保存失败'));
        return;
      }
      const updatedUser = identity === 'university' || identity === 'enterprise'
        ? { ...user, identity: 'personal', identity_requested: identity, identity_review_status: 'pending' }
        : { ...user, identity };
      localStorage.setItem('user', JSON.stringify(updatedUser));
      userDispatch({ type: 'login', payload: updatedUser });
    } catch {
      showError(t('身份保存失败'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title={t('请选择您的身份')}
      visible={shouldSelectIdentity}
      footer={null}
      closable={false}
      maskClosable={false}
      onCancel={() => {}}
    >
      <Typography.Paragraph type='tertiary'>
        {t('选择身份后即可完成账户设置，提交后不可跳过。')}
      </Typography.Paragraph>
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
        {identities.map((identity) => (
          <Button
            key={identity.value}
            block
            theme='light'
            type='tertiary'
            className='!h-auto !justify-start !px-4 !py-4 text-left'
            loading={saving}
            onClick={() => chooseIdentity(identity.value)}
          >
            {React.createElement(identityIcons[identity.value], { size: 20, className: 'mr-3 text-blue-500' })}
            {t(identity.label)}
          </Button>
        ))}
      </div>
    </Modal>
  );
};

export default IdentitySelectionModal;
