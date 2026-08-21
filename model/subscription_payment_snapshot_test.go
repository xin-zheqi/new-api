package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSubscriptionPaymentSnapshotParsesExactProviderAmounts(t *testing.T) {
	major, err := NewSubscriptionPaymentFromMajorUnits(" 12.345678 ", "cny")
	require.NoError(t, err)
	assert.Equal(t, int64(12_345_678), major.AmountMicros)
	assert.Equal(t, "CNY", major.Currency)

	minor, err := NewSubscriptionPaymentFromMinorUnits(999, "usd")
	require.NoError(t, err)
	assert.Equal(t, int64(9_990_000), minor.AmountMicros)
	assert.Equal(t, "USD", minor.Currency)

	for _, amount := range []string{"", "0", "-1", "+1", "1e2", "1.0000001", "1.", ".5"} {
		_, err := NewSubscriptionPaymentFromMajorUnits(amount, "USD")
		require.Error(t, err, amount)
	}
	yen, err := NewSubscriptionPaymentFromMinorUnits(100, "JPY")
	require.NoError(t, err)
	assert.Equal(t, int64(100_000_000), yen.AmountMicros)
	_, err = NewSubscriptionPaymentFromMinorUnits(100, "US1")
	require.Error(t, err)
}

func TestCreateUserSubscriptionOnlyEnablesInvoiceForPaidSources(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{
		Title: "Paid source plan", PriceAmount: 9.99, Currency: "USD", DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, Enabled: true, TotalAmount: 1000, InvoiceEligible: true,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	user := User{Username: "paid-source-user", Password: "password", AffCode: "paid-source-user"}
	require.NoError(t, DB.Create(&user).Error)
	payment := SubscriptionPaymentSnapshot{AmountMicros: 9_990_000, Currency: "USD"}

	var adminSubscription, redemptionSubscription, paidSubscription *UserSubscription
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		adminSubscription, err = CreateUserSubscriptionFromPlanTx(tx, user.Id, &plan, "admin", nil)
		if err != nil {
			return err
		}
		redemptionSubscription, err = CreateUserSubscriptionFromPlanTx(tx, user.Id, &plan, "redemption", nil)
		if err != nil {
			return err
		}
		paidSubscription, err = CreateUserSubscriptionFromPlanTx(tx, user.Id, &plan, "order", &payment)
		return err
	}))

	for _, subscription := range []*UserSubscription{adminSubscription, redemptionSubscription} {
		assert.False(t, subscription.InvoiceEligible)
		assert.Zero(t, subscription.PaidAmountMicros)
		assert.Empty(t, subscription.PaidCurrency)
	}
	assert.True(t, paidSubscription.InvoiceEligible)
	assert.Equal(t, payment.AmountMicros, paidSubscription.PaidAmountMicros)
	assert.Equal(t, payment.Currency, paidSubscription.PaidCurrency)
}

func TestCompleteSubscriptionOrderRejectsUnderpaymentAndSnapshotsVerifiedCharge(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{
		Title: "Verified payment plan", PriceAmount: 9.99, Currency: "USD", DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, Enabled: true, TotalAmount: 1000, InvoiceEligible: true, StripePriceId: "price_verified",
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	user := User{Username: "verified-payment-user", Password: "password", AffCode: "verified-payment-user"}
	require.NoError(t, DB.Create(&user).Error)
	order := SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "verified-subscription-order",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
		ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
	}
	require.NoError(t, CreatePendingSubscriptionOrder(&order, &plan))

	err := CompleteSubscriptionOrder(order.TradeNo, `{}`, PaymentProviderStripe, "", SubscriptionPaymentSnapshot{AmountMicros: 1_000_000, Currency: "USD"})
	require.ErrorIs(t, err, ErrSubscriptionPaymentMismatch)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, user.Id))

	err = CompleteSubscriptionOrder(order.TradeNo, `{}`, PaymentProviderStripe, "", SubscriptionPaymentSnapshot{AmountMicros: 9_990_000, Currency: "CNY"})
	require.ErrorIs(t, err, ErrSubscriptionPaymentMismatch)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, user.Id))

	verifiedPayment := SubscriptionPaymentSnapshot{AmountMicros: 10_500_000, Currency: "USD"}
	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, `{"verified":true}`, PaymentProviderStripe, "card", verifiedPayment))
	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, `{"verified":true}`, PaymentProviderStripe, "card", verifiedPayment))
	assert.Equal(t, int64(1), countUserSubscriptionsForPaymentGuardTest(t, user.Id))

	completedOrder := GetSubscriptionOrderByTradeNo(order.TradeNo)
	require.NotNil(t, completedOrder)
	assert.Equal(t, common.TopUpStatusSuccess, completedOrder.Status)
	assert.Equal(t, verifiedPayment.AmountMicros, completedOrder.PaidAmountMicros)
	assert.Equal(t, verifiedPayment.Currency, completedOrder.PaidCurrency)
	assert.Equal(t, "card", completedOrder.PaymentMethod)
	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
	assert.InDelta(t, 10.5, topUp.Money, 0.000001)
	assert.Equal(t, "card", topUp.PaymentMethod)
	assert.Equal(t, PaymentProviderStripe, topUp.PaymentProvider)
	assert.Equal(t, TopUpSourceSubscription, topUp.Source)
	assert.Equal(t, "USD", topUp.Currency)
	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&subscription).Error)
	assert.True(t, subscription.InvoiceEligible)
	assert.Equal(t, verifiedPayment.AmountMicros, subscription.PaidAmountMicros)
	assert.Equal(t, verifiedPayment.Currency, subscription.PaidCurrency)
}

