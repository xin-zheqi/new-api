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

import React, { useContext, useEffect, useState, useRef } from 'react';
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  parseHttpStatusCodeRules,
  setStatusData,
  toBoolean,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';
import HttpStatusCodeRulesInput from '../../../components/settings/HttpStatusCodeRulesInput';
import { StatusContext } from '../../../context/Status';

const MONITORING_DEFAULTS = {
  ChannelDisableThreshold: '',
  QuotaRemindThreshold: '',
  QuotaRemindEnabled: true,
  AutomaticDisableChannelEnabled: false,
  AutomaticEnableChannelEnabled: false,
  AutomaticDisableKeywords: '',
  AutomaticDisableStatusCodes: '401',
  AutomaticRetryStatusCodes: '100-199,300-399,401-407,409-499,500-599',
  'monitor_setting.auto_test_channel_enabled': false,
  'monitor_setting.auto_test_only_auto_disabled': false,
  'monitor_setting.channel_test_mode': 'scheduled_all',
  'monitor_setting.channel_test_concurrency': 1,
  'monitor_setting.auto_test_channel_minutes': 10,
  'monitor_setting.auto_test_channel_time_range': '00:00-23:59',
};

export default function SettingsMonitoring(props) {
  const { t } = useTranslation();
  const [, statusDispatch] = useContext(StatusContext);
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({ ...MONITORING_DEFAULTS });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const parsedAutoDisableStatusCodes = parseHttpStatusCodeRules(
    inputs.AutomaticDisableStatusCodes || '',
  );
  const parsedAutoRetryStatusCodes = parseHttpStatusCodeRules(
    inputs.AutomaticRetryStatusCodes || '',
  );
  const autoTestTimeRangePattern =
    /^([01]?\d|2[0-3]):[0-5]\d-([01]?\d|2[0-3]):[0-5]\d$/;

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const changedKeys = new Set(updateArray.map(({ key }) => key));
    if (
      changedKeys.has('AutomaticDisableStatusCodes') &&
      !parsedAutoDisableStatusCodes.ok
    ) {
      const details =
        parsedAutoDisableStatusCodes.invalidTokens &&
        parsedAutoDisableStatusCodes.invalidTokens.length > 0
          ? `: ${parsedAutoDisableStatusCodes.invalidTokens.join(', ')}`
          : '';
      return showError(`${t('自动禁用状态码格式不正确')}${details}`);
    }
    if (
      changedKeys.has('AutomaticRetryStatusCodes') &&
      !parsedAutoRetryStatusCodes.ok
    ) {
      const details =
        parsedAutoRetryStatusCodes.invalidTokens &&
        parsedAutoRetryStatusCodes.invalidTokens.length > 0
          ? `: ${parsedAutoRetryStatusCodes.invalidTokens.join(', ')}`
          : '';
      return showError(`${t('自动重试状态码格式不正确')}${details}`);
    }
    if (
      changedKeys.has('monitor_setting.auto_test_channel_time_range') &&
      !autoTestTimeRangePattern.test(
        inputs['monitor_setting.auto_test_channel_time_range'] || '',
      )
    ) {
      return showError(t('定时测试时间范围格式不正确'));
    }
    const policyOptions = {};
    const requestQueue = [];
    const normalizedMap = {
      AutomaticDisableStatusCodes: parsedAutoDisableStatusCodes.normalized,
      AutomaticRetryStatusCodes: parsedAutoRetryStatusCodes.normalized,
    };
    for (const { key } of updateArray) {
      const value = String(normalizedMap[key] ?? inputs[key] ?? '');
      if (key === 'QuotaRemindEnabled' || key === 'QuotaRemindThreshold') {
        requestQueue.push(API.put('/api/option/', { key, value }));
      } else {
        policyOptions[key] = value;
      }
    }
    // Keep the original form while retaining the backend's atomic policy save.
    // The mode and its legacy alias must never be published independently.
    if (changedKeys.has('monitor_setting.channel_test_mode')) {
      policyOptions['monitor_setting.auto_test_only_auto_disabled'] = String(
        inputs['monitor_setting.channel_test_mode'] === 'passive_recovery',
      );
    }
    if (Object.keys(policyOptions).length) {
      requestQueue.push(
        API.patch('/api/option/request_policy', { options: policyOptions }),
      );
    }
    setLoading(true);
    Promise.all(requestQueue)
      .then(async (res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }
        const failed = res.find((response) => !response?.data?.success);
        if (failed) {
          return showError(failed.data?.message || t('部分保存失败，请重试'));
        }
        if (
          updateArray.some(
            (item) =>
              item.key === 'QuotaRemindEnabled' ||
              item.key === 'QuotaRemindThreshold',
          )
        ) {
          const status = await API.get('/api/status');
          if (status.data.success) {
            statusDispatch({ type: 'set', payload: status.data.data });
            setStatusData(status.data.data);
          }
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = { ...MONITORING_DEFAULTS };
    for (const key of Object.keys(MONITORING_DEFAULTS)) {
      if (props.options?.[key] === undefined) continue;
      currentInputs[key] =
        typeof MONITORING_DEFAULTS[key] === 'boolean'
          ? toBoolean(props.options[key])
          : props.options[key];
    }
    let channelTestMode = 'scheduled_all';
    if (
      currentInputs['monitor_setting.channel_test_mode'] ===
        'passive_recovery' ||
      currentInputs['monitor_setting.auto_test_only_auto_disabled'] === true
    ) {
      channelTestMode = 'passive_recovery';
    } else if (
      currentInputs['monitor_setting.channel_test_mode'] === 'auto_ban_only'
    ) {
      channelTestMode = 'auto_ban_only';
    }
    currentInputs['monitor_setting.channel_test_mode'] = channelTestMode;
    currentInputs['monitor_setting.auto_test_only_auto_disabled'] =
      channelTestMode === 'passive_recovery';
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('监控设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'monitor_setting.auto_test_channel_enabled'}
                  label={t('定时测试所有通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.auto_test_channel_enabled': value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('自动测试所有通道间隔时间')}
                  step={0.1}
                  min={0}
                  suffix={t('分钟')}
                  extraText={t('每隔多少分钟测试一次所有通道')}
                  placeholder={''}
                  field={'monitor_setting.auto_test_channel_minutes'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.auto_test_channel_minutes': value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Input
                  label={t('定时测试时间范围')}
                  placeholder={'08:00-23:59'}
                  extraText={t('仅在服务器本地时间处于该范围内时执行定时测试')}
                  field={'monitor_setting.auto_test_channel_time_range'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'monitor_setting.auto_test_channel_time_range': value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Select
                  field='monitor_setting.channel_test_mode'
                  label={t('Channel test mode')}
                  style={{ width: '100%' }}
                  optionList={[
                    { value: 'scheduled_all', label: t('Scheduled testing') },
                    {
                      value: 'auto_ban_only',
                      label: t('Test automatically disabled channels'),
                    },
                    { value: 'passive_recovery', label: t('Passive recovery') },
                  ]}
                  onChange={(value) =>
                    setInputs((current) => ({
                      ...current,
                      'monitor_setting.auto_test_only_auto_disabled':
                        value === 'passive_recovery',
                      'monitor_setting.channel_test_mode': value,
                    }))
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field='monitor_setting.channel_test_concurrency'
                  label={t('Channel test concurrency')}
                  min={1}
                  max={32}
                  step={1}
                  precision={0}
                  onChange={(value) =>
                    setInputs((current) => ({
                      ...current,
                      'monitor_setting.channel_test_concurrency': value,
                    }))
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('测试所有渠道的最长响应时间')}
                  step={0.1}
                  min={0}
                  suffix={t('秒')}
                  extraText={t(
                    '当运行通道全部测试时，超过此时间将自动禁用通道',
                  )}
                  placeholder={''}
                  field={'ChannelDisableThreshold'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ChannelDisableThreshold: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field='QuotaRemindEnabled'
                  label={t('Site-wide low balance reminders')}
                  aria-label={t('Site-wide low balance reminders')}
                  extraText={t(
                    'Notification switches do not affect verification codes or account security emails.',
                  )}
                  onChange={(value) =>
                    setInputs((current) => ({
                      ...current,
                      QuotaRemindEnabled: value,
                    }))
                  }
                />
                <Form.InputNumber
                  label={t('额度提醒阈值')}
                  step={1}
                  min={0}
                  suffix={'Token'}
                  extraText={t(
                    'Send a reminder when the wallet or subscription balance crosses below this threshold.',
                  )}
                  placeholder={''}
                  field={'QuotaRemindThreshold'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      QuotaRemindThreshold: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'AutomaticDisableChannelEnabled'}
                  label={t('失败时自动禁用通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      AutomaticDisableChannelEnabled: value,
                    });
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'AutomaticEnableChannelEnabled'}
                  label={t('成功时自动启用通道')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      AutomaticEnableChannelEnabled: value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <HttpStatusCodeRulesInput
                  label={t('自动禁用状态码')}
                  placeholder={t('例如：401, 403, 429, 500-599')}
                  extraText={t(
                    '支持填写单个状态码或范围（含首尾），使用逗号分隔',
                  )}
                  field={'AutomaticDisableStatusCodes'}
                  onChange={(value) =>
                    setInputs({ ...inputs, AutomaticDisableStatusCodes: value })
                  }
                  parsed={parsedAutoDisableStatusCodes}
                  invalidText={t('自动禁用状态码格式不正确')}
                />
                <HttpStatusCodeRulesInput
                  label={t('自动重试状态码')}
                  placeholder={t('例如：401, 403, 429, 500-599')}
                  extraText={t(
                    '支持填写单个状态码或范围（含首尾），使用逗号分隔',
                  )}
                  field={'AutomaticRetryStatusCodes'}
                  onChange={(value) =>
                    setInputs({ ...inputs, AutomaticRetryStatusCodes: value })
                  }
                  parsed={parsedAutoRetryStatusCodes}
                  invalidText={t('自动重试状态码格式不正确')}
                />
                <Form.TextArea
                  label={t('自动禁用关键词')}
                  placeholder={t('一行一个，不区分大小写')}
                  extraText={t(
                    '当上游通道返回错误中包含这些关键词时（不区分大小写），自动禁用通道',
                  )}
                  field={'AutomaticDisableKeywords'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  onChange={(value) =>
                    setInputs({ ...inputs, AutomaticDisableKeywords: value })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Button
                size='default'
                onClick={onSubmit}
                loading={loading}
                disabled={loading}
              >
                {t('保存监控设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
