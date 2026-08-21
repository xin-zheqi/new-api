package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestCalcSubscriptionBalanceQuotaRejectsSaturatedConversion(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = float64(common.MaxQuota)
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	quota, err := calcSubscriptionBalanceQuota(9999)
	require.Error(t, err)
	require.Zero(t, quota)
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}