func TestBalancePurchaseDoesNotCreateInvoicePaymentSnapshot(t *testing.T) {
	setupInvoiceTest(t)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	plan := SubscriptionPlan{
		Title: "Balance invoice plan", PriceAmount: 12.345678, Currency: "USD", DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, Enabled: true, TotalAmount: 1000, InvoiceEligible: true,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	user := User{Username: "balance-invoice-user", Password: "password", AffCode: "balance-invoice-user", Quota: 10_000_000}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))
	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&subscription).Error)
	assert.False(t, subscription.InvoiceEligible)
	assert.Zero(t, subscription.PaidAmountMicros)
	assert.Empty(t, subscription.PaidCurrency)
	var order SubscriptionOrder
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&order).Error)
	assert.Zero(t, order.ExpectedAmountMicros)
	assert.Zero(t, order.PaidAmountMicros)
	assert.Empty(t, order.ExpectedCurrency)
	assert.Empty(t, order.PaidCurrency)
	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&topUp).Error)
	assert.Equal(t, TopUpSourceSubscription, topUp.Source)
	assert.Empty(t, topUp.Currency)
	assert.InDelta(t, plan.PriceAmount, topUp.Money, 0.000001)
}

func TestCreatePendingSubscriptionOrderAtomicallyReservesPurchaseLimit(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{
		Title: "Single purchase plan", PriceAmount: 9.99, Currency: "USD", DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, Enabled: true, TotalAmount: 1000, MaxPurchasePerUser: 1, StripePriceId: "price_single",
	}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "pending-reservation-user", Password: "password", AffCode: "pending-reservation-user"}
	require.NoError(t, DB.Create(&user).Error)
	orders := []*SubscriptionOrder{
		{
			UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "reservation-order-one",
			PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
			ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
		},
		{
			UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "reservation-order-two",
			PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
			ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
		},
	}
	start := make(chan struct{})
	results := make(chan error, len(orders))
	for _, order := range orders {
		order := order
		go func() {
			<-start
			results <- CreatePendingSubscriptionOrder(order, &plan)
		}()
	}
	close(start)
	successes := 0
	limits := 0
	for range orders {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrSubscriptionPurchaseLimit):
			limits++
		default:
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, limits)

	var reserved SubscriptionOrder
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).First(&reserved).Error)
	require.NoError(t, ExpireSubscriptionOrder(reserved.TradeNo, PaymentProviderStripe))
	retry := &SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "reservation-order-retry",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
		ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
	}
	require.NoError(t, CreatePendingSubscriptionOrder(retry, &plan))
}

func TestDelayedPaidSubscriptionCallbackKeepsItsReservation(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{
		Title: "Delayed callback plan", PriceAmount: 9.99, Currency: "USD", DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, Enabled: true, TotalAmount: 1000, MaxPurchasePerUser: 1, StripePriceId: "price_delayed",
	}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "delayed-callback-user", Password: "password", AffCode: "delayed-callback-user"}
	require.NoError(t, DB.Create(&user).Error)

	staleOrder := &SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "delayed-callback-stale",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
		ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
		CreateTime: common.GetTimestamp() - 90*24*60*60,
	}
	require.NoError(t, CreatePendingSubscriptionOrder(staleOrder, &plan))
	recentOrder := &SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "delayed-callback-recent",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
		ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
	}
	err := CreatePendingSubscriptionOrder(recentOrder, &plan)
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimit)

	payment := SubscriptionPaymentSnapshot{AmountMicros: 9_990_000, Currency: "USD"}
	require.NoError(t, CompleteSubscriptionOrder(staleOrder.TradeNo, `{}`, PaymentProviderStripe, "card", payment))

	var subscriptionCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
	var staleStored SubscriptionOrder
	require.NoError(t, DB.Where("trade_no = ?", staleOrder.TradeNo).First(&staleStored).Error)
	assert.Equal(t, common.TopUpStatusSuccess, staleStored.Status)
	var staleTopUpCount int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", staleOrder.TradeNo).Count(&staleTopUpCount).Error)
	assert.Equal(t, int64(1), staleTopUpCount)
}

