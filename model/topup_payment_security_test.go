package model

import (
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

	user := &User{Id: userID, Username: "topup-payment-security", AffCode: "topup-payment-security", Quota: common.MaxQuota - 10}
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
	assert.Equal(t, common.MaxQuota-10, getTopUpSecurityQuota(t, userID))

	err = RechargeWithPayment(tradeNo, "cus_security", "127.0.0.1", SubscriptionPaymentSnapshot{
		AmountMicros: 2_000_000, Currency: "USD",
	})
	require.ErrorIs(t, err, ErrTopUpQuotaOverflow)
	assert.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo(tradeNo).Status)
	assert.Equal(t, common.MaxQuota-10, getTopUpSecurityQuota(t, userID))
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
	err := RechargeEpay(tradeNo, "alipay", "1.00", "127.0.0.1")
	require.NoError(t, err)
	stored := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, int64(1_000_000), stored.ExpectedAmountMicros)
	assert.Equal(t, "CNY", stored.ExpectedCurrency)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
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

func getTopUpSecurityQuota(t *testing.T, userID int) int {
	t.Helper()
	var user User
	err := DB.Select("quota").Where("id = ?", userID).First(&user).Error
	require.NoError(t, err)
	return user.Quota
}
