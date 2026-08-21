package model

import (
	"fmt"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertTopUpForQueryTest(t *testing.T, userID int, tradeNo string, createTime int64) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          10,
		Money:           2.5,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodManual,
		PaymentProvider: PaymentProviderManual,
		CreateTime:      createTime,
		CompleteTime:    createTime,
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, topUp.Insert())
}

func insertUserForTopUpQueryTest(t *testing.T, id int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: fmt.Sprintf("topup_query_user_%d", id),
		Status:   common.UserStatusEnabled,
		AffCode:  "topup_query_" + common.GetRandomString(8),
	}
	require.NoError(t, DB.Create(user).Error)
}

func TestGetUserTopUps_AllHistoryVisibleWithoutWindowLimit(t *testing.T) {
	truncateTables(t)

	insertUserForTopUpQueryTest(t, 9001)
	insertTopUpForQueryTest(t, 9001, "q-old-user", 1700000000)
	insertTopUpForQueryTest(t, 9001, "q-new-user", 1800000000)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	topups, total, err := GetUserTopUps(9001, pageInfo, TopUpQueryFilter{})
	require.NoError(t, err)
	require.Len(t, topups, 2)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, "q-new-user", topups[0].TradeNo)
	assert.Equal(t, "q-old-user", topups[1].TradeNo)
}

func TestGetUserTopUps_RespectsDateRange(t *testing.T) {
	truncateTables(t)

	insertUserForTopUpQueryTest(t, 9002)
	insertTopUpForQueryTest(t, 9002, "q-range-old", 1700000000)
	insertTopUpForQueryTest(t, 9002, "q-range-mid", 1750000000)
	insertTopUpForQueryTest(t, 9002, "q-range-new", 1800000000)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	topups, total, err := GetUserTopUps(9002, pageInfo, TopUpQueryFilter{
		StartTime: 1740000000,
		EndTime:   1790000000,
	})
	require.NoError(t, err)
	require.Len(t, topups, 1)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, "q-range-mid", topups[0].TradeNo)
}

func TestSearchAndAdminTopUps_UseSameDateRangeFilter(t *testing.T) {
	truncateTables(t)

	insertUserForTopUpQueryTest(t, 9003)
	insertUserForTopUpQueryTest(t, 9004)
	insertTopUpForQueryTest(t, 9003, "q-user-a-old", 1700000000)
	insertTopUpForQueryTest(t, 9003, "q-user-a-new", 1800000000)
	insertTopUpForQueryTest(t, 9004, "q-user-b-old", 1700000000)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	userTopups, userTotal, err := SearchUserTopUps(9003, "q-user-a-old", pageInfo, TopUpQueryFilter{
		StartTime: 1690000000,
		EndTime:   1710000000,
	})
	require.NoError(t, err)
	require.Len(t, userTopups, 1)
	assert.EqualValues(t, 1, userTotal)
	assert.Equal(t, "q-user-a-old", userTopups[0].TradeNo)

	adminTopups, adminTotal, err := SearchAllTopUps("%q-user%", pageInfo, TopUpQueryFilter{
		StartTime: 1690000000,
		EndTime:   1710000000,
	})
	require.NoError(t, err)
	require.Len(t, adminTopups, 2)
	assert.EqualValues(t, 2, adminTotal)
	assert.ElementsMatch(t, []string{"q-user-a-old", "q-user-b-old"},
		[]string{adminTopups[0].TradeNo, adminTopups[1].TradeNo})

	allTopups, allTotal, err := GetAllTopUps(pageInfo, TopUpQueryFilter{})
	require.NoError(t, err)
	require.Len(t, allTopups, 3)
	assert.EqualValues(t, 3, allTotal)
}

func TestCreateManualTopUpSaturatesOversizedQuotaWithoutWrappingNegative(t *testing.T) {
	truncateTables(t)
	insertUserForTopUpQueryTest(t, 9005)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 2
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	topUp, err := CreateManualTopUp(ManualTopUpParams{
		UserId: 9005, Amount: math.MaxInt64, Money: 1, PaymentMethod: PaymentMethodManual,
		CreateTime: common.GetTimestamp(), CreditBalance: true,
	})
	require.NoError(t, err)
	require.NotNil(t, topUp)

	var user User
	require.NoError(t, DB.First(&user, 9005).Error)
	assert.Equal(t, common.MaxQuota, user.Quota)
	assert.Positive(t, user.Quota)
}
