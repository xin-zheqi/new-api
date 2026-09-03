package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestCalcSubscriptionBalanceQuotaUsesWalletQuotaBoundary(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	quota, err := calcSubscriptionBalanceQuota(float64(common.MaxQuota + 1))
	require.NoError(t, err)
	require.Equal(t, common.MaxQuota+1, quota)

	quota, err = calcSubscriptionBalanceQuota(float64(common.MaxWalletQuota) + 1)
	require.Error(t, err)
	require.Zero(t, quota)
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}
