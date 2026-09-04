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
import { Banner, Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import { ShieldCheck } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, toBoolean } from '../../../helpers';

const defaultInputs = {
  WechatEnabled: false,
  WechatMchID: '',
  WechatAppID: '',
  WechatApiV3Key: '',
  WechatCertificateSerialNumber: '',
  WechatPrivateKey: '',
  WechatMerchantCertificate: '',
  WechatPayPublicKeyID: '',
  WechatPayPublicKey: '',
  WechatPlatformCertificate: '',
  WechatNotifyURL: '',
  WechatOrderExpireMinutes: 120,
  WechatUnitPrice: 7.3,
};

export default function SettingsPaymentGatewayWechat(props) {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState(defaultInputs);
  const [loading, setLoading] = useState(false);
  const formApiRef = useRef(null);
  const configuredCallbackBase = String(
    props.options?.CustomCallbackAddress || props.options?.ServerAddress || '',
  )
    .trim()
    .replace(/\/+$/, '');
  const callbackBase = configuredCallbackBase.startsWith('https://')
    ? configuredCallbackBase
    : 'https://your-domain.example';
  const defaultNotifyUrl = `${callbackBase}/api/wechat/notify`;

  useEffect(() => {
    if (!props.options) return;
    const values = {
      ...defaultInputs,
      WechatEnabled: toBoolean(props.options.WechatEnabled),
      WechatMchID: props.options.WechatMchID || '',
      WechatAppID: props.options.WechatAppID || '',
      WechatCertificateSerialNumber:
        props.options.WechatCertificateSerialNumber || '',
      WechatPayPublicKeyID: props.options.WechatPayPublicKeyID || '',
      WechatNotifyURL: props.options.WechatNotifyURL || '',
      WechatOrderExpireMinutes:
        parseInt(props.options.WechatOrderExpireMinutes) || 120,
      WechatUnitPrice:
        Number.isFinite(parseFloat(props.options.WechatUnitPrice)) &&
        parseFloat(props.options.WechatUnitPrice) > 0
          ? parseFloat(props.options.WechatUnitPrice)
          : 7.3,
    };
    setInputs(values);
    formApiRef.current?.setValues(values);
  }, [props.options]);

  const saveWechatSettings = async () => {
    const merchantId = String(inputs.WechatMchID || '').trim();
    const appId = String(inputs.WechatAppID || '').trim();
    const certificateSerial = String(
      inputs.WechatCertificateSerialNumber || '',
    ).trim();
    const publicKeyId = String(inputs.WechatPayPublicKeyID || '').trim();
    const apiV3Key = String(inputs.WechatApiV3Key || '').trim();
    const notifyUrl = String(inputs.WechatNotifyURL || '').trim();
    const expiryMinutes = Number(inputs.WechatOrderExpireMinutes);
    const unitPrice = Number(inputs.WechatUnitPrice);

    if (!Number.isFinite(unitPrice) || unitPrice <= 0 || unitPrice > 1000000) {
      showError(t('请输入有效的数字'));
      return;
    }

    if (inputs.WechatEnabled) {
      if (
        (merchantId && !/^\d{8,32}$/.test(merchantId)) ||
        (appId && !/^wx[A-Za-z0-9]{16}$/.test(appId)) ||
        (certificateSerial &&
          (!/^[A-Fa-f0-9]+$/.test(certificateSerial) ||
            certificateSerial.length > 64)) ||
        (publicKeyId && !/^PUB_KEY_ID_\d{1,53}$/.test(publicKeyId))
      ) {
        showError(t('微信支付商户信息格式不正确'));
        return;
      }
      if (!merchantId || !appId || !certificateSerial) {
        showError(t('启用微信支付前请填写商户号、AppID 和商户证书序列号'));
        return;
      }
      if (apiV3Key && new TextEncoder().encode(apiV3Key).length !== 32) {
        showError(t('API v3 密钥必须正好为 32 字节'));
        return;
      }
      if (notifyUrl) {
        try {
          const parsed = new URL(notifyUrl);
          if (
            parsed.protocol !== 'https:' ||
            parsed.username ||
            parsed.password ||
            parsed.search ||
            parsed.hash ||
            new TextEncoder().encode(notifyUrl).length > 255
          ) {
            throw new Error('invalid notification URL');
          }
        } catch {
          showError(
            t('支付通知地址必须是不含凭据、查询参数或片段的 HTTPS 地址'),
          );
          return;
        }
      }
    }
    if (
      !Number.isInteger(expiryMinutes) ||
      expiryMinutes < 1 ||
      expiryMinutes > 120
    ) {
      showError(t('订单有效期必须是 1 到 120 分钟之间的整数'));
      return;
    }

    setLoading(true);
    try {
      const response = await API.put('/api/option/wechat', {
        enabled: Boolean(inputs.WechatEnabled),
        mch_id: merchantId,
        app_id: appId,
        api_v3_key: apiV3Key,
        certificate_serial_number: certificateSerial,
        private_key: String(inputs.WechatPrivateKey || '').trim(),
        merchant_certificate: String(
          inputs.WechatMerchantCertificate || '',
        ).trim(),
        pay_public_key_id: publicKeyId,
        pay_public_key: String(inputs.WechatPayPublicKey || '').trim(),
        platform_certificate: String(
          inputs.WechatPlatformCertificate || '',
        ).trim(),
        notify_url: notifyUrl.replace(/\/+$/, ''),
        order_expire_minutes: expiryMinutes,
        unit_price: unitPrice,
      });
      if (!response.data?.success) {
        throw new Error(response.data?.message || t('更新失败'));
      }
      showSuccess(t('微信支付设置已更新'));
      await props.refresh?.();
    } catch (error) {
      showError(error instanceof Error ? error.message : t('更新失败'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={setInputs}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section
          text={props.hideSectionTitle ? undefined : t('微信支付设置')}
        >
          <Banner
            type='info'
            icon={<ShieldCheck size={16} />}
            description={
              <div className='flex min-w-0 flex-col gap-2'>
                <div className='font-medium leading-5'>
                  {t('微信支付 API v3 扫码支付')}
                </div>
                <div className='break-words leading-5'>
                  {t(
                    '请填写同一微信支付商户下签发的商户凭据。敏感字段保存后不会回显，留空表示保持原值。',
                  )}
                </div>
              </div>
            }
            closeIcon={null}
            style={{ marginBottom: 16 }}
          />

          <Row gutter={16}>
            <Col xs={24} md={6}>
              <Form.Switch
                field='WechatEnabled'
                label={t('启用微信支付')}
                extraText={t('在余额充值和订阅购买中展示微信扫码支付')}
              />
            </Col>
            <Col xs={24} md={6}>
              <Form.Input
                field='WechatMchID'
                label={t('微信支付商户号')}
                placeholder='mchid'
              />
            </Col>
            <Col xs={24} md={6}>
              <Form.Input
                field='WechatAppID'
                label='AppID'
                placeholder='wx...'
              />
            </Col>
            <Col xs={24} md={6}>
              <Form.InputNumber
                field='WechatOrderExpireMinutes'
                label={t('订单有效期（分钟）')}
                min={1}
                max={120}
                precision={0}
              />
            </Col>
          </Row>

          <Row gutter={16} style={{ marginTop: 16 }}>
            <Col xs={24} md={8}>
              <Form.Input
                field='WechatCertificateSerialNumber'
                label={t('商户 API 证书序列号')}
                placeholder={t('大写十六进制序列号')}
              />
            </Col>
            <Col xs={24} md={8}>
              <Form.Input
                field='WechatApiV3Key'
                type='password'
                label={t('API v3 密钥')}
                placeholder={t('填写新值以替换，留空保持不变')}
                extraText={t('必须为 32 字节')}
              />
            </Col>
            <Col xs={24} md={8}>
              <Form.Input
                field='WechatPayPublicKeyID'
                label={t('微信支付公钥 ID')}
                placeholder='PUB_KEY_ID_...'
                extraText={t('使用微信支付公钥验签时填写')}
              />
            </Col>
          </Row>

          <Row gutter={16} style={{ marginTop: 16 }}>
            <Col xs={24} md={8}>
              <Form.InputNumber
                field='WechatUnitPrice'
                label={t('充值价格（x元/美金）')}
                min={0.000001}
                max={1000000}
                step={0.01}
                precision={6}
                extraText={t('例如：7，就是7元/美金')}
              />
            </Col>
          </Row>

          <Row gutter={16} style={{ marginTop: 16 }}>
            <Col xs={24} lg={8}>
              <Form.TextArea
                field='WechatPrivateKey'
                label={t('商户 RSA 私钥')}
                rows={8}
                placeholder='-----BEGIN PRIVATE KEY-----'
                extraText={t('支持 PKCS#8 或 PKCS#1，留空保持不变')}
              />
            </Col>
            <Col xs={24} lg={8}>
              <Form.TextArea
                field='WechatPayPublicKey'
                label={t('微信支付公钥')}
                rows={8}
                placeholder='-----BEGIN PUBLIC KEY-----'
                extraText={t('推荐用于微信响应和支付通知验签')}
              />
            </Col>
            <Col xs={24} lg={8}>
              <Form.TextArea
                field='WechatPlatformCertificate'
                label={t('微信支付平台证书')}
                rows={8}
                placeholder='-----BEGIN CERTIFICATE-----'
                extraText={t('未使用微信支付公钥时的兼容验签方式')}
              />
            </Col>
          </Row>

          <Row gutter={16} style={{ marginTop: 16 }}>
            <Col xs={24} md={12}>
              <Form.TextArea
                field='WechatMerchantCertificate'
                label={t('商户 API 证书（可选）')}
                rows={5}
                placeholder='-----BEGIN CERTIFICATE-----'
                extraText={t('用于校验商户私钥和证书序列号是否匹配')}
              />
            </Col>
            <Col xs={24} md={12}>
              <Form.Input
                field='WechatNotifyURL'
                label={t('支付通知地址')}
                placeholder={defaultNotifyUrl}
                extraText={t(
                  '必须为不带查询参数的 HTTPS 地址；留空使用系统回调域名。',
                )}
              />
            </Col>
          </Row>

          <Button onClick={saveWechatSettings} style={{ marginTop: 16 }}>
            {t('更新微信支付设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