func TestSubscriptionOrderFulfillsDeletedPlanFromImmutableSnapshot(t *testing.T) {
	setupInvoiceTest(t)
	allowWalletOverflow := false
	plan := SubscriptionPlan{
		Title: "Original snapshot plan", PriceAmount: 19.99, Currency: "USD",
		DurationUnit: SubscriptionDurationCustom, CustomSeconds: 7200,
		Enabled: true, TotalAmount: 777, QuotaResetPeriod: SubscriptionResetCustom,
		QuotaResetCustomSeconds: 1800, UpgradeGroup: "snapshot-vip", DowngradeGroup: "snapshot-base",
		AllowWalletOverflow: &allowWalletOverflow, InvoiceEligible: true, MaxPurchasePerUser: 2,
		StripePriceId: "price_original", CreemProductId: "prod_original", WaffoPancakeProductId: "waffo_original",
	}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{
		Username: "snapshot-entitlement-user", Password: "password", AffCode: "snapshot-entitlement-user",
		Group: "default", Identity: UserIdentityUniversity,
	}
	require.NoError(t, DB.Create(&user).Error)
	order := SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: plan.PriceAmount, TradeNo: "immutable-plan-order",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, ExpectedAmountMicros: 19_990_000, ExpectedCurrency: "USD",
	}
	require.NoError(t, CreatePendingSubscriptionOrder(&order, &plan))
	require.True(t, DB.Migrator().HasColumn(&SubscriptionOrder{}, "plan_snapshot"))

	var storedOrder SubscriptionOrder
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&storedOrder).Error)
	var snapshot subscriptionPlanSnapshot
	require.NoError(t, common.UnmarshalJsonStr(storedOrder.PlanSnapshot, &snapshot))
	assert.Equal(t, subscriptionPlanSnapshotVersion, snapshot.Version)
	assert.Equal(t, "Original snapshot plan", snapshot.Title)
	assert.Equal(t, int64(7200), snapshot.CustomSeconds)
	assert.Equal(t, int64(777), snapshot.TotalAmount)
	assert.Equal(t, "snapshot-vip", snapshot.UpgradeGroup)
	assert.Equal(t, "snapshot-base", snapshot.DowngradeGroup)
	require.NotNil(t, snapshot.AllowWalletOverflow)
	assert.False(t, *snapshot.AllowWalletOverflow)
	assert.True(t, snapshot.InvoiceEligible)
	assert.Equal(t, 2, snapshot.MaxPurchasePerUser)
	assert.Equal(t, "price_original", snapshot.StripePriceId)
	assert.Equal(t, "prod_original", snapshot.CreemProductId)
	assert.Equal(t, "waffo_original", snapshot.WaffoPancakeProductId)

	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{
		"title": "Mutated plan", "custom_seconds": int64(60), "total_amount": int64(1),
		"upgrade_group": "mutated-group", "invoice_eligible": false,
	}).Error)
	require.NoError(t, DB.Delete(&SubscriptionPlan{}, plan.Id).Error)
	InvalidateSubscriptionPlanCache(plan.Id)

	payment := SubscriptionPaymentSnapshot{AmountMicros: 19_990_000, Currency: "USD"}
	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, `{}`, PaymentProviderStripe, "card", payment))
	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&subscription).Error)
	assert.Equal(t, int64(777), subscription.AmountTotal)
	assert.Equal(t, int64(7200), subscription.EndTime-subscription.StartTime)
	assert.Equal(t, subscription.StartTime, subscription.LastResetTime)
	assert.Equal(t, subscription.StartTime+1800, subscription.NextResetTime)
	assert.Equal(t, "snapshot-vip", subscription.UpgradeGroup)
	assert.Equal(t, "snapshot-base", subscription.DowngradeGroup)
	assert.False(t, subscription.AllowWalletOverflow)
	assert.True(t, subscription.InvoiceEligible)
	var storedUser User
	require.NoError(t, DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, "snapshot-vip", storedUser.Group)
	assert.Equal(t, "Original snapshot plan", subscription.PlanTitleSnapshot)
	planInfo, err := GetSubscriptionPlanInfoByUserSubscriptionId(subscription.Id)
	require.NoError(t, err)
	assert.Equal(t, "Original snapshot plan", planInfo.PlanTitle)

	eligible, err := GetInvoiceEligibleSubscriptions(user.Id, GetInvoiceSettings())
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, "Original snapshot plan", eligible[0].PlanTitle)
	application, err := CreateInvoiceApplication(
		user.Id, GetInvoiceSettings(), invoiceTestInput("Snapshot entitlement invoice"), []int{subscription.Id},
	)
	require.NoError(t, err)
	require.Len(t, application.Items, 1)
	assert.Equal(t, "Original snapshot plan", application.Items[0].PlanTitle)
}

