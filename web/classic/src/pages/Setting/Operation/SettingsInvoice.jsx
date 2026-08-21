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
import { Button, Card, Form, Switch, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, toBoolean } from '../../../helpers';
import { getInvoiceErrorMessage } from '../../Invoice/invoice';

const SettingsInvoice = ({ options, refresh }) => {
  const { t } = useTranslation();
  const [values, setValues] = useState({
    InvoiceEnabled: true,
    InvoiceApplicationDay: 25,
    InvoiceLookbackDays: 90,
    InvoiceMonthlyLimit: 1,
  });
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setValues({
      InvoiceEnabled:
        options.InvoiceEnabled === undefined
          ? true
          : toBoolean(options.InvoiceEnabled),
      InvoiceApplicationDay: Number(options.InvoiceApplicationDay || 25),
      InvoiceLookbackDays: Number(options.InvoiceLookbackDays || 90),
      InvoiceMonthlyLimit: Number(options.InvoiceMonthlyLimit || 1),
    });
  }, [options]);

  const save = async () => {
    setSaving(true);
    try {
      const response = await API.put(
        '/api/option/invoice',
        {
          invoice_enabled: values.InvoiceEnabled,
          application_day: Number(values.InvoiceApplicationDay),
          lookback_days: Number(values.InvoiceLookbackDays),
          monthly_limit: Number(values.InvoiceMonthlyLimit),
        },
        { skipErrorHandler: true },
      );
      if (!response.data?.success) {
        showError(
          getInvoiceErrorMessage(
            response,
            t,
            t('Failed to save invoice settings'),
          ),
        );
        return;
      }
      showSuccess(t('Invoice settings saved'));
      await refresh();
    } catch (error) {
      showError(
        getInvoiceErrorMessage(error, t, t('Failed to save invoice settings')),
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <Typography.Title heading={5}>{t('Invoice settings')}</Typography.Title>
      <Typography.Paragraph type='tertiary'>
        {t('Configure the invoice application window and monthly limits.')}
      </Typography.Paragraph>
      <Form layout='vertical'>
        <div className='mb-4 flex items-center justify-between'>
          <Typography.Text>{t('Enable invoice center')}</Typography.Text>
          <Switch
            checked={values.InvoiceEnabled}
            onChange={(checked) =>
              setValues((current) => ({
                ...current,
                InvoiceEnabled: checked,
              }))
            }
          />
        </div>
        <Form.InputNumber
          field='InvoiceApplicationDay'
          label={t('Application day of each month')}
          min={1}
          max={28}
          value={values.InvoiceApplicationDay}
          onChange={(value) =>
            setValues((current) => ({
              ...current,
              InvoiceApplicationDay: value,
            }))
          }
        />
        <Form.InputNumber
          field='InvoiceLookbackDays'
          label={t('Eligible subscription lookback (days)')}
          min={1}
          max={3650}
          value={values.InvoiceLookbackDays}
          onChange={(value) =>
            setValues((current) => ({ ...current, InvoiceLookbackDays: value }))
          }
        />
        <Form.InputNumber
          field='InvoiceMonthlyLimit'
          label={t('Applications per user per month')}
          min={1}
          max={31}
          value={values.InvoiceMonthlyLimit}
          onChange={(value) =>
            setValues((current) => ({ ...current, InvoiceMonthlyLimit: value }))
          }
        />
        <Button theme='solid' loading={saving} disabled={saving} onClick={save}>
          {t('Save invoice settings')}
        </Button>
      </Form>
    </Card>
  );
};

export default SettingsInvoice;
