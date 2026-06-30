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
import { Button, Form, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  compareObjects,
  showError,
  showSuccess,
  showWarning,
  toBoolean,
} from '../../../helpers';

const defaultInputs = {
  LotteryEnabled: false,
  LotteryRequireRecharge: false,
  LotteryMinRechargeAmount: 0,
  LotteryRechargeWindowDays: 0,
  LotteryCountRedemptionAsRecharge: false,
  LotteryMinAccountAgeDays: 0,
  LotteryMinRequestCount: 0,
  LotteryRequireEmailVerified: false,
};

export default function SettingsLottery(props) {
  const { t } = useTranslation();
  const refForm = useRef();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(defaultInputs);
  const [inputsRow, setInputsRow] = useState(defaultInputs);

  function handleFieldChange(fieldName) {
    return (value) => {
      setInputs((current) => ({ ...current, [fieldName]: value }));
    };
  }

  function submitLotterySettings() {
    const updates = compareObjects(inputs, inputsRow);
    if (!updates.length) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }
    setLoading(true);
    Promise.all(
      updates.map((item) =>
        API.put('/api/option/', {
          key: item.key,
          value:
            typeof inputs[item.key] === 'boolean'
              ? String(inputs[item.key])
              : String(inputs[item.key] ?? ''),
        }),
      ),
    )
      .then((res) => {
        if (res.includes(undefined)) {
          showError(t('部分保存失败，请重试'));
          return;
        }
        showSuccess(t('保存成功'));
        props.refresh?.();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const nextInputs = { ...defaultInputs };
    Object.keys(defaultInputs).forEach((key) => {
      if (props.options?.[key] === undefined) return;
      nextInputs[key] =
        typeof defaultInputs[key] === 'boolean'
          ? toBoolean(props.options[key])
          : props.options[key];
    });
    setInputs(nextInputs);
    setInputsRow(nextInputs);
    refForm.current?.setValues(nextInputs);
  }, [props.options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formApi) => (refForm.current = formApi)}
        labelPosition='left'
        labelWidth={180}
      >
        <Form.Section
          text={t('抽奖设置')}
          extraText={t('配置抽奖功能是否启用，以及用户参与抽奖所需满足的通用条件')}
        >
          <Form.Switch
            field='LotteryEnabled'
            label={t('启用抽奖功能')}
            checkedText='｜'
            uncheckedText='〇'
            onChange={handleFieldChange('LotteryEnabled')}
          />
          <Form.Switch
            field='LotteryRequireRecharge'
            label={t('参与前必须完成充值')}
            checkedText='｜'
            uncheckedText='〇'
            onChange={handleFieldChange('LotteryRequireRecharge')}
          />
          <Form.InputNumber
            field='LotteryMinRechargeAmount'
            label={t('最低充值金额')}
            min={0}
            step={1}
            extraText={t('大于 0 时，用户需要至少有一笔不低于该金额的成功充值记录')}
            onChange={handleFieldChange('LotteryMinRechargeAmount')}
          />
          <Form.InputNumber
            field='LotteryRechargeWindowDays'
            label={t('充值有效期天数')}
            min={0}
            step={1}
            extraText={t('大于 0 时，只有在最近指定天数内完成的充值或兑换才会参与资格判断；0 表示不限制时间')}
            onChange={handleFieldChange('LotteryRechargeWindowDays')}
          />
          <Form.InputNumber
            field='LotteryMinAccountAgeDays'
            label={t('账号最小天数')}
            min={0}
            step={1}
            extraText={t('大于 0 时，新注册未满指定天数的用户不能参与')}
            onChange={handleFieldChange('LotteryMinAccountAgeDays')}
          />
          <Form.InputNumber
            field='LotteryMinRequestCount'
            label={t('最小请求次数')}
            min={0}
            step={1}
            extraText={t('大于 0 时，用户请求次数需要达到该阈值')}
            onChange={handleFieldChange('LotteryMinRequestCount')}
          />
          <Form.Switch
            field='LotteryCountRedemptionAsRecharge'
            label={t('兑换码兑换计入充值条件')}
            checkedText='｜'
            uncheckedText='〇'
            onChange={handleFieldChange('LotteryCountRedemptionAsRecharge')}
          />
          <Form.Switch
            field='LotteryRequireEmailVerified'
            label={t('参与前必须绑定邮箱')}
            checkedText='｜'
            uncheckedText='〇'
            onChange={handleFieldChange('LotteryRequireEmailVerified')}
          />
          <Button type='primary' onClick={submitLotterySettings}>
            {t('保存抽奖设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