func TestLegacySubscriptionOrderFallsBackToCurrentPlanOnlyWhenSnapshotIsEmpty(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{
		Title: "Legacy plan", PriceAmount: 9.99, Currency: "USD", DurationUnit: SubscriptionDurationCustom,
		CustomSeconds: 60, Enabled: true, TotalAmount: 100,
	}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "legacy-order-user", Password: "password", AffCode: "legacy-order-user"}
	require.NoError(t, DB.Create(&user).Error)
	legacyOrder := SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "legacy-empty-snapshot-order",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
	}
	require.NoError(t, legacyOrder.Insert())
	require.Empty(t, legacyOrder.PlanSnapshot)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).
		Updates(map[string]any{"custom_seconds": int64(120), "total_amount": int64(200)}).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	require.NoError(t, CompleteSubscriptionOrder(
		legacyOrder.TradeNo, `{}`, PaymentProviderStripe, "card",
		SubscriptionPaymentSnapshot{AmountMicros: 9_990_000, Currency: "USD"},
	))
	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&subscription).Error)
	assert.Equal(t, int64(200), subscription.AmountTotal)
	assert.Equal(t, int64(120), subscription.EndTime-subscription.StartTime)

	invalidOrder := SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "invalid-plan-snapshot-order",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
		PlanSnapshot: `{"version":999}`,
	}
	require.NoError(t, invalidOrder.Insert())
	err := CompleteSubscriptionOrder(
		invalidOrder.TradeNo, `{}`, PaymentProviderStripe, "card",
		SubscriptionPaymentSnapshot{AmountMicros: 9_990_000, Currency: "USD"},
	)
	require.ErrorIs(t, err, ErrSubscriptionPlanSnapshotInvalid)

	invalidDurationSnapshot, err := newSubscriptionPlanSnapshot(&plan)
	require.NoError(t, err)
	invalidDurationSnapshot.CustomSeconds = maxSubscriptionDurationSeconds + 1
	invalidDurationJSON, err := common.Marshal(invalidDurationSnapshot)
	require.NoError(t, err)
	invalidDurationOrder := SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "invalid-plan-duration-snapshot-order",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "USD",
		PlanSnapshot: string(invalidDurationJSON),
	}
	require.NoError(t, invalidDurationOrder.Insert())
	err = CompleteSubscriptionOrder(
		invalidDurationOrder.TradeNo, `{}`, PaymentProviderStripe, "card",
		SubscriptionPaymentSnapshot{AmountMicros: 9_990_000, Currency: "USD"},
	)
	require.ErrorIs(t, err, ErrSubscriptionPlanSnapshotInvalid)
}

func TestBalanceSubscriptionPurchaseLimitRollsBackChargeAndHistory(t *testing.T) {
	setupInvoiceTest(t)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	plan := SubscriptionPlan{
		Title: "Balance limit plan", PriceAmount: 1, Currency: "USD", DurationUnit: SubscriptionDurationMonth,
		DurationValue: 1, Enabled: true, TotalAmount: 1000, MaxPurchasePerUser: 1,
	}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "balance-limit-user", Password: "password", AffCode: "balance-limit-user", Quota: 300}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))
	require.ErrorIs(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id), ErrSubscriptionPurchaseLimit)

	var storedUser User
	require.NoError(t, DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, 200, storedUser.Quota)
	var subscriptionCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
	var orderCount int64
	require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&orderCount).Error)
	assert.Equal(t, int64(1), orderCount)
	var topUpCount int64
	require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ? AND source = ?", user.Id, TopUpSourceSubscription).Count(&topUpCount).Error)
	assert.Equal(t, int64(1), topUpCount)
}
