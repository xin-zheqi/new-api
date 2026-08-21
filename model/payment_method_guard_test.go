package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "payment_guard_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:               userID,
		PlanId:               planID,
		Money:                9.99,
		ExpectedAmountMicros: 9_990_000,
		ExpectedCurrency:     "USD",
		TradeNo:              tradeNo,
		PaymentMethod:        paymentProvider,
		PaymentProvider:      paymentProvider,
		Status:               common.TopUpStatusPending,
		CreateTime:           time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func TestNormalizePaymentCurrency_RequiresRecognizedISO4217Code(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{name: "normalizes valid code", input: " usd ", expected: "USD"},
		{name: "accepts Chinese yuan", input: "CNY", expected: "CNY"},
		{name: "rejects unknown alphabetic code", input: "ZZZ", wantErr: true},
		{name: "rejects another unknown code", input: "ABC", wantErr: true},
		{name: "rejects no-currency code", input: "XXX", wantErr: true},
		{name: "rejects test code", input: "XTS", wantErr: true},
		{name: "rejects malformed code", input: "US1", wantErr: true},
		{name: "rejects empty code", input: "", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, err := NormalizePaymentCurrency(testCase.input)
			if testCase.wantErr {
				require.Error(t, err)
				assert.Empty(t, actual)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard", "USD")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestTopUpCurrencyAndTerminalUpdatesAreStable(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 175, 0)

	topUp := &TopUp{
		UserId: 175, Amount: 2, Money: 2, TradeNo: "stripe-terminal-idempotency",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Currency: " usd ", Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
	assert.Equal(t, TopUpSourceRecharge, topUp.Source)
	assert.Equal(t, "USD", topUp.Currency)

	require.NoError(t, UpdatePendingTopUpStatus(topUp.TradeNo, PaymentProviderStripe, common.TopUpStatusFailed))
	require.NoError(t, UpdatePendingTopUpStatus(topUp.TradeNo, PaymentProviderStripe, common.TopUpStatusFailed))
	assert.Equal(t, common.TopUpStatusFailed, getTopUpStatusForPaymentGuardTest(t, topUp.TradeNo))
}

func TestStripeRechargeCallbackIsIdempotentAndPersistsCurrency(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 176, 0)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	topUp := &TopUp{
		UserId: 176, Amount: 2, Money: 2, TradeNo: "stripe-recharge-idempotency",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
	require.NoError(t, Recharge(topUp.TradeNo, "cus_verified", "127.0.0.1", "usd"))
	require.NoError(t, Recharge(topUp.TradeNo, "cus_verified", "127.0.0.1", "USD"))

	var stored TopUp
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
	assert.Equal(t, TopUpSourceRecharge, stored.Source)
	assert.Equal(t, "USD", stored.Currency)
	assert.Equal(t, 200, getUserQuotaForPaymentGuardTest(t, 176))
	var logCount int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 176, LogTypeTopup).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)
}

func TestRechargeEpayValidatesAmountAndIsIdempotent(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 177, 25)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	topUp := &TopUp{
		UserId: 177, Amount: 2, Money: 9.99, TradeNo: "epay-recharge-idempotency",
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		Currency: "CNY", Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	err := RechargeEpay(topUp.TradeNo, "wechat", "9.98", "127.0.0.1")
	require.ErrorIs(t, err, ErrTopUpPaymentMismatch)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, topUp.TradeNo))
	assert.Equal(t, 25, getUserQuotaForPaymentGuardTest(t, topUp.UserId))

	require.NoError(t, RechargeEpay(topUp.TradeNo, "wechat", "9.990000", "127.0.0.1"))
	require.NoError(t, RechargeEpay(topUp.TradeNo, "alipay", "9.99", "127.0.0.1"))

	var stored TopUp
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
	assert.Equal(t, "wechat", stored.PaymentMethod)
	assert.Equal(t, "CNY", stored.Currency)
	assert.Positive(t, stored.CompleteTime)
	assert.Equal(t, 225, getUserQuotaForPaymentGuardTest(t, topUp.UserId))

	var logCount int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", topUp.UserId, LogTypeTopup).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)
}

