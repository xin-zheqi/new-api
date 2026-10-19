package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeriodicTokenQuotaResetsAndReservesAtomically(t *testing.T) {
	truncateTables(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })

	token := Token{
		UserId:           1,
		Key:              "periodic-" + common.GetRandomString(8),
		Name:             "periodic",
		Status:           common.TokenStatusEnabled,
		ExpiredTime:      -1,
		UnlimitedQuota:   true,
		QuotaPeriod:      QuotaPeriodDay,
		QuotaPeriodLimit: 100,
		QuotaPeriodUsed:  90,
		QuotaPeriodStart: QuotaPeriodStart(QuotaPeriodDay, time.Now().AddDate(0, 0, -1)),
	}
	require.NoError(t, token.Insert())

	reserved, err := TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 25)
	require.NoError(t, err)
	require.True(t, reserved, "an expired period must reset before enforcing the new limit")

	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	require.Equal(t, 25, stored.QuotaPeriodUsed)
	require.Equal(t, QuotaPeriodStart(QuotaPeriodDay, time.Now()), stored.QuotaPeriodStart)

	reserved, err = TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 80)
	require.NoError(t, err)
	require.False(t, reserved, "a reservation over the current period limit must be rejected")
	require.NoError(t, AdjustTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, -10))
	require.NoError(t, DB.First(&stored, token.Id).Error)
	require.Equal(t, 15, stored.QuotaPeriodUsed)
}

func TestQuotaPeriodStartUsesCalendarBoundaries(t *testing.T) {
	day := time.Date(2026, 2, 28, 23, 59, 0, 0, time.Local)
	require.Equal(t,
		time.Date(2026, 2, 28, 0, 0, 0, 0, time.Local).Unix(),
		QuotaPeriodStart(QuotaPeriodDay, day),
	)
	require.Equal(t,
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local).Unix(),
		QuotaPeriodStart(QuotaPeriodMonth, day),
	)
	require.Equal(t,
		time.Date(2026, 2, 23, 0, 0, 0, 0, time.Local).Unix(),
		QuotaPeriodStart(QuotaPeriodWeek, day),
	)
}

func TestPeriodicTokenQuotaRejectsEvenZeroEstimateAfterLimitIsExhausted(t *testing.T) {
	truncateTables(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })

	token := Token{
		UserId:           1,
		Key:              "exhausted-period-" + common.GetRandomString(8),
		Name:             "exhausted-period",
		Status:           common.TokenStatusEnabled,
		ExpiredTime:      -1,
		UnlimitedQuota:   true,
		QuotaPeriod:      QuotaPeriodDay,
		QuotaPeriodLimit: 100,
		QuotaPeriodUsed:  90,
		QuotaPeriodStart: QuotaPeriodStart(QuotaPeriodDay, time.Now()),
	}
	require.NoError(t, token.Insert())
	reserved, err := TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 10)
	require.NoError(t, err)
	require.True(t, reserved)

	reserved, err = TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 0)
	require.NoError(t, err)
	assert.False(t, reserved, "an exhausted key must reject even a zero-estimate request")
	reserved, err = TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 1)
	require.NoError(t, err)
	assert.False(t, reserved)
}

func TestPeriodicTokenSettlementCapsUsageWhenActualCostExceedsReservation(t *testing.T) {
	truncateTables(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })

	token := Token{
		UserId: 1, Key: "settlement-cap-" + common.GetRandomString(8),
		Name: "settlement-cap", Status: common.TokenStatusEnabled, ExpiredTime: -1,
		UnlimitedQuota: true, QuotaPeriod: QuotaPeriodDay,
		QuotaPeriodLimit: 100, QuotaPeriodStart: QuotaPeriodStart(QuotaPeriodDay, time.Now()),
	}
	require.NoError(t, token.Insert())
	reserved, err := TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 80)
	require.NoError(t, err)
	require.True(t, reserved)

	// The upstream's actual bill is only known after the response. The request
	// must not fail after succeeding upstream, but the key must be exhausted.
	require.NoError(t, AdjustTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 40))
	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, 100, stored.QuotaPeriodUsed)
	reserved, err = TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 0)
	require.NoError(t, err)
	assert.False(t, reserved)
}
