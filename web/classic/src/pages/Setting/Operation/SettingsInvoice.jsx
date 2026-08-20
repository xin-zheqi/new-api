import React, { useEffect, useState } from 'react';
import { Button, Card, Form, Switch, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, toBoolean } from '../../../helpers';

const SettingsInvoice = ({ options, refresh }) => {
  const { t } = useTranslation();
  const [values, setValues] = useState({
    InvoiceEnabled: true,
    InvoiceApplicationDay: 25,
    InvoiceLookbackDays: 90,
    InvoiceMonthlyLimit: 1,
  });

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
    try {
      for (const [key, value] of Object.entries(values)) {
        const response = await API.put('/api/option/', {
          key,
          value: String(value),
        });
        if (!response.data?.success) throw new Error(response.data?.message);
      }
      showSuccess(t('Invoice settings saved'));
      await refresh();
    } catch (error) {
      showError(error.message || t('Failed to save invoice settings'));
    }
  };

  return (
    <Card>
      <Typography.Title heading={5}>{t('Invoice settings')}</Typography.Title>
      <Typography.Paragraph type='tertiary'>
        {t('Configure invoice eligibility and the monthly application window.')}
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
        <Button theme='solid' onClick={save}>
          {t('Save invoice settings')}
        </Button>
      </Form>
    </Card>
  );
};

export default SettingsInvoice;
