package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepareBalanceSubscriptionTest(t *testing.T, userQuota int, planPrice float64) (*User, *SubscriptionPlan) {
	t.Helper()
	truncateTables(t)

	user := insertUserForPaymentGuardTest(t, 7501, userQuota)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 7601)
	plan.PriceAmount = planPrice
	require.NoError(t, DB.Save(plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	return user, plan
}

func enableBalanceSubscriptionForTest(t *testing.T, enabled bool) {
	t.Helper()
	payment := operation_setting.GetPaymentSetting()
	previous := payment.BalanceSubscriptionEnabled
	payment.BalanceSubscriptionEnabled = enabled
	t.Cleanup(func() { payment.BalanceSubscriptionEnabled = previous })
}

func assertNoBalanceSubscriptionWrites(t *testing.T, userID int) {
	t.Helper()
	var subscriptionCount, orderCount, topUpCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&subscriptionCount).Error)
	require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("user_id = ?", userID).Count(&orderCount).Error)
	require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ?", userID).Count(&topUpCount).Error)
	assert.Zero(t, subscriptionCount)
	assert.Zero(t, orderCount)
	assert.Zero(t, topUpCount)
}

func TestPurchaseSubscriptionWithBalanceDisabledHasNoSideEffects(t *testing.T) {
	enableBalanceSubscriptionForTest(t, false)
	user, plan := prepareBalanceSubscriptionTest(t, 1_000, 2.25)

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)

	require.ErrorIs(t, err, ErrSubscriptionBalancePaymentDisabled)
	assert.Equal(t, 1_000, getUserQuotaForPaymentGuardTest(t, user.Id))
	assertNoBalanceSubscriptionWrites(t, user.Id)
}

func TestPurchaseSubscriptionWithBalanceCreatesAtomicNonInvoiceablePurchase(t *testing.T) {
	enableBalanceSubscriptionForTest(t, true)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	user, plan := prepareBalanceSubscriptionTest(t, 1_000, 2.25)

	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))

	assert.Equal(t, 775, getUserQuotaForPaymentGuardTest(t, user.Id))
	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&subscription).Error)
	assert.Equal(t, "balance", subscription.Source)
	assert.False(t, subscription.InvoiceEligible)
	assert.Zero(t, subscription.PaidAmountMicros)
	assert.Empty(t, subscription.PaidCurrency)

	var order SubscriptionOrder
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&order).Error)
	assert.Equal(t, PaymentMethodBalance, order.PaymentMethod)
	assert.Equal(t, PaymentProviderBalance, order.PaymentProvider)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.InDelta(t, 2.25, order.Money, 0.000001)

	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
	assert.Equal(t, TopUpSourceSubscription, topUp.Source)
	assert.Equal(t, PaymentMethodBalance, topUp.PaymentMethod)
	assert.Equal(t, PaymentProviderBalance, topUp.PaymentProvider)
	assert.Zero(t, topUp.InvoiceAmountMicros)
	assert.Empty(t, topUp.InvoiceCurrency)
}

func TestPurchaseSubscriptionWithBalanceRoundsFractionalQuotaUp(t *testing.T) {
	enableBalanceSubscriptionForTest(t, true)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	user, plan := prepareBalanceSubscriptionTest(t, 2, 0.000001)

	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))

	assert.Equal(t, 1, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestPurchaseSubscriptionWithBalanceFailuresRollBack(t *testing.T) {
	enableBalanceSubscriptionForTest(t, true)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	t.Run("insufficient balance", func(t *testing.T) {
		user, plan := prepareBalanceSubscriptionTest(t, 224, 2.25)

		err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)

		require.ErrorIs(t, err, ErrSubscriptionInsufficientBalance)
		assert.Equal(t, 224, getUserQuotaForPaymentGuardTest(t, user.Id))
		assertNoBalanceSubscriptionWrites(t, user.Id)
	})

	t.Run("purchase limit", func(t *testing.T) {
		user, plan := prepareBalanceSubscriptionTest(t, 1_000, 2.25)
		plan.MaxPurchasePerUser = 1
		require.NoError(t, DB.Save(plan).Error)
		require.NoError(t, DB.Create(&UserSubscription{
			UserId: user.Id, PlanId: plan.Id, Status: "active", Source: "admin",
		}).Error)

		err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)

		require.ErrorIs(t, err, ErrSubscriptionPurchaseLimit)
		assert.Equal(t, 1_000, getUserQuotaForPaymentGuardTest(t, user.Id))
		var subscriptionCount, orderCount, topUpCount int64
		require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
		require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("user_id = ?", user.Id).Count(&orderCount).Error)
		require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ?", user.Id).Count(&topUpCount).Error)
		assert.Equal(t, int64(1), subscriptionCount)
		assert.Zero(t, orderCount)
		assert.Zero(t, topUpCount)
	})
}

func TestPurchaseSubscriptionWithBalanceRejectsQuotaOverflow(t *testing.T) {
	enableBalanceSubscriptionForTest(t, true)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = float64(common.MaxWalletQuota)
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	user, plan := prepareBalanceSubscriptionTest(t, common.MaxWalletQuota, 2)

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)

	require.Error(t, err)
	var clamp *common.QuotaClamp
	assert.True(t, errors.As(err, &clamp))
	assert.Equal(t, common.MaxWalletQuota, getUserQuotaForPaymentGuardTest(t, user.Id))
	assertNoBalanceSubscriptionWrites(t, user.Id)
}
