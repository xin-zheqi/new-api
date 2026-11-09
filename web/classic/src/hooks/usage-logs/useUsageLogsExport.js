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

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { exportUsageLogsCsv } from '../../../../shared/usage-log-export';
import { API } from '../../helpers/api';
import { showError, showInfo, timestamp2string } from '../../helpers/utils';

export function useUsageLogsExport(appliedLogQuery) {
  const { t } = useTranslation();
  const [exportingLogs, setExportingLogs] = useState(false);
  const exportController = useRef(null);

  useEffect(
    () => () => {
      exportController.current?.abort();
      exportController.current = null;
    },
    [],
  );

  const exportLogs = async () => {
    if (!appliedLogQuery.current || exportController.current) return;

    const controller = new AbortController();
    exportController.current = controller;
    setExportingLogs(true);

    try {
      // Snapshot query, currency and labels once for the entire download.
      const { path, params } = appliedLogQuery.current;
      const query = new URLSearchParams(params);
      const now = Math.floor(Date.now() / 1000);
      const end = Number(query.get('end_timestamp'));
      query.set('end_timestamp', String(end > 0 ? Math.min(end, now) : now));

      let status = {};
      try {
        status = JSON.parse(localStorage.getItem('status') || '{}') || {};
      } catch {
        // A stale local status value must not prevent an otherwise valid export.
      }
      const configuredQuota = Number(localStorage.getItem('quota_per_unit'));
      const quotaPerUnit =
        Number.isFinite(configuredQuota) && configuredQuota > 0
          ? configuredQuota
          : 500000;
      const displayType = localStorage.getItem('quota_display_type') || 'USD';
      let unit = 'USD';
      let scale = 1 / quotaPerUnit;
      if (displayType === 'TOKENS') {
        unit = t('Tokens');
        scale = 1;
      } else if (displayType === 'CNY' || displayType === 'CUSTOM') {
        unit =
          displayType === 'CNY' ? 'CNY' : status.custom_currency_symbol || '¤';
        const rate = Number(
          displayType === 'CNY'
            ? status.usd_exchange_rate
            : status.custom_currency_exchange_rate,
        );
        scale *= Number.isFinite(rate) && rate > 0 ? rate : 1;
      }

      const count = await exportUsageLogsCsv({
        headers: [
          t('Username'),
          t('Token Name'),
          t('Model Name'),
          t('Input tokens'),
          t('Output tokens'),
          t('Cache read tokens'),
          t('Cache write tokens'),
          t('Amount ({{unit}})', { unit }),
          t('Time'),
        ],
        formatAmount: (quota) => quota * scale,
        formatTime: timestamp2string,
        filename: `usage-logs-${timestamp2string(now).replace(/[: ]/g, '-')}.csv`,
        signal: controller.signal,
        fetchPage: async (page, pageSize) => {
          const pageQuery = new URLSearchParams(query);
          pageQuery.set('p', String(page));
          pageQuery.set('page_size', String(pageSize));
          const response = await API.get(`${path}?${pageQuery}`, {
            signal: controller.signal,
            disableDuplicate: true,
            skipErrorHandler: true,
            timeout: 60000,
          });
          if (!response.data?.success || !response.data.data) {
            throw new Error(
              response.data?.message || t('Failed to export logs'),
            );
          }
          return response.data.data;
        },
      });
      if (count === 0 && !controller.signal.aborted) {
        showInfo(t('No logs to export'));
      }
    } catch {
      if (!controller.signal.aborted) showError(t('Failed to export logs'));
    } finally {
      if (exportController.current === controller) {
        exportController.current = null;
        setExportingLogs(false);
      }
    }
  };

  return { exportLogs, exportingLogs };
}
