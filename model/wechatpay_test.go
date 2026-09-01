package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListExpiredPendingWechatOrdersIsProviderScopedAndCursorStable(t *testing.T) {
	truncateTables(t)
	now := int64(2_000)
	require.NoError(t, DB.Create(&TopUp{
		TradeNo: "W-oldest", PaymentProvider: PaymentProviderWechat,
		Status: common.TopUpStatusPending, PaymentExpireTime: 1_000,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		TradeNo: "S-middle", PaymentProvider: PaymentProviderWechat,
		Status: common.TopUpStatusPending, PaymentExpireTime: 1_100,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		TradeNo: "W-latest", PaymentProvider: PaymentProviderWechat,
		Status: common.TopUpStatusPending, PaymentExpireTime: 1_200,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		TradeNo: "zz-stripe-expired", PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, PaymentExpireTime: 1_100,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		TradeNo: "W-future", PaymentProvider: PaymentProviderWechat,
		Status: common.TopUpStatusPending, PaymentExpireTime: now + 1,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		TradeNo: "W-complete", PaymentProvider: PaymentProviderWechat,
		Status: common.TopUpStatusSuccess, PaymentExpireTime: 800,
	}).Error)

	firstPage, err := ListExpiredPendingWechatOrders(now, 0, "", 2)
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	assert.Equal(t, []string{"W-oldest", "S-middle"}, []string{firstPage[0].TradeNo, firstPage[1].TradeNo})

	secondPage, err := ListExpiredPendingWechatOrders(
		now, firstPage[1].PaymentExpireTime, firstPage[1].TradeNo, 2,
	)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	assert.Equal(t, "W-latest", secondPage[0].TradeNo)
}

func TestListExpiredPendingWechatOrdersRejectsInvalidCursor(t *testing.T) {
	_, err := ListExpiredPendingWechatOrders(2_000, 0, "unexpected", 20)
	require.Error(t, err)
}
