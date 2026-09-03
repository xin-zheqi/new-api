package service

import (
	"math"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type quotaBoundaryFunding struct {
	settleCalls int
}

func (*quotaBoundaryFunding) Source() string       { return BillingSourceWallet }
func (*quotaBoundaryFunding) PreConsume(int) error { return nil }
func (f *quotaBoundaryFunding) Settle(int) error {
	f.settleCalls++
	return nil
}
func (*quotaBoundaryFunding) Refund() error { return nil }

// TestAttachQuotaSaturationNestsUnderAdminInfo verifies the saturation marker
// is nested under other.admin_info.quota_saturation so it is admin-only (the
// log formatter strips admin_info for non-admin viewers).
func TestAttachQuotaSaturationNestsUnderAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		UserId:          7,
		OriginModelName: "gpt-image-1",
		QuotaClamp: &common.QuotaClamp{
			Op:       "QuotaFromDecimal",
			Kind:     common.QuotaClampOverflow,
			Original: 1.8e19,
			Clamped:  common.MaxQuota,
		},
	}

	other := map[string]interface{}{"model_price": 0.004}
	attachQuotaSaturation(ctx, relayInfo, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok, "admin_info should be created")
	sat, ok := adminInfo["quota_saturation"].(map[string]interface{})
	require.True(t, ok, "quota_saturation should be nested under admin_info")
	require.Equal(t, "QuotaFromDecimal", sat["op"])
	require.Equal(t, common.QuotaClampOverflow, sat["kind"])
	require.Equal(t, common.MaxQuota, sat["clamped"])
}

func TestCalcViolationFeeQuotaSaturates(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	quota, clamp := calcViolationFeeQuota(1e20, 1)
	require.Equal(t, common.MaxQuota, quota)
	require.NotNil(t, clamp)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}

func TestCalcViolationFeeQuotaRejectsNonFiniteInputs(t *testing.T) {
	tests := []struct {
		name       string
		amount     float64
		groupRatio float64
	}{
		{name: "NaN amount", amount: math.NaN(), groupRatio: 1},
		{name: "infinite amount", amount: math.Inf(1), groupRatio: 1},
		{name: "NaN group ratio", amount: 1, groupRatio: math.NaN()},
		{name: "infinite group ratio", amount: 1, groupRatio: math.Inf(1)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			quota, clamp := calcViolationFeeQuota(tc.amount, tc.groupRatio)
			require.Zero(t, quota)
			require.Nil(t, clamp)
		})
	}
}

func TestCalcOpenRouterCacheCreateTokensDoesNotWrap(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	got := CalcOpenRouterCacheCreateTokens(dto.Usage{Cost: math.Inf(1)}, hosttypes.PriceData{
		ModelRatio:         1,
		CacheCreationRatio: 2,
		CacheRatio:         1,
		CompletionRatio:    1,
	})
	require.Equal(t, -1, got)
}

// TestAttachQuotaSaturationPreservesExistingAdminInfo verifies the marker is
// merged into a pre-existing admin_info map without clobbering it.
func TestAttachQuotaSaturationPreservesExistingAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		QuotaClamp: &common.QuotaClamp{Op: "QuotaFromFloat", Kind: common.QuotaClampUnderflow, Clamped: common.MinQuota},
	}
	other := map[string]interface{}{
		"admin_info": map[string]interface{}{"admin_username": "root"},
	}
	attachQuotaSaturation(ctx, relayInfo, other)

	adminInfo := other["admin_info"].(map[string]interface{})
	require.Equal(t, "root", adminInfo["admin_username"], "existing admin_info fields preserved")
	require.NotNil(t, adminInfo["quota_saturation"])
}

// TestAttachQuotaSaturationNoClampNoMarker verifies the common case (no
// saturation) leaves the log untouched.
func TestAttachQuotaSaturationNoClampNoMarker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{QuotaClamp: nil}
	other := map[string]interface{}{"model_price": 0.004}
	attachQuotaSaturation(ctx, relayInfo, other)

	_, hasAdmin := other["admin_info"]
	require.False(t, hasAdmin, "no admin_info should be added when there is no clamp")
}

func TestPreConsumeBillingRejectsSaturatedQuotaBeforeDeduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		QuotaClamp: &common.QuotaClamp{
			Op:       "QuotaFromFloat",
			Kind:     common.QuotaClampOverflow,
			Original: 1e30,
			Clamped:  common.MaxQuota,
		},
	}

	apiErr := PreConsumeBilling(c, common.MaxQuota, info)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeModelPriceError, apiErr.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Same(t, info.QuotaClamp, apiErr.Err)
	var clamp *common.QuotaClamp
	require.ErrorAs(t, apiErr, &clamp)
	require.Same(t, info.QuotaClamp, clamp)
	require.Nil(t, info.Billing)
}

func TestPreConsumeBillingRejectsNegativeQuotaBeforeDeduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{}

	apiErr := PreConsumeBilling(c, -1, info)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeModelPriceError, apiErr.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Nil(t, info.Billing)
}

func TestPreConsumeBillingRejectsQuotaAboveSingleRequestLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{}

	apiErr := PreConsumeBilling(c, common.MaxQuota+1, info)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeModelPriceError, apiErr.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Nil(t, info.Billing)
}

func TestBillingSessionRejectsInvalidSettlementBeforeFundingMutation(t *testing.T) {
	for _, actualQuota := range []int{-1, common.MaxQuota + 1} {
		funding := &quotaBoundaryFunding{}
		session := &BillingSession{
			relayInfo:        &relaycommon.RelayInfo{IsPlayground: true},
			funding:          funding,
			preConsumedQuota: 100,
		}

		require.Error(t, session.Settle(actualQuota))
		require.Zero(t, funding.settleCalls)
		require.False(t, session.settled)
	}
}

func TestBillingSessionReserveRejectsInvalidTargetBeforeFundingMutation(t *testing.T) {
	funding := &quotaBoundaryFunding{}
	session := &BillingSession{
		relayInfo:        &relaycommon.RelayInfo{IsPlayground: true},
		funding:          funding,
		preConsumedQuota: 100,
	}

	require.Error(t, session.Reserve(common.MaxQuota+1))
	require.Equal(t, 100, session.preConsumedQuota)
	require.Zero(t, funding.settleCalls)
}
