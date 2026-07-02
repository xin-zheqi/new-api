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
          extraText={t('配置抽奖功能是否启用。参与条件在每个抽奖活动中单独设置')}
        >
          <Form.Switch
            field='LotteryEnabled'
            label={t('启用抽奖功能')}
            checkedText='｜'
            uncheckedText='〇'
            onChange={handleFieldChange('LotteryEnabled')}
          />
          <Button type='primary' onClick={submitLotterySettings}>
            {t('保存抽奖设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
