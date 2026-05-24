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
import React, { useEffect, useMemo, useState, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  downloadTextAsFile,
  showError,
  showSuccess,
  renderQuota,
  getCurrencyConfig,
} from '../../../../helpers';
import {
  quotaToDisplayAmount,
  displayAmountToQuota,
} from '../../../../helpers/quota';
import { normalizePlanRecords } from '../../../../helpers/subscription';
import { formatSubscriptionDuration } from '../../../../helpers/subscriptionFormat';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  Modal,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Form,
  Avatar,
  Row,
  Col,
  InputNumber,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconSave,
  IconClose,
  IconGift,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const EditRedemptionModal = (props) => {
  const { t } = useTranslation();
  const isEdit = props.editingRedemption.id !== undefined;
  const [loading, setLoading] = useState(isEdit);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [showQuotaInput, setShowQuotaInput] = useState(false);
  const [plans, setPlans] = useState([]);
  const [plansLoading, setPlansLoading] = useState(false);

  const getInitValues = () => ({
    name: '',
    redeem_type: 'quota',
    quota: 100000,
    amount: Number(quotaToDisplayAmount(100000).toFixed(6)),
    subscription_plan_id: undefined,
    count: 1,
    expired_time: null,
  });

  const handleCancel = () => {
    props.handleClose();
  };

  const loadPlans = async () => {
    setPlansLoading(true);
    try {
      const res = await API.get('/api/subscription/admin/plans');
      const { success, message, data } = res.data;
      if (success) {
        setPlans(normalizePlanRecords(data));
      } else {
        setPlans([]);
        showError(message);
      }
    } catch (e) {
      setPlans([]);
      showError(t('请求失败'));
    } finally {
      setPlansLoading(false);
    }
  };

  const loadRedemption = async () => {
    setLoading(true);
    let res = await API.get(`/api/redemption/${props.editingRedemption.id}`);
    const { success, message, data } = res.data;
    if (success) {
      if (data.expired_time === 0) {
        data.expired_time = null;
      } else {
        data.expired_time = new Date(data.expired_time * 1000);
      }
      data.redeem_type = data.redeem_type || 'quota';
      data.amount = Number(quotaToDisplayAmount(data.quota || 0).toFixed(6));
      formApiRef.current?.setValues({ ...getInitValues(), ...data });
    } else {
      showError(message);
    }
    setLoading(false);
  };

  useEffect(() => {
    if (!props.visiable) {
      return;
    }
    loadPlans();
    if (isEdit) {
      loadRedemption();
    } else {
      formApiRef.current?.setValues(getInitValues());
    }
  }, [props.visiable, props.editingRedemption.id, isEdit]);

  const planOptions = useMemo(() => {
    return plans.map((record) => {
      const plan = record?.plan || {};
      const title = (plan.title || '').trim() || t('订阅套餐');
      const subtitle = (plan.subtitle || '').trim();
      const price = `$${Number(plan.price_amount || 0).toFixed(2)}`;
      const duration = formatSubscriptionDuration(plan, t);
      const totalAmount = Number(plan.total_amount || 0);
      const quota = totalAmount > 0 ? renderQuota(totalAmount) : t('不限');
      const statusLabel = plan.enabled ? t('已启用') : t('已禁用');
      const meta = [
        `${t('总额度')}: ${quota}`,
        `${t('有效期')}: ${duration}`,
        price,
        statusLabel,
      ].join(' · ');
      return {
        value: plan.id,
        label: subtitle
          ? `ID ${plan.id} · ${title} · ${subtitle} · ${meta}`
          : `ID ${plan.id} · ${title} · ${meta}`,
      };
    });
  }, [plans, t]);

  const submit = async (values) => {
    let name = values.name;
    if (!isEdit && (!name || name === '')) {
      if (values.redeem_type === 'subscription') {
        const selectedPlan = plans.find(
          (item) => item?.plan?.id === values.subscription_plan_id,
        );
        name = selectedPlan?.plan?.title || t('订阅兑换码');
      } else {
        name = renderQuota(values.quota);
      }
    }
    if (!name || name === '') {
      name = renderQuota(values.quota);
    }
    setLoading(true);
    let localInputs = { ...values };
    localInputs.count = parseInt(localInputs.count) || 0;
    localInputs.redeem_type = localInputs.redeem_type || 'quota';
    if (localInputs.redeem_type === 'subscription') {
      if (!localInputs.subscription_plan_id) {
        showError(t('订阅兑换码必须绑定一个订阅套餐'));
        setLoading(false);
        return;
      }
      localInputs.quota = 0;
    } else {
      localInputs.quota = displayAmountToQuota(localInputs.amount);
      localInputs.subscription_plan_id = 0;
      if (localInputs.quota <= 0) {
        showError(t('请输入金额'));
        setLoading(false);
        return;
      }
    }
    localInputs.name = name;
    if (!localInputs.expired_time) {
      localInputs.expired_time = 0;
    } else {
      localInputs.expired_time = Math.floor(
        localInputs.expired_time.getTime() / 1000,
      );
    }
    let res;
    if (isEdit) {
      res = await API.put(`/api/redemption/`, {
        ...localInputs,
        id: parseInt(props.editingRedemption.id),
      });
    } else {
      res = await API.post(`/api/redemption/`, {
        ...localInputs,
      });
    }
    const { success, message, data } = res.data;
    if (success) {
      if (isEdit) {
        showSuccess(t('兑换码更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showSuccess(t('兑换码创建成功！'));
        props.refresh();
        formApiRef.current?.setValues(getInitValues());
        props.handleClose();
      }
    } else {
      showError(message);
    }
    if (!isEdit && data) {
      let text = '';
      for (let i = 0; i < data.length; i++) {
        text += data[i] + '\n';
      }
      Modal.confirm({
        title: t('兑换码创建成功'),
        content: (
          <div>
            <p>{t('兑换码创建成功，是否下载兑换码？')}</p>
            <p>{t('兑换码将以文本文件的形式下载，文件名为兑换码的名称。')}</p>
          </div>
        ),
        onOk: () => {
          downloadTextAsFile(text, `${localInputs.name}.txt`);
        },
      });
    }
    setLoading(false);
  };

  return (
    <>
      <SideSheet
        placement={isEdit ? 'right' : 'left'}
        title={
          <Space>
            {isEdit ? (
              <Tag color='blue' shape='circle'>
                {t('更新')}
              </Tag>
            ) : (
              <Tag color='green' shape='circle'>
                {t('新建')}
              </Tag>
            )}
            <Title heading={4} className='m-0'>
              {isEdit ? t('更新兑换码信息') : t('创建新的兑换码')}
            </Title>
          </Space>
        }
        bodyStyle={{ padding: '0' }}
        visible={props.visiable}
        width={isMobile ? '100%' : 600}
        footer={
          <div className='flex justify-end bg-white'>
            <Space>
              <Button
                theme='solid'
                onClick={() => formApiRef.current?.submitForm()}
                icon={<IconSave />}
                loading={loading}
              >
                {t('提交')}
              </Button>
              <Button
                theme='light'
                type='primary'
                onClick={handleCancel}
                icon={<IconClose />}
              >
                {t('取消')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={() => handleCancel()}
      >
        <Spin spinning={loading}>
          <Form
            initValues={getInitValues()}
            getFormApi={(api) => (formApiRef.current = api)}
            onSubmit={submit}
          >
            {({ values }) => (
              <div className='p-2'>
                <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
                  {/* Header: Basic Info */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='blue'
                      className='mr-2 shadow-md'
                    >
                      <IconGift size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {t('基本信息')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {t('设置兑换码的基本信息')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    <Col span={24}>
                      <Form.Input
                        field='name'
                        label={t('名称')}
                        placeholder={t('请输入名称')}
                        style={{ width: '100%' }}
                        rules={
                          !isEdit
                            ? []
                            : [{ required: true, message: t('请输入名称') }]
                        }
                        showClear
                      />
                    </Col>
                    <Col span={24}>
                      <Form.DatePicker
                        field='expired_time'
                        label={t('过期时间')}
                        type='dateTime'
                        placeholder={t('选择过期时间（可选，留空为永久）')}
                        style={{ width: '100%' }}
                        showClear
                      />
                    </Col>
                    <Col span={24}>
                      <Form.Select
                        field='redeem_type'
                        label={t('兑换类型')}
                        placeholder={t('请选择')}
                        style={{ width: '100%' }}
                      >
                        <Form.Select.Option value='quota'>
                          {t('额度充值码')}
                        </Form.Select.Option>
                        <Form.Select.Option value='subscription'>
                          {t('订阅开通码')}
                        </Form.Select.Option>
                      </Form.Select>
                    </Col>
                  </Row>
                </Card>

                <Card className='!rounded-2xl shadow-sm border-0'>
                  {/* Header: Benefit Settings */}
                  <div className='flex items-center mb-2'>
                    <Avatar
                      size='small'
                      color='green'
                      className='mr-2 shadow-md'
                    >
                      <IconCreditCard size={16} />
                    </Avatar>
                    <div>
                      <Text className='text-lg font-medium'>
                        {values.redeem_type === 'subscription'
                          ? t('订阅套餐')
                          : t('额度设置')}
                      </Text>
                      <div className='text-xs text-gray-600'>
                        {values.redeem_type === 'subscription'
                          ? t('用户兑换后将开通所选订阅套餐')
                          : t('设置兑换码的额度和数量')}
                      </div>
                    </div>
                  </div>

                  <Row gutter={12}>
                    {values.redeem_type === 'subscription' ? (
                      <Col span={24}>
                        <Form.Select
                          field='subscription_plan_id'
                          label={t('订阅套餐')}
                          placeholder={
                            plansLoading
                              ? t('加载中...')
                              : plans.length === 0
                                ? t('暂无订阅套餐')
                                : t('请选择订阅套餐')
                          }
                          optionList={planOptions}
                          disabled={plansLoading || plans.length === 0}
                          filter
                          style={{ width: '100%' }}
                        />
                        <div className='text-xs text-gray-500 mt-1'>
                          {plansLoading
                            ? t('加载中...')
                            : plans.length === 0
                              ? t('暂无订阅套餐')
                              : t('用户兑换后将开通所选订阅套餐')}
                        </div>
                      </Col>
                    ) : (
                      <Col span={24}>
                        <Form.InputNumber
                          field='amount'
                          label={t('金额')}
                          prefix={getCurrencyConfig().symbol}
                          placeholder={t('输入金额')}
                          precision={6}
                          min={0}
                          step={0.000001}
                          style={{ width: '100%' }}
                          onChange={(val) => {
                            const amount = val === '' || val == null ? 0 : val;
                            formApiRef.current?.setValue('amount', amount);
                            formApiRef.current?.setValue(
                              'quota',
                              displayAmountToQuota(amount),
                            );
                          }}
                          showClear
                        />
                        <div
                          className='text-xs cursor-pointer mt-1'
                          style={{ color: 'var(--semi-color-text-2)' }}
                          onClick={() => setShowQuotaInput((v) => !v)}
                        >
                          {showQuotaInput
                            ? `▾ ${t('收起原生额度输入')}`
                            : `▸ ${t('使用原生额度输入')}`}
                        </div>
                        <div
                          style={{ display: showQuotaInput ? 'block' : 'none' }}
                          className='mt-2'
                        >
                          <Form.InputNumber
                            field='quota'
                            label={t('额度')}
                            placeholder={t('输入额度')}
                            rules={[
                              { required: true, message: t('请输入额度') },
                              {
                                validator: (rule, v) => {
                                  const num = parseInt(v, 10);
                                  return num > 0
                                    ? Promise.resolve()
                                    : Promise.reject(t('额度必须大于0'));
                                },
                              },
                            ]}
                            onChange={(val) => {
                              const quota = val === '' || val == null ? 0 : val;
                              formApiRef.current?.setValue('quota', quota);
                              formApiRef.current?.setValue(
                                'amount',
                                Number(quotaToDisplayAmount(quota).toFixed(6)),
                              );
                            }}
                            style={{ width: '100%' }}
                            showClear
                          />
                        </div>
                      </Col>
                    )}
                    {!isEdit && (
                      <Col span={12}>
                        <Form.InputNumber
                          field='count'
                          label={t('生成数量')}
                          min={1}
                          rules={[
                            { required: true, message: t('请输入生成数量') },
                            {
                              validator: (rule, v) => {
                                const num = parseInt(v, 10);
                                return num > 0
                                  ? Promise.resolve()
                                  : Promise.reject(t('生成数量必须大于0'));
                              },
                            },
                          ]}
                          style={{ width: '100%' }}
                          showClear
                        />
                      </Col>
                    )}
                  </Row>
                </Card>
              </div>
            )}
          </Form>
        </Spin>
      </SideSheet>
    </>
  );
};

export default EditRedemptionModal;
