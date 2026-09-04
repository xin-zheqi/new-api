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

import React, { useEffect, useState, useContext, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  renderQuota,
  renderQuotaWithAmount,
  copy,
  getQuotaPerUnit,
  openHttpCheckoutUrl,
  redirectToHttpCheckoutUrl,
  submitHttpCheckoutForm,
  toBoolean,
} from '../../helpers';
import { Modal, Toast } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';

import RechargeCard from './RechargeCard';
import InvitationCard from './InvitationCard';
import TransferModal from './modals/TransferModal';
import PaymentConfirmModal from './modals/PaymentConfirmModal';
import TopupHistoryModal from './modals/TopupHistoryModal';
import { getPaymentAmountError } from './paymentError';
import WechatPayModal from './modals/WechatPayModal';

const TopUp = () => {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);

  const [redemptionCode, setRedemptionCode] = useState('');
  const [amount, setAmount] = useState(0.0);
  const [minTopUp, setMinTopUp] = useState(statusState?.status?.min_topup || 1);
  const [topUpCount, setTopUpCount] = useState(
    statusState?.status?.min_topup || 1,
  );
  const [topUpLink, setTopUpLink] = useState('');
  const [mallEnabled, setMallEnabled] = useState(false);
  const [mallModeKnown, setMallModeKnown] = useState(false);
  const [mallUrl, setMallUrl] = useState('');
  const [enableOnlineTopUp, setEnableOnlineTopUp] = useState(
    statusState?.status?.enable_online_topup || false,
  );
  const [priceRatio, setPriceRatio] = useState(statusState?.status?.price || 1);

  const [enableStripeTopUp, setEnableStripeTopUp] = useState(
    statusState?.status?.enable_stripe_topup || false,
  );
  const [statusLoading, setStatusLoading] = useState(true);

  // Creem 相关状态
  const [creemProducts, setCreemProducts] = useState([]);
  const [enableCreemTopUp, setEnableCreemTopUp] = useState(false);
  const [creemOpen, setCreemOpen] = useState(false);
  const [selectedCreemProduct, setSelectedCreemProduct] = useState(null);

  // Waffo 相关状态
  const [enableWaffoTopUp, setEnableWaffoTopUp] = useState(false);
  const [waffoPayMethods, setWaffoPayMethods] = useState([]);
  const [waffoMinTopUp, setWaffoMinTopUp] = useState(1);
  const [enableWaffoPancakeTopUp, setEnableWaffoPancakeTopUp] = useState(false);
  const [waffoPancakeMinTopUp, setWaffoPancakeMinTopUp] = useState(1);
  const [enableWechatTopUp, setEnableWechatTopUp] = useState(false);
  const [wechatUnitPrice, setWechatUnitPrice] = useState(7.3);
  const [wechatPayment, setWechatPayment] = useState(null);
  const [wechatVisible, setWechatVisible] = useState(false);

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [open, setOpen] = useState(false);
  const [payWay, setPayWay] = useState('');
  const [amountLoading, setAmountLoading] = useState(false);
  const [paymentLoading, setPaymentLoading] = useState(false);
  const [confirmLoading, setConfirmLoading] = useState(false);
  const [payMethods, setPayMethods] = useState([]);

  const affFetchedRef = useRef(false);

  // 邀请相关状态
  const [affLink, setAffLink] = useState('');
  const [openTransfer, setOpenTransfer] = useState(false);
  const [transferAmount, setTransferAmount] = useState(0);

  // 账单Modal状态
  const [openHistory, setOpenHistory] = useState(false);

  // 订阅相关
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const [subscriptionLoading, setSubscriptionLoading] = useState(true);
  const [billingPreference, setBillingPreference] =
    useState('subscription_first');
  const [activeSubscriptions, setActiveSubscriptions] = useState([]);
  const [allSubscriptions, setAllSubscriptions] = useState([]);

  // 预设充值额度选项
  const [presetAmounts, setPresetAmounts] = useState([]);
  const [selectedPreset, setSelectedPreset] = useState(null);

  // 充值配置信息
  const [topupInfo, setTopupInfo] = useState({
    amount_options: [],
    discount: {},
    wechat_min_topup: 1,
    wechat_unit_price: 7.3,
    default_payment_method: '',
    enable_redemption: true,
    enable_wechat_topup: false,
    payment_compliance_confirmed: true,
  });

  const confirmPayMethods = [
    ...payMethods,
    ...waffoPayMethods.map((method, index) => ({
      ...method,
      type: `waffo:${index}`,
      min_topup: waffoMinTopUp,
      color: method.color || 'rgba(var(--semi-primary-5), 1)',
    })),
  ];

  const getPayMethodConfig = (payment) =>
    confirmPayMethods.find((method) => method.type === payment);

  const getPaymentMinTopUp = (payment) => {
    const configuredMinTopUp = Number(getPayMethodConfig(payment)?.min_topup);
    if (Number.isFinite(configuredMinTopUp) && configuredMinTopUp > 0) {
      return configuredMinTopUp;
    }
    if (payment === 'wechat_native_v3') {
      const wechatMinimum = Number(topupInfo.wechat_min_topup);
      if (Number.isFinite(wechatMinimum) && wechatMinimum > 0) {
        return wechatMinimum;
      }
    }
    return minTopUp;
  };

  const requestAmountByPayment = async (payment, value) => {
    if (payment === 'stripe') {
      return getStripeAmount(value);
    }
    if (payment === 'waffo_pancake') {
      return getWaffoPancakeAmount(value);
    }
    if (typeof payment === 'string' && payment.startsWith('waffo:')) {
      return getWaffoAmount(value);
    }
    if (payment === 'wechat_native_v3') {
      return getWechatAmount(value);
    }
    return getAmount(value);
  };

  const getCurrentPaymentAmount = (value) =>
    requestAmountByPayment(payWay || payMethods[0]?.type || '', value);

  const getPaymentPriceRatio = (payment) => {
    if (payment === 'wechat_native_v3') {
      const configuredPrice = Number(wechatUnitPrice);
      return Number.isFinite(configuredPrice) && configuredPrice > 0
        ? configuredPrice
        : priceRatio;
    }
    return priceRatio;
  };

  const topUp = async () => {
    if (redemptionCode === '') {
      showInfo(t('请输入兑换码！'));
      return;
    }
    setIsSubmitting(true);
    try {
      const res = await API.post('/api/user/topup', {
        key: redemptionCode,
      });
      const { success, message, data } = res.data;
      if (success) {
        const kind = res.data.redemption_kind || 'quota';
        const subscription = res.data.subscription;
        if (kind === 'subscription' && subscription) {
          showSuccess(t('兑换成功！'));
          Modal.success({
            title: t('兑换成功！'),
            content: t('兑换成功，已开通订阅：') + (subscription.title || '-'),
            centered: true,
          });
          await getUserQuota();
          await getSubscriptionSelf();
        } else {
          showSuccess(t('兑换成功！'));
          Modal.success({
            title: t('兑换成功！'),
            content: t('成功兑换额度：') + renderQuota(data),
            centered: true,
          });
          if (userState.user) {
            const updatedUser = {
              ...userState.user,
              quota: userState.user.quota + data,
            };
            userDispatch({ type: 'login', payload: updatedUser });
          }
        }
        setRedemptionCode('');
      } else {
        showError(message);
      }
    } catch (err) {
      showError(t('请求失败'));
    } finally {
      setIsSubmitting(false);
    }
  };

  const openTopUpLink = () => {
    if (!topUpLink) {
      showError(t('超级管理员未设置充值链接！'));
      return;
    }
    if (!openHttpCheckoutUrl(topUpLink)) {
      showError(t('支付跳转地址不安全'));
    }
  };

  const preTopUp = async (payment) => {
    if (payment === 'wechat_native_v3') {
      if (!enableWechatTopUp) {
        showError(t('管理员未开启微信支付！'));
        return;
      }
    } else if (payment === 'stripe') {
      if (!enableStripeTopUp) {
        showError(t('管理员未开启Stripe充值！'));
        return;
      }
    } else if (payment === 'waffo_pancake') {
      if (!enableWaffoPancakeTopUp) {
        showError(t('管理员未开启 Waffo Pancake 充值！'));
        return;
      }
    } else if (payment.startsWith('waffo:')) {
      if (!enableWaffoTopUp) {
        showError(t('管理员未开启 Waffo 充值！'));
        return;
      }
    } else {
      if (!enableOnlineTopUp) {
        showError(t('管理员未开启在线充值！'));
        return;
      }
    }

    setPayWay(payment);
    setPaymentLoading(true);
    try {
      const selectedMinTopUp = getPaymentMinTopUp(payment);
      await requestAmountByPayment(payment, topUpCount);

      if (topUpCount < selectedMinTopUp) {
        showError(t('充值数量不能小于') + selectedMinTopUp);
        return;
      }
      setOpen(true);
    } catch (error) {
      showError(t('获取金额失败'));
    } finally {
      setPaymentLoading(false);
    }
  };

  const onlineTopUp = async () => {
    if (payWay === 'wechat_native_v3') {
      if (!enableWechatTopUp) {
        showError(t('管理员未开启微信支付！'));
        return;
      }
      setConfirmLoading(true);
      try {
        const res = await API.post('/api/user/wechat/pay', {
          amount: parseInt(topUpCount),
        });
        const response = res?.data;
        if (
          response?.success &&
          response.data?.trade_no &&
          response.data?.code_url
        ) {
          setWechatPayment(response.data);
          setOpen(false);
          setWechatVisible(true);
        } else {
          showError(response?.message || t('支付请求失败'));
        }
      } catch (error) {
        showError(t('支付请求失败'));
      } finally {
        setConfirmLoading(false);
      }
      return;
    }

    if (payWay === 'waffo_pancake') {
      setConfirmLoading(true);
      try {
        await waffoPancakeTopUp();
      } finally {
        setOpen(false);
        setConfirmLoading(false);
      }
      return;
    }

    if (payWay.startsWith('waffo:')) {
      const payMethodIndex = Number(payWay.split(':')[1]);
      setConfirmLoading(true);
      try {
        await waffoTopUp(Number.isFinite(payMethodIndex) ? payMethodIndex : 0);
      } finally {
        setOpen(false);
        setConfirmLoading(false);
      }
      return;
    }

    const selectedMinTopUp = getPaymentMinTopUp(payWay);
    if (amount === 0) {
      await requestAmountByPayment(payWay, topUpCount);
    }

    if (topUpCount < selectedMinTopUp) {
      showError(t('充值数量不能小于') + selectedMinTopUp);
      return;
    }
    setConfirmLoading(true);
    try {
      let res;
      if (payWay === 'stripe') {
        // Stripe 支付请求
        res = await API.post('/api/user/stripe/pay', {
          amount: parseInt(topUpCount),
          payment_method: 'stripe',
        });
      } else {
        // 普通支付请求
        res = await API.post('/api/user/pay', {
          amount: parseInt(topUpCount),
          payment_method: payWay,
        });
      }

      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          if (payWay === 'stripe') {
            // Stripe 支付回调处理
            if (!openHttpCheckoutUrl(data.pay_link)) {
              showError(t('支付跳转地址不安全'));
              return;
            }
          } else {
            // 普通支付表单提交
            if (!submitHttpCheckoutForm(res.data.url, data)) {
              showError(t('支付跳转地址不安全'));
              return;
            }
          }
        } else {
          const errorMsg =
            typeof data === 'string' ? data : message || t('支付失败');
          showError(errorMsg);
        }
      } else {
        showError(res);
      }
    } catch (err) {
      showError(t('支付请求失败'));
    } finally {
      setOpen(false);
      setConfirmLoading(false);
    }
  };

  const creemPreTopUp = async (product) => {
    if (!enableCreemTopUp) {
      showError(t('管理员未开启 Creem 充值！'));
      return;
    }
    setSelectedCreemProduct(product);
    setCreemOpen(true);
  };

  const onlineCreemTopUp = async () => {
    if (!selectedCreemProduct) {
      showError(t('请选择产品'));
      return;
    }
    // Validate product has required fields
    if (!selectedCreemProduct.productId) {
      showError(t('产品配置错误，请联系管理员'));
      return;
    }
    setConfirmLoading(true);
    try {
      const res = await API.post('/api/user/creem/pay', {
        product_id: selectedCreemProduct.productId,
        payment_method: 'creem',
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          processCreemCallback(data);
        } else {
          const errorMsg =
            typeof data === 'string' ? data : message || t('支付失败');
          showError(errorMsg);
        }
      } else {
        showError(res);
      }
    } catch (err) {
      showError(t('支付请求失败'));
    } finally {
      setCreemOpen(false);
      setConfirmLoading(false);
    }
  };

  const waffoTopUp = async (payMethodIndex) => {
    try {
      if (topUpCount < waffoMinTopUp) {
        showError(t('充值数量不能小于') + waffoMinTopUp);
        return;
      }
      setPaymentLoading(true);
      const requestBody = {
        amount: parseInt(topUpCount),
      };
      if (payMethodIndex != null) {
        requestBody.pay_method_index = payMethodIndex;
      }
      const res = await API.post('/api/user/waffo/pay', requestBody);
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success' && data?.payment_url) {
          if (!openHttpCheckoutUrl(data.payment_url)) {
            showError(t('支付跳转地址不安全'));
          }
        } else {
          showError(data || t('支付请求失败'));
        }
      } else {
        showError(res);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaymentLoading(false);
    }
  };

  const getWaffoAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post('/api/user/waffo/amount', {
        amount: parseInt(value),
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({
            content: getPaymentAmountError(message, data, t('获取金额失败')),
            id: 'getAmount',
          });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // amount fetch failed silently
    } finally {
      setAmountLoading(false);
    }
  };

  const getWechatAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post('/api/user/wechat/amount', {
        amount: parseInt(value),
      });
      if (res !== undefined) {
        const { success, message, data } = res.data;
        if (success || message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({
            content: getPaymentAmountError(message, data, t('获取金额失败')),
            id: 'getAmount',
          });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // The checkout request performs the same server-side validation.
    } finally {
      setAmountLoading(false);
    }
  };

  const waffoPancakeTopUp = async () => {
    const minTopUpValue = Number(waffoPancakeMinTopUp || 1);
    if (topUpCount < minTopUpValue) {
      showError(t('充值数量不能小于') + minTopUpValue);
      return;
    }

    setPaymentLoading(true);
    try {
      const res = await API.post('/api/user/waffo-pancake/pay', {
        amount: parseInt(topUpCount),
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          const checkoutUrl = data?.checkout_url || '';
          if (checkoutUrl && redirectToHttpCheckoutUrl(checkoutUrl)) {
            // In-tab redirect (not window.open) — popup blocker fires after
            // the await loses user-gesture context.
          } else if (checkoutUrl) {
            showError(t('支付跳转地址不安全'));
          } else {
            showError(t('支付请求失败'));
          }
        } else {
          const errorMsg =
            typeof data === 'string' ? data : message || t('支付请求失败');
          showError(errorMsg);
        }
      } else {
        showError(res);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaymentLoading(false);
    }
  };

  const getWaffoPancakeAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post('/api/user/waffo-pancake/amount', {
        amount: parseInt(value),
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({
            content: getPaymentAmountError(message, data, t('获取金额失败')),
            id: 'getAmount',
          });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // amount fetch failed silently
    } finally {
      setAmountLoading(false);
    }
  };

  const processCreemCallback = (data) => {
    // 与 Stripe 保持一致的实现方式
    if (!openHttpCheckoutUrl(data?.checkout_url)) {
      showError(t('支付跳转地址不安全'));
    }
  };

  const getUserQuota = async () => {
    let res = await API.get(`/api/user/self`);
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
    } else {
      showError(message);
    }
  };

  const getSubscriptionPlans = async () => {
    setSubscriptionLoading(true);
    try {
      const res = await API.get('/api/subscription/plans');
      if (res.data?.success) {
        setSubscriptionPlans(res.data.data || []);
      }
    } catch (e) {
      setSubscriptionPlans([]);
    } finally {
      setSubscriptionLoading(false);
    }
  };

  const getSubscriptionSelf = async () => {
    try {
      const res = await API.get('/api/subscription/self');
      if (res.data?.success) {
        setBillingPreference(
          res.data.data?.billing_preference || 'subscription_first',
        );
        // Active subscriptions
        const activeSubs = res.data.data?.subscriptions || [];
        setActiveSubscriptions(activeSubs);
        // All subscriptions (including expired)
        const allSubs = res.data.data?.all_subscriptions || [];
        setAllSubscriptions(allSubs);
      }
    } catch (e) {
      // ignore
    }
  };

  const updateBillingPreference = async (pref) => {
    const previousPref = billingPreference;
    setBillingPreference(pref);
    try {
      const res = await API.put('/api/subscription/self/preference', {
        billing_preference: pref,
      });
      if (res.data?.success) {
        showSuccess(t('更新成功'));
        const normalizedPref =
          res.data?.data?.billing_preference || pref || previousPref;
        setBillingPreference(normalizedPref);
      } else {
        showError(res.data?.message || t('更新失败'));
        setBillingPreference(previousPref);
      }
    } catch (e) {
      showError(t('请求失败'));
      setBillingPreference(previousPref);
    }
  };

  // 获取充值配置信息
  const getTopupInfo = async () => {
    setMallModeKnown(false);
    try {
      const res = await API.get('/api/user/topup/info');
      const { message, data, success } = res.data;
      if (success) {
        const parseArray = (value) => {
          if (Array.isArray(value)) return value;
          if (typeof value !== 'string' || value.trim() === '') return [];
          try {
            const parsed = JSON.parse(value);
            return Array.isArray(parsed) ? parsed : [];
          } catch {
            return [];
          }
        };
        const normalizedAmountOptions = parseArray(data.amount_options)
          .map((value) => Number(value))
          .filter((value) => Number.isFinite(value) && value > 0);
        const normalizedDiscount = (() => {
          const parsed =
            typeof data.discount === 'string'
              ? (() => {
                  try {
                    return JSON.parse(data.discount);
                  } catch {
                    return {};
                  }
                })()
              : data.discount;
          return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
            ? parsed
            : {};
        })();
        const wechatMinimum = Number(data.wechat_min_topup);
        const normalizedWechatMinimum =
          Number.isFinite(wechatMinimum) && wechatMinimum > 0
            ? wechatMinimum
            : 1;
        const normalizedWechatUnitPrice = Number(data.wechat_unit_price);
        const effectiveWechatUnitPrice =
          Number.isFinite(normalizedWechatUnitPrice) &&
          normalizedWechatUnitPrice > 0
            ? normalizedWechatUnitPrice
            : priceRatio;
        const enableStripeTopUp = toBoolean(data.enable_stripe_topup);
        const enableOnlineTopUp = toBoolean(data.enable_online_topup);
        const enableCreemTopUp = toBoolean(data.enable_creem_topup);
        const enableWaffoTopUp = toBoolean(data.enable_waffo_topup);
        const enableWaffoPancakeTopUp = toBoolean(
          data.enable_waffo_pancake_topup,
        );
        const enableWechatTopUp = toBoolean(data.enable_wechat_topup);
        setMallEnabled(toBoolean(data.mall_enabled));
        setMallUrl(data.mall_url || '');
        setMallModeKnown(true);

        // 处理支付方式
        let payMethods = parseArray(data.pay_methods);
        try {
          payMethods = payMethods
            .filter((method) => method && method.name && method.type)
            .map((method) => {
              const type = String(method.type);
              const configuredMinTopup = Number(method.min_topup);
              let minimum =
                Number.isFinite(configuredMinTopup) && configuredMinTopup > 0
                  ? configuredMinTopup
                  : 0;
              if (minimum <= 0 && type === 'stripe') {
                const stripeMinimum = Number(data.stripe_min_topup);
                minimum =
                  Number.isFinite(stripeMinimum) && stripeMinimum > 0
                    ? stripeMinimum
                    : 0;
              }
              if (minimum <= 0 && type === 'wechat_native_v3') {
                minimum = normalizedWechatMinimum;
              }

              let color = method.color;
              if (!color) {
                if (type === 'alipay') {
                  color = 'rgba(var(--semi-blue-5), 1)';
                } else if (type === 'wxpay') {
                  color = 'rgba(var(--semi-green-5), 1)';
                } else if (type === 'stripe') {
                  color = 'rgba(var(--semi-purple-5), 1)';
                } else {
                  color = 'rgba(var(--semi-primary-5), 1)';
                }
              }
              return { ...method, type, min_topup: minimum, color };
            });

          // 如果启用了 Stripe 支付，添加到支付方法列表
          // 这个逻辑现在由后端处理，如果 Stripe 启用，后端会在 pay_methods 中包含它

          setPayMethods(payMethods);
          const configuredDefault = String(
            data.default_payment_method || '',
          ).trim();
          let defaultPayment = payMethods.some(
            (method) => method.type === configuredDefault,
          )
            ? configuredDefault
            : '';
          if (
            !defaultPayment &&
            enableWechatTopUp &&
            payMethods.some((method) => method.type === 'wechat_native_v3')
          ) {
            defaultPayment = 'wechat_native_v3';
          }
          if (!defaultPayment) {
            defaultPayment = payMethods[0]?.type || '';
          }
          setPayWay(defaultPayment);

          let minTopUpValue = Number(data.min_topup);
          const selectedMethod = payMethods.find(
            (method) => method.type === defaultPayment,
          );
          if (selectedMethod?.min_topup > 0) {
            minTopUpValue = selectedMethod.min_topup;
          } else if (defaultPayment === 'stripe') {
            minTopUpValue = Number(data.stripe_min_topup);
          } else if (defaultPayment === 'waffo_pancake') {
            minTopUpValue = Number(data.waffo_pancake_min_topup);
          } else if (defaultPayment.startsWith('waffo:')) {
            minTopUpValue = Number(data.waffo_min_topup);
          } else if (defaultPayment === 'wechat_native_v3') {
            minTopUpValue = normalizedWechatMinimum;
          }
          if (!Number.isFinite(minTopUpValue) || minTopUpValue <= 0) {
            minTopUpValue = 1;
          }

          setEnableOnlineTopUp(enableOnlineTopUp);
          setEnableStripeTopUp(enableStripeTopUp);
          setEnableCreemTopUp(enableCreemTopUp);
          setEnableWaffoTopUp(enableWaffoTopUp);
          setWaffoPayMethods(data.waffo_pay_methods || []);
          setWaffoMinTopUp(Number(data.waffo_min_topup) || 1);
          setEnableWaffoPancakeTopUp(enableWaffoPancakeTopUp);
          setEnableWechatTopUp(enableWechatTopUp);
          setWaffoPancakeMinTopUp(Number(data.waffo_pancake_min_topup) || 1);
          setMinTopUp(minTopUpValue);
          setTopUpCount(minTopUpValue);
          setTopUpLink(data.topup_link || '');
          setTopupInfo({
            amount_options: normalizedAmountOptions,
            discount: normalizedDiscount,
            enable_redemption: !(
              data.enable_redemption === false ||
              data.enable_redemption === 'false'
            ),
            payment_compliance_confirmed:
              data.payment_compliance_confirmed !== false,
            payment_compliance_terms_version:
              data.payment_compliance_terms_version || '',
            enable_wechat_topup: enableWechatTopUp,
            wechat_min_topup: normalizedWechatMinimum,
            wechat_unit_price: effectiveWechatUnitPrice,
            default_payment_method: defaultPayment,
            pay_methods: payMethods,
            enable_online_topup: enableOnlineTopUp,
            enable_stripe_topup: enableStripeTopUp,
            enable_creem_topup: enableCreemTopUp,
            enable_waffo_topup: enableWaffoTopUp,
            enable_waffo_pancake_topup: enableWaffoPancakeTopUp,
            min_topup: Number(data.min_topup) || 1,
            stripe_min_topup: Number(data.stripe_min_topup) || 1,
            waffo_min_topup: Number(data.waffo_min_topup) || 1,
            waffo_pancake_min_topup: Number(data.waffo_pancake_min_topup) || 1,
          });
          setWechatUnitPrice(effectiveWechatUnitPrice);

          // 设置 Creem 产品
          try {
            const products = JSON.parse(data.creem_products || '[]');
            setCreemProducts(products);
          } catch (e) {
            setCreemProducts([]);
          }

          // 如果没有自定义充值数量选项，根据最小充值金额生成预设充值额度选项
          if (normalizedAmountOptions.length === 0) {
            setPresetAmounts(generatePresetAmounts(minTopUpValue));
          }

          // 初始化显示实付金额
          await requestAmountByPayment(defaultPayment, minTopUpValue);
        } catch (e) {
          setPayMethods([]);
        }

        // 如果有自定义充值数量选项，使用它们替换默认的预设选项
        if (normalizedAmountOptions.length > 0) {
          const customPresets = normalizedAmountOptions.map((amount) => ({
            value: amount,
            discount: normalizedDiscount[amount] || 1.0,
          }));
          setPresetAmounts(customPresets);
        }
      } else {
        setMallEnabled(false);
        setMallModeKnown(false);
        setMallUrl('');
        showError(data || t('获取充值配置失败'));
      }
    } catch (error) {
      setMallEnabled(false);
      setMallModeKnown(false);
      setMallUrl('');
      showError(t('获取充值配置异常'));
    }
  };

  // 获取邀请链接
  const getAffLink = async () => {
    const res = await API.get('/api/user/aff');
    const { success, message, data } = res.data;
    if (success) {
      let link = `${window.location.origin}/register?aff=${data}`;
      setAffLink(link);
    } else {
      showError(message);
    }
  };

  // 划转邀请额度
  const transfer = async () => {
    if (transferAmount < getQuotaPerUnit()) {
      showError(t('划转金额最低为') + ' ' + renderQuota(getQuotaPerUnit()));
      return;
    }
    const res = await API.post(`/api/user/aff_transfer`, {
      quota: transferAmount,
    });
    const { success, message } = res.data;
    if (success) {
      showSuccess(message);
      setOpenTransfer(false);
      getUserQuota().then();
    } else {
      showError(message);
    }
  };

  // 复制邀请链接
  const handleAffLinkClick = async () => {
    await copy(affLink);
    showSuccess(t('邀请链接已复制到剪切板'));
  };

  // URL 参数自动打开账单弹窗（支付回跳时触发）
  useEffect(() => {
    if (searchParams.get('show_history') === 'true') {
      setOpenHistory(true);
      searchParams.delete('show_history');
      setSearchParams(searchParams, { replace: true });
    }
  }, []);

  useEffect(() => {
    // 始终获取最新用户数据，确保余额等统计信息准确
    getUserQuota().then();
    setTransferAmount(getQuotaPerUnit());
  }, []);

  useEffect(() => {
    if (affFetchedRef.current) return;
    affFetchedRef.current = true;
    getAffLink().then();
  }, []);

  // 在 statusState 可用时获取充值信息
  useEffect(() => {
    getTopupInfo().then();
    getSubscriptionPlans().then();
    getSubscriptionSelf().then();
  }, []);

  useEffect(() => {
    if (statusState?.status) {
      // const minTopUpValue = statusState.status.min_topup || 1;
      // setMinTopUp(minTopUpValue);
      // setTopUpCount(minTopUpValue);
      setPriceRatio(statusState.status.price || 1);

      setStatusLoading(false);
    }
  }, [statusState?.status]);

  const renderAmount = () => {
    return amount + ' ' + t('元');
  };

  const getAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post('/api/user/amount', {
        amount: parseFloat(value),
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({
            content: getPaymentAmountError(message, data, t('获取金额失败')),
            id: 'getAmount',
          });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // amount fetch failed silently
    }
    setAmountLoading(false);
  };

  const getStripeAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post('/api/user/stripe/amount', {
        amount: parseFloat(value),
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({
            content: getPaymentAmountError(message, data, t('获取金额失败')),
            id: 'getAmount',
          });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // amount fetch failed silently
    } finally {
      setAmountLoading(false);
    }
  };

  const handleCancel = () => {
    setOpen(false);
  };

  const handleTransferCancel = () => {
    setOpenTransfer(false);
  };

  const handleOpenHistory = () => {
    setOpenHistory(true);
  };

  const handleHistoryCancel = () => {
    setOpenHistory(false);
  };

  const handleCreemCancel = () => {
    setCreemOpen(false);
    setSelectedCreemProduct(null);
  };

  // 选择预设充值额度
  const selectPresetAmount = (preset) => {
    setTopUpCount(preset.value);
    setSelectedPreset(preset.value);

    // Keep the displayed amount in sync with the selected gateway. Native
    // WeChat uses its own CNY exchange rate, while Epay uses the legacy rate.
    requestAmountByPayment(payWay || payMethods[0]?.type || '', preset.value);
  };

  // 格式化大数字显示
  const formatLargeNumber = (num) => {
    return num.toString();
  };

  // 根据最小充值金额生成预设充值额度选项
  const generatePresetAmounts = (minAmount) => {
    const multipliers = [1, 5, 10, 30, 50, 100, 300, 500];
    return multipliers.map((multiplier) => ({
      value: minAmount * multiplier,
    }));
  };

  return (
    <div className='w-full max-w-7xl mx-auto relative min-h-screen lg:min-h-0 mt-[60px] px-2'>
      {/* 划转模态框 */}
      <TransferModal
        t={t}
        openTransfer={openTransfer}
        transfer={transfer}
        handleTransferCancel={handleTransferCancel}
        userState={userState}
        renderQuota={renderQuota}
        getQuotaPerUnit={getQuotaPerUnit}
        transferAmount={transferAmount}
        setTransferAmount={setTransferAmount}
      />

      {/* 充值确认模态框 */}
      <PaymentConfirmModal
        t={t}
        open={open}
        onlineTopUp={onlineTopUp}
        handleCancel={handleCancel}
        confirmLoading={confirmLoading}
        topUpCount={topUpCount}
        renderQuotaWithAmount={renderQuotaWithAmount}
        amountLoading={amountLoading}
        renderAmount={renderAmount}
        payWay={payWay}
        payMethods={confirmPayMethods}
        amountNumber={amount}
        discountRate={topupInfo?.discount?.[topUpCount] || 1.0}
      />

      {/* 充值账单模态框 */}
      <TopupHistoryModal
        visible={openHistory}
        onCancel={handleHistoryCancel}
        t={t}
      />

      {/* Creem 充值确认模态框 */}
      <Modal
        title={t('确定要充值 $')}
        visible={creemOpen}
        onOk={onlineCreemTopUp}
        onCancel={handleCreemCancel}
        maskClosable={false}
        size='small'
        centered
        confirmLoading={confirmLoading}
      >
        {selectedCreemProduct && (
          <>
            <p>
              {t('产品名称')}：{selectedCreemProduct.name}
            </p>
            <p>
              {t('价格')}：{selectedCreemProduct.currency === 'EUR' ? '€' : '$'}
              {selectedCreemProduct.price}
            </p>
            <p>
              {t('充值额度')}：{selectedCreemProduct.quota}
            </p>
            <p>{t('是否确认充值？')}</p>
          </>
        )}
      </Modal>

      {wechatPayment ? (
        <WechatPayModal
          t={t}
          visible={wechatVisible}
          payment={wechatPayment}
          onCancel={() => {
            setWechatVisible(false);
            setWechatPayment(null);
          }}
          onSuccess={async () => {
            await Promise.all([getUserQuota(), getSubscriptionSelf()]);
          }}
        />
      ) : null}

      {/* 主布局区域 */}
      <div className='grid grid-cols-1 lg:grid-cols-2 gap-6'>
        <RechargeCard
          t={t}
          enableOnlineTopUp={enableOnlineTopUp}
          enableStripeTopUp={enableStripeTopUp}
          enableCreemTopUp={enableCreemTopUp}
          creemProducts={creemProducts}
          creemPreTopUp={creemPreTopUp}
          enableWaffoTopUp={enableWaffoTopUp}
          enableWaffoPancakeTopUp={enableWaffoPancakeTopUp}
          enableWechatTopUp={enableWechatTopUp}
          presetAmounts={presetAmounts}
          selectedPreset={selectedPreset}
          selectPresetAmount={selectPresetAmount}
          formatLargeNumber={formatLargeNumber}
          priceRatio={getPaymentPriceRatio(payWay || payMethods[0]?.type || '')}
          topUpCount={topUpCount}
          minTopUp={getPaymentMinTopUp(payWay || payMethods[0]?.type || '')}
          renderQuotaWithAmount={renderQuotaWithAmount}
          getAmount={getCurrentPaymentAmount}
          setTopUpCount={setTopUpCount}
          setSelectedPreset={setSelectedPreset}
          renderAmount={renderAmount}
          amountLoading={amountLoading}
          payMethods={confirmPayMethods}
          preTopUp={preTopUp}
          paymentLoading={paymentLoading}
          payWay={payWay}
          redemptionCode={redemptionCode}
          setRedemptionCode={setRedemptionCode}
          topUp={topUp}
          isSubmitting={isSubmitting}
          topUpLink={topUpLink}
          openTopUpLink={openTopUpLink}
          userState={userState}
          renderQuota={renderQuota}
          statusLoading={statusLoading}
          topupInfo={topupInfo}
          onOpenHistory={handleOpenHistory}
          subscriptionLoading={subscriptionLoading}
          subscriptionPlans={subscriptionPlans}
          billingPreference={billingPreference}
          onChangeBillingPreference={updateBillingPreference}
          activeSubscriptions={activeSubscriptions}
          allSubscriptions={allSubscriptions}
          reloadSubscriptionSelf={getSubscriptionSelf}
          enableRedemption={topupInfo.enable_redemption !== false}
          walletOnly
          mallEnabled={mallEnabled}
          mallModeKnown={mallModeKnown}
          mallUrl={mallUrl}
          reloadUser={getUserQuota}
        />
        <InvitationCard
          t={t}
          userState={userState}
          renderQuota={renderQuota}
          setOpenTransfer={setOpenTransfer}
          affLink={affLink}
          handleAffLinkClick={handleAffLinkClick}
          complianceConfirmed={topupInfo.payment_compliance_confirmed !== false}
        />
      </div>
    </div>
  );
};

export default TopUp;