func TestRechargeEpayRollsBackOrderWhenUserCreditFails(t *testing.T) {
	truncateTables(t)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	topUp := &TopUp{
		UserId: 999, Amount: 4, Money: 4.50, TradeNo: "epay-recharge-rollback",
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		Currency: "CNY", Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
	require.Error(t, RechargeEpay(topUp.TradeNo, "wechat", "4.50", "127.0.0.1"))

	var stored TopUp
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
	assert.Equal(t, "alipay", stored.PaymentMethod)
	assert.Zero(t, stored.CompleteTime)

	var logCount int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", topUp.UserId, LogTypeTopup).Count(&logCount).Error)
	assert.Zero(t, logCount)
}

func TestRechargeCompletionRollsBackWhenUserDoesNotExist(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	testCases := []struct {
		name            string
		paymentProvider string
		complete        func(tradeNo string) error
	}{
		{
			name:            "stripe",
			paymentProvider: PaymentProviderStripe,
			complete: func(tradeNo string) error {
				return Recharge(tradeNo, "cus_missing", "127.0.0.1", "USD")
			},
		},
		{
			name:            "creem",
			paymentProvider: PaymentProviderCreem,
			complete: func(tradeNo string) error {
				return RechargeCreem(tradeNo, "", "", "127.0.0.1", "USD")
			},
		},
		{
			name:            "waffo",
			paymentProvider: PaymentProviderWaffo,
			complete: func(tradeNo string) error {
				return RechargeWaffo(tradeNo, "127.0.0.1")
			},
		},
		{
			name:            "waffo pancake",
			paymentProvider: PaymentProviderWaffoPancake,
			complete: func(tradeNo string) error {
				return RechargeWaffoPancake(tradeNo, "USD")
			},
		},
		{
			name:            "manual completion",
			paymentProvider: PaymentProviderStripe,
			complete: func(tradeNo string) error {
				return ManualCompleteTopUp(tradeNo, "127.0.0.1")
			},
		},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			tradeNo := fmt.Sprintf("missing-user-%d", index)
			topUp := &TopUp{
				UserId:          9000 + index,
				Amount:          2,
				Money:           2,
				TradeNo:         tradeNo,
				PaymentMethod:   testCase.paymentProvider,
				PaymentProvider: testCase.paymentProvider,
				Status:          common.TopUpStatusPending,
				CreateTime:      time.Now().Unix(),
			}
			require.NoError(t, topUp.Insert())

			require.Error(t, testCase.complete(tradeNo))

			var stored TopUp
			require.NoError(t, DB.Where("trade_no = ?", tradeNo).First(&stored).Error)
			assert.Equal(t, common.TopUpStatusPending, stored.Status)
			assert.Zero(t, stored.CompleteTime)

			var logCount int64
			require.NoError(t, LOG_DB.Model(&Log{}).
				Where("user_id = ? AND type = ?", topUp.UserId, LogTypeTopup).
				Count(&logCount).Error)
			assert.Zero(t, logCount)
		})
	}
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	err := CompleteSubscriptionOrder(
		"sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay",
		SubscriptionPaymentSnapshot{AmountMicros: 9_990_000, Currency: "USD"},
	)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestCreateManualTopUp_CreditBalanceOption(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 501, 100)

	recordOnly, err := CreateManualTopUp(ManualTopUpParams{
		UserId:        501,
		Amount:        3,
		Money:         12.34,
		PaymentMethod: "bank_transfer",
		CreateTime:    1710000000,
		CallerIp:      "127.0.0.1",
		CreditBalance: false,
	})
	require.NoError(t, err)
	require.NotNil(t, recordOnly)
	assert.Equal(t, common.TopUpStatusSuccess, recordOnly.Status)
	assert.EqualValues(t, 1710000000, recordOnly.CreateTime)
	assert.EqualValues(t, 1710000000, recordOnly.CompleteTime)
	assert.Equal(t, PaymentProviderManual, recordOnly.PaymentProvider)
	assert.Equal(t, 100, getUserQuotaForPaymentGuardTest(t, 501))

	credited, err := CreateManualTopUp(ManualTopUpParams{
		UserId:        501,
		Amount:        2,
		Money:         8.88,
		PaymentMethod: "corporate_transfer",
		CreateTime:    1710000100,
		CallerIp:      "127.0.0.1",
		CreditBalance: true,
	})
	require.NoError(t, err)
	require.NotNil(t, credited)
	assert.Equal(t, PaymentProviderManual, credited.PaymentProvider)
	assert.Equal(t, 100+int(2*common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 501))

	var manageCount int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 501, LogTypeManage).Count(&manageCount).Error)
	assert.EqualValues(t, 1, manageCount)

	var topupCount int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", 501, LogTypeTopup).Count(&topupCount).Error)
	assert.EqualValues(t, 1, topupCount)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}
