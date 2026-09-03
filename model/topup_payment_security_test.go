package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopUpPaymentSnapshotRejectsMismatchAndQuotaOverflow(t *testing.T) {
	const userID = 981001
	const tradeNo = "topup-payment-security"
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
		DB.Where("id = ?", userID).Delete(&User{})
	})

	user := &User{Id: userID, Username: "topup-payment-security", AffCode: "topup-payment-security", Quota: common.MaxWalletQuota - 10}
	require.NoError(t, DB.Create(user).Error)
	topUp := &TopUp{
		UserId: userID, Amount: 1, Money: 2, TradeNo: tradeNo,
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Currency: "USD", ExpectedAmountMicros: 2_000_000, ExpectedCurrency: "USD",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, DB.Create(topUp).Error)

	err := RechargeWithPayment(tradeNo, "cus_security", "127.0.0.1", SubscriptionPaymentSnapshot{
		AmountMicros: 3_000_000, Currency: "USD",
	})
	require.ErrorIs(t, err, ErrTopUpPaymentMismatch)
	assert.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo(tradeNo).Status)
	assert.Equal(t, common.MaxWalletQuota-10, getTopUpSecurityQuota(t, userID))

	err = RechargeWithPayment(tradeNo, "cus_security", "127.0.0.1", SubscriptionPaymentSnapshot{
		AmountMicros: 2_000_000, Currency: "USD",
	})
	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	assert.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo(tradeNo).Status)
	assert.Equal(t, common.MaxWalletQuota-10, getTopUpSecurityQuota(t, userID))
}

func TestLegacyTopUpFloatAmountIsPinnedAfterVerifiedCallback(t *testing.T) {
	const userID = 981002
	const tradeNo = "topup-payment-legacy-float"
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
		DB.Where("id = ?", userID).Delete(&User{})
	})
	require.NoError(t, DB.Create(&User{Id: userID, Username: "topup-payment-legacy-float", AffCode: "topup-payment-legacy-float"}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: userID, Amount: 1, Money: 1.005, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		Currency: "CNY", Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}).Error)

	// Legacy checkout creation used strconv.FormatFloat(..., 'f', 2, 64),
	// which serializes this binary float as 1.00 rather than decimal-rounding
	// it to 1.01. The migration must preserve that historical amount.
	err := RechargeEpayWithPayment(tradeNo, "alipay", "1.00", "127.0.0.1")
	require.NoError(t, err)
	stored := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, int64(1_000_000), stored.ExpectedAmountMicros)
	assert.Equal(t, "CNY", stored.ExpectedCurrency)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
	assert.Equal(t, 100, getTopUpSecurityQuota(t, userID))
}

func TestLegacyCreemTopUpPreservesThreeDecimalCurrencyAmount(t *testing.T) {
	topUp := &TopUp{PaymentProvider: PaymentProviderCreem, Money: 1.234, Currency: "KWD"}
	paid := SubscriptionPaymentSnapshot{AmountMicros: 1_234_000, Currency: "KWD"}

	expected, err := expectedTopUpPayment(topUp, paid)

	require.NoError(t, err)
	assert.Equal(t, paid, expected)
}

func TestTopUpCallbackUsesFrozenCreditedQuotaAfterUnitSettingChanges(t *testing.T) {
	const userID = 981004
	const tradeNo = "topup-frozen-credited-quota"
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1_000
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
		DB.Where("id = ?", userID).Delete(&User{})
	})

	require.NoError(t, DB.Create(&User{Id: userID, Username: "topup-frozen-quota", AffCode: "topup-frozen-quota"}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: userID, Amount: 2, Money: 2, CreditedQuota: 150, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		Currency: "CNY", ExpectedAmountMicros: 2_000_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, RechargeEpayWithPayment(tradeNo, "alipay", "2.00", "127.0.0.1"))
	assert.Equal(t, 150, getTopUpSecurityQuota(t, userID))
	assert.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo(tradeNo).Status)
}

