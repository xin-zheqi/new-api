function toNumber(value, fallback = 0) {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string') {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return fallback;
}

function toStringValue(value, fallback = '') {
  return typeof value === 'string' ? value : fallback;
}

function toBoolean(value, fallback = false) {
  if (typeof value === 'boolean') {
    return value;
  }
  if (typeof value === 'number') {
    return value !== 0;
  }
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (normalized === 'true' || normalized === '1') {
      return true;
    }
    if (normalized === 'false' || normalized === '0') {
      return false;
    }
  }
  return fallback;
}

function normalizeSubscriptionPlan(plan) {
  if (!plan || typeof plan !== 'object') {
    return null;
  }
  const id = toNumber(plan.id, 0);
  const title = toStringValue(plan.title).trim();
  if (id <= 0 || title === '') {
    return null;
  }
  return {
    id,
    title,
    subtitle: toStringValue(plan.subtitle),
    price_amount: toNumber(plan.price_amount, 0),
    currency: toStringValue(plan.currency, 'USD') || 'USD',
    duration_unit: toStringValue(plan.duration_unit, 'month') || 'month',
    duration_value: toNumber(plan.duration_value, 1),
    custom_seconds: toNumber(plan.custom_seconds, 0),
    quota_reset_period:
      toStringValue(plan.quota_reset_period, 'never') || 'never',
    quota_reset_custom_seconds: toNumber(plan.quota_reset_custom_seconds, 0),
    enabled: toBoolean(plan.enabled, true),
    sort_order: toNumber(plan.sort_order, 0),
    max_purchase_per_user: toNumber(plan.max_purchase_per_user, 0),
    total_amount: toNumber(plan.total_amount, 0),
    upgrade_group: toStringValue(plan.upgrade_group),
    stripe_price_id: toStringValue(plan.stripe_price_id),
    creem_product_id: toStringValue(plan.creem_product_id),
    waffo_pancake_product_id: toStringValue(plan.waffo_pancake_product_id),
  };
}

export function normalizePlanRecords(data) {
  if (Array.isArray(data)) {
    return data.flatMap((item) => {
      const nestedPlan = normalizeSubscriptionPlan(item?.plan);
      if (nestedPlan) {
        return [{ plan: nestedPlan }];
      }
      const plainPlan = normalizeSubscriptionPlan(item);
      if (plainPlan) {
        return [{ plan: plainPlan }];
      }
      return [];
    });
  }
  if (data?.items) {
    return normalizePlanRecords(data.items);
  }
  if (data?.plans) {
    return normalizePlanRecords(data.plans);
  }
  return [];
}
