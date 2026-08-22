/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

import React, { useEffect, useState } from 'react';
import { Spin } from '@douyinfe/semi-ui';
import SettingsInvoice from '../../pages/Setting/Operation/SettingsInvoice';
import { API, showError, toBoolean } from '../../helpers';

const invoiceBooleanKeys = new Set([
  'InvoiceEnabled',
  'InvoiceSystemRechargeEnabled',
  'InvoiceRedemptionRechargeEnabled',
  'InvoiceSystemSubscriptionEnabled',
  'InvoiceRedemptionSubscriptionEnabled',
]);
const invoiceNumberKeys = new Set([
  'InvoiceApplicationDay',
  'InvoiceLookbackDays',
  'InvoiceMonthlyLimit',
]);

const InvoiceSetting = () => {
  const [options, setOptions] = useState({});
  const [loading, setLoading] = useState(false);

  const refresh = async () => {
    setLoading(true);
    try {
      const response = await API.get('/api/option/');
      const { success, message, data } = response.data;
      if (!success) {
        showError(message);
        return;
      }
      const invoiceOptions = {};
      data.forEach((item) => {
        if (invoiceBooleanKeys.has(item.key)) {
          invoiceOptions[item.key] = toBoolean(item.value);
        } else if (invoiceNumberKeys.has(item.key)) {
          invoiceOptions[item.key] = Number(item.value);
        }
      });
      setOptions(invoiceOptions);
    } catch (error) {
      showError('刷新失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  return (
    <Spin spinning={loading} size='large'>
      <SettingsInvoice options={options} refresh={refresh} />
    </Spin>
  );
};

export default InvoiceSetting;