func TestStripeTopUpAcceptsPromotionAmountAndStoresActualPayment(t *testing.T) {
	const userID = 981005
	const tradeNo = "topup-stripe-promotion"
	t.Cleanup(func() {
		DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
		DB.Where("id = ?", userID).Delete(&User{})
	})

	require.NoError(t, DB.Create(&User{Id: userID, Username: tradeNo, AffCode: tradeNo}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: userID, Amount: 2, Money: 2, CreditedQuota: 137, TradeNo: tradeNo,
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		Currency: "USD", ExpectedAmountMicros: 2_000_000, ExpectedCurrency: "USD",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}).Error)

	require.ErrorIs(t, RechargeWithPayment(tradeNo, "cus_promotion", "127.0.0.1", SubscriptionPaymentSnapshot{
		AmountMicros: 2_000_001, Currency: "USD",
	}), ErrTopUpPaymentMismatch)
	require.NoError(t, RechargeWithPayment(tradeNo, "cus_promotion", "127.0.0.1", SubscriptionPaymentSnapshot{
		AmountMicros: 1_500_000, Currency: "USD",
	}))

	stored := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
	assert.Equal(t, int64(1_500_000), stored.ExpectedAmountMicros)
	assert.Equal(t, "USD", stored.ExpectedCurrency)
	assert.Equal(t, 1.5, stored.Money)
	assert.Equal(t, 137, getTopUpSecurityQuota(t, userID))

	invoiceAmount, invoiceCurrency, eligible := invoiceTopUpPaymentSnapshot(*stored)
	assert.True(t, eligible)
	assert.Equal(t, int64(1_500_000), invoiceAmount)
	assert.Equal(t, "USD", invoiceCurrency)
}

func TestResolveTopUpCreditedQuotaKeepsLegacyProviderSemantics(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	tests := []struct {
		name   string
		topUp  TopUp
		wanted int
		pinned bool
	}{
		{name: "frozen snapshot", topUp: TopUp{PaymentProvider: PaymentProviderEpay, Amount: 99, CreditedQuota: 123}, wanted: 123, pinned: true},
		{name: "legacy amount provider", topUp: TopUp{PaymentProvider: PaymentProviderEpay, Amount: 2}, wanted: 200, pinned: true},
		{name: "legacy Stripe", topUp: TopUp{PaymentProvider: PaymentProviderStripe, Money: 2.5}, wanted: 250, pinned: true},
		{name: "legacy Creem", topUp: TopUp{PaymentProvider: PaymentProviderCreem, Amount: 250}, wanted: 250, pinned: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			quota, err := resolveTopUpCreditedQuota(&tc.topUp)
			require.NoError(t, err)
			assert.Equal(t, tc.wanted, quota)
			if tc.pinned {
				assert.Equal(t, int64(tc.wanted), tc.topUp.CreditedQuota)
			}
		})
	}
}

func TestTopUpIdempotentCallbackIgnoresLaterPaymentMismatch(t *testing.T) {
	const userID = 981003
	const tradeNo = "topup-payment-idempotent"
	t.Cleanup(func() {
		DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
		DB.Where("id = ?", userID).Delete(&User{})
	})
	require.NoError(t, DB.Create(&User{Id: userID, Username: "topup-payment-idempotent", AffCode: "topup-payment-idempotent"}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: userID, Amount: 1, Money: 2, TradeNo: tradeNo,
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		ExpectedAmountMicros: 2_000_000, ExpectedCurrency: "USD", Currency: "USD",
		Status: common.TopUpStatusSuccess, CreateTime: time.Now().Unix(),
	}).Error)

	err := RechargeWithPayment(tradeNo, "cus_security", "127.0.0.1", SubscriptionPaymentSnapshot{
		AmountMicros: 1_000_000, Currency: "EUR",
	})
	assert.NoError(t, err)
}

