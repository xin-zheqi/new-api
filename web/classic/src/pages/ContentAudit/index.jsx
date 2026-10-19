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
import { useTranslation } from 'react-i18next';
import { Banner, Card, Tabs, TabPane, Typography } from '@douyinfe/semi-ui';
import { auditRequest } from './api';
import Rules from './Rules';
import Records from './Records';
export default function ContentAudit() {
  const { t } = useTranslation();
  const [data, setData] = useState(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let alive = true;
    auditRequest('/policy')
      .then((value) => {
        if (alive) setData(value);
      })
      .catch(() => {
        if (alive) setFailed(true);
      });
    return () => {
      alive = false;
    };
  }, []);
  if (!data)
    return (
      <p role='status' className='p-6'>
        {failed
          ? t('Content audit is unavailable or disabled by the administrator.')
          : t('Loading...')}
      </p>
    );
  return (
    <div className='mt-[60px] p-2 md:p-4'>
      <Card className='!rounded-2xl'>
        <div className='flex flex-col gap-4'>
          <Typography.Title heading={4}>{t('Content audit')}</Typography.Title>
          <Banner
            type='warning'
            description={t(
              'Your rules and hit details are private to your account and encrypted at rest. We will do our best to protect the security of your data. Do not send secrets to the network.',
            )}
          />
          <Typography.Text type='secondary'>
            {t(
              'Records expire after {{days}} days; cleanup runs every 10 minutes.',
              { days: data.retention_days },
            )}
          </Typography.Text>
          <Typography.Text type='secondary'>
            {t(
              'Audits text in Chat, Responses (HTTP/WebSocket), Messages, Gemini and supported text fields. Images, audio and file contents are not scanned. Realtime audio is unavailable while your audit policy is active.',
            )}
          </Typography.Text>
          <Tabs type='line' defaultActiveKey='rules' keepDOM={false}>
            <TabPane itemKey='rules' tab={t('My rules')}>
              <Rules
                data={data}
                onSaved={(policy) => setData({ ...data, policy })}
              />
            </TabPane>
            <TabPane itemKey='records' tab={t('Hit records')}>
              <Records />
            </TabPane>
          </Tabs>
        </div>
      </Card>
    </div>
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
