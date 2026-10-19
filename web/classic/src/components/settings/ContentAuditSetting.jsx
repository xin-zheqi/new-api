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
import React, { useContext, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Card,
  InputNumber,
  Space,
  Spin,
  Switch,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../helpers';
import { StatusContext } from '../../context/Status';
export default function ContentAuditSetting() {
  const { t } = useTranslation();
  const [, dispatch] = useContext(StatusContext);
  const [values, setValues] = useState({
    enabled: false,
    retention_days: 7,
  });
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let alive = true;
    API.get('/api/option/')
      .then(({ data }) => {
        if (!data.success) throw new Error(data.message);
        const saved = data.data.find(
          (item) => item.key === 'ContentAuditSettings',
        );
        if (alive) {
          if (saved) {
            const parsed = JSON.parse(saved.value);
            setValues({
              enabled: Boolean(parsed.enabled),
              retention_days: Number(parsed.retention_days) || 7,
            });
          }
          setReady(true);
        }
      })
      .catch((error) => showError(error.message));
    return () => {
      alive = false;
    };
  }, []);
  const save = async () => {
    if (
      !Number.isInteger(values.retention_days) ||
      values.retention_days < 1 ||
      values.retention_days > 90
    ) {
      showError(t('Enter a whole number from 1 to 90.'));
      return;
    }
    setBusy(true);
    try {
      const { data } = await API.put('/api/option/', {
        key: 'ContentAuditSettings',
        value: JSON.stringify(values),
      });
      if (!data.success) throw new Error(data.message);
      const status = await API.get('/api/status');
      dispatch({ type: 'set', payload: status.data.data });
      showSuccess(t('Saved'));
    } catch (error) {
      showError(error.message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Spin spinning={!ready}>
      <Card title={t('Content audit')} className='!rounded-xl'>
        <div className='flex flex-col gap-4'>
          <Banner
            type='warning'
            description={t(
              'Before enabling content audit, configure a persistent CRYPTO_SECRET or SESSION_SECRET in the server environment (Docker environment variables or a Compose/Kubernetes Secret). Keep it stable across replicas and restarts; it is not stored on this page.',
            )}
          />
          <Space>
            <Switch
              aria-label={t('Enable content audit for users')}
              checked={values.enabled}
              onChange={(enabled) => setValues({ ...values, enabled })}
            />
            <span>{t('Enable content audit for users')}</span>
          </Space>
          <label className='flex flex-col gap-2'>
            {t('Hit record retention (days)')}
            <InputNumber
              aria-label={t('Hit record retention (days)')}
              min={1}
              max={90}
              value={values.retention_days}
              onChange={(retention_days) =>
                setValues({ ...values, retention_days })
              }
            />
          </label>
          <p>
            {t(
              '1–90 days. Expired records are hidden immediately and hard-deleted within 10 minutes. Database files and backups may retain historical bytes; this is not secure media erasure.',
            )}
          </p>
          <Button
            className='self-start'
            theme='solid'
            disabled={!ready}
            loading={busy}
            onClick={save}
          >
            {t('Save')}
          </Button>
        </div>
      </Card>
    </Spin>
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