func TestEveryTopUpProviderUsesFrozenQuotaExactlyOnce(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 999_999
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	tests := []struct {
		name              string
		provider          string
		paymentMethod     string
		currency          string
		settle            func(string) error
		expectedUserField func(*testing.T, User)
	}{
		{
			name: "Epay", provider: PaymentProviderEpay, paymentMethod: "alipay", currency: "CNY",
			settle: func(tradeNo string) error {
				return RechargeEpayWithPayment(tradeNo, "alipay", "2.00", "127.0.0.1")
			},
		},
		{
			name: "Stripe", provider: PaymentProviderStripe, paymentMethod: PaymentMethodStripe, currency: "USD",
			settle: func(tradeNo string) error {
				return RechargeWithPayment(tradeNo, "cus_frozen", "127.0.0.1", SubscriptionPaymentSnapshot{AmountMicros: 2_000_000, Currency: "USD"})
			},
			expectedUserField: func(t *testing.T, user User) {
				assert.Equal(t, "cus_frozen", user.StripeCustomer)
			},
		},
		{
			name: "WeChat", provider: PaymentProviderWechat, paymentMethod: PaymentMethodWechat, currency: "CNY",
			settle: func(tradeNo string) error {
				return RechargeWechatPay(tradeNo, 200, "CNY", "127.0.0.1")
			},
		},
		{
			name: "Creem", provider: PaymentProviderCreem, paymentMethod: PaymentMethodCreem, currency: "USD",
			settle: func(tradeNo string) error {
				return RechargeCreemWithPayment(tradeNo, "paid@example.com", "Paid User", "127.0.0.1", SubscriptionPaymentSnapshot{AmountMicros: 2_000_000, Currency: "USD"})
			},
			expectedUserField: func(t *testing.T, user User) {
				assert.Equal(t, "paid@example.com", user.Email)
			},
		},
		{
			name: "Waffo", provider: PaymentProviderWaffo, paymentMethod: PaymentMethodWaffo, currency: "USD",
			settle: func(tradeNo string) error {
				return RechargeWaffoWithPayment(tradeNo, "127.0.0.1", SubscriptionPaymentSnapshot{AmountMicros: 2_000_000, Currency: "USD"})
			},
		},
		{
			name: "Waffo Pancake", provider: PaymentProviderWaffoPancake, paymentMethod: PaymentMethodWaffoPancake, currency: "USD",
			settle: func(tradeNo string) error {
				return RechargeWaffoPancakeWithPayment(tradeNo, SubscriptionPaymentSnapshot{AmountMicros: 2_000_000, Currency: "USD"})
			},
		},
	}

	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			userID := 982000 + index
			tradeNo := fmt.Sprintf("topup-frozen-provider-%d", index)
			t.Cleanup(func() {
				DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
				DB.Where("id = ?", userID).Delete(&User{})
			})
			require.NoError(t, DB.Create(&User{
				Id: userID, Username: tradeNo, AffCode: tradeNo,
			}).Error)
			require.NoError(t, DB.Create(&TopUp{
				UserId: userID, Amount: 2, Money: 2, CreditedQuota: 137,
				TradeNo: tradeNo, PaymentMethod: tc.paymentMethod, PaymentProvider: tc.provider,
				Currency: tc.currency, ExpectedAmountMicros: 2_000_000, ExpectedCurrency: tc.currency,
				Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
			}).Error)

			require.NoError(t, tc.settle(tradeNo))
			require.NoError(t, tc.settle(tradeNo))

			var user User
			require.NoError(t, DB.First(&user, userID).Error)
			assert.Equal(t, 137, user.Quota)
			if tc.expectedUserField != nil {
				tc.expectedUserField(t, user)
			}
			stored := GetTopUpByTradeNo(tradeNo)
			require.NotNil(t, stored)
			assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
			assert.Equal(t, int64(137), stored.CreditedQuota)
		})
	}
}

func getTopUpSecurityQuota(t *testing.T, userID int) int {
	t.Helper()
	var user User
	err := DB.Select("quota").Where("id = ?", userID).First(&user).Error
	require.NoError(t, err)
	return user.Quota
}
