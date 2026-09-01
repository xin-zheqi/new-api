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
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Modal, Banner, Button, Typography, Spin } from '@douyinfe/semi-ui';
import { QRCodeSVG } from 'qrcode.react';
import { CheckCircle2, Clock3, Copy, XCircle } from 'lucide-react';
import { API, copy, showSuccess } from '../../../helpers';

const { Text } = Typography;

const isTerminalPaymentStatus = (status) =>
  status === 'success' || status === 'failed' || status === 'expired';

const WechatPayModal = ({ t, visible, payment, onCancel, onSuccess }) => {
  const [status, setStatus] = useState('pending');
  const statusRef = useRef('pending');
  const [checking, setChecking] = useState(false);
  const checkingRef = useRef(false);
  const successHandledRef = useRef(false);
  const onSuccessRef = useRef(onSuccess);
  const [remaining, setRemaining] = useState('');

  const updateStatus = useCallback((nextStatus) => {
    statusRef.current = nextStatus;
    setStatus(nextStatus);
  }, []);

  useEffect(() => {
    onSuccessRef.current = onSuccess;
  }, [onSuccess]);

  const checkOrder = useCallback(async () => {
    if (
      !visible ||
      !payment?.trade_no ||
      checkingRef.current ||
      isTerminalPaymentStatus(statusRef.current)
    ) {
      return;
    }
    checkingRef.current = true;
    setChecking(true);
    try {
      const res = await API.get(
        `/api/user/wechat/order/${encodeURIComponent(payment.trade_no)}`,
      );
      const data = res?.data?.data;
      if (!res?.data?.success || !data) {
        updateStatus('error');
      } else {
        switch (data.status) {
          case 'success':
            updateStatus('success');
            if (!successHandledRef.current) {
              successHandledRef.current = true;
              showSuccess(t('支付成功'));
              await onSuccessRef.current?.();
            }
            break;
          case 'failed':
            updateStatus('failed');
            break;
          case 'expired':
            updateStatus('expired');
            break;
          case 'pending':
            updateStatus('pending');
            break;
          default:
            updateStatus('error');
        }
      }
    } catch {
      updateStatus('error');
    } finally {
      checkingRef.current = false;
      setChecking(false);
    }
  }, [payment?.trade_no, t, updateStatus, visible]);

  useEffect(() => {
    if (!visible || !payment?.trade_no) return undefined;

    updateStatus('pending');
    successHandledRef.current = false;
    let cancelled = false;
    let timer;

    const poll = async () => {
      if (cancelled || isTerminalPaymentStatus(statusRef.current)) {
        return;
      }
      await checkOrder();
      if (!cancelled && !isTerminalPaymentStatus(statusRef.current)) {
        timer = window.setTimeout(poll, 3000);
      }
    };

    poll();
    return () => {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [checkOrder, payment?.trade_no, visible]);

  useEffect(() => {
    if (!visible || !payment?.expires_at) {
      setRemaining('');
      return undefined;
    }
    let timer;
    const updateRemaining = () => {
      const seconds = Math.max(
        0,
        Number(payment.expires_at) - Math.floor(Date.now() / 1000),
      );
      if (seconds === 0) {
        setRemaining('');
        if (!isTerminalPaymentStatus(statusRef.current)) void checkOrder();
        return;
      }
      setRemaining(
        `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`,
      );
      timer = window.setTimeout(updateRemaining, 1000);
    };
    updateRemaining();
    return () => {
      if (timer) window.clearTimeout(timer);
    };
  }, [checkOrder, payment?.expires_at, visible]);

  const handleCopy = async () => {
    if (!payment?.trade_no) return;
    await copy(payment.trade_no);
    showSuccess(t('订单号已复制'));
  };

  const renderStatus = () => {
    if (status === 'success') {
      return (
        <Banner
          type='success'
          icon={<CheckCircle2 size={16} />}
          description={t('支付成功')}
          closeIcon={null}
        />
      );
    }
    if (status === 'expired') {
      return (
        <Banner
          type='danger'
          icon={<XCircle size={16} />}
          description={t('支付订单已过期')}
          closeIcon={null}
        />
      );
    }
    if (status === 'failed') {
      return (
        <Banner
          type='danger'
          icon={<XCircle size={16} />}
          description={t('支付失败')}
          closeIcon={null}
        />
      );
    }
    if (status === 'error') {
      return (
        <Banner
          type='danger'
          icon={<XCircle size={16} />}
          description={t('暂时无法查询支付状态，请稍后重试')}
          closeIcon={null}
        />
      );
    }
    return (
      <Banner
        type='info'
        icon={<Clock3 size={16} />}
        description={t('请使用微信扫描二维码完成支付')}
        closeIcon={null}
      />
    );
  };

  return (
    <Modal
      title={t('微信支付')}
      visible={visible}
      onCancel={onCancel}
      footer={null}
      maskClosable={false}
      centered
      size='small'
    >
      <div className='flex flex-col items-center gap-4'>
        {status === 'pending' || status === 'error' ? (
          <div className='rounded-lg border bg-white p-4'>
            <QRCodeSVG
              value={payment?.code_url || ''}
              size={220}
              includeMargin
            />
          </div>
        ) : status === 'success' ? (
          <CheckCircle2 size={72} color='#16a34a' />
        ) : (
          <XCircle size={72} color='#dc2626' />
        )}
        {payment?.amount ? (
          <Text strong>
            {payment.amount} {payment.currency || 'CNY'}
          </Text>
        ) : null}
        {remaining && (status === 'pending' || status === 'error') ? (
          <Text type='tertiary'>
            {t('订单剩余时间')}：{remaining}
          </Text>
        ) : null}
        {renderStatus()}
        {payment?.trade_no ? (
          <div className='flex w-full items-center justify-between rounded-md border px-3 py-2'>
            <Text ellipsis={{ showTooltip: true }} type='tertiary'>
              {payment.trade_no}
            </Text>
            <Button
              theme='borderless'
              icon={<Copy size={15} />}
              aria-label={t('复制订单号')}
              onClick={handleCopy}
            />
          </div>
        ) : null}
        {status === 'pending' || status === 'error' ? (
          <Button
            theme='borderless'
            onClick={() => void checkOrder()}
            disabled={checking}
          >
            {checking ? <Spin size='small' /> : t('查询支付状态')}
          </Button>
        ) : null}
      </div>
    </Modal>
  );
};

export default WechatPayModal;
