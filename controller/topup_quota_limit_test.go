package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTopUpQuotaValidation(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	testCases := []struct {
		name        string
		displayType string
		amount      int64
		wantQuota   int
		wantErr     bool
	}{
		{
			name:        "currency amount below limit",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      4294,
			wantQuota:   2_147_000_000,
		},
		{
			name:        "currency amount above limit",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      4295,
			wantQuota:   2_147_500_000,
		},
		{
			name:        "token amount uses complete units",
			displayType: operation_setting.QuotaDisplayTypeTokens,
			amount:      2_147_500_000,
			wantQuota:   2_147_500_000,
		},
		{
			name:        "token amount above legacy int32 range",
			displayType: operation_setting.QuotaDisplayTypeTokens,
			amount:      4_294_500_000,
			wantQuota:   4_294_500_000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.displayType
			quota, err := getTopUpQuota(tc.amount)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantQuota, quota)
		})
	}
}

func TestValidateTopUpAmountRejectsPartialTokenUnit(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500_000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	valid, message := validateTopUpAmount(1_250_000, 500_000)
	assert.False(t, valid)
	assert.Equal(t, "充值数量必须为完整额度单位", message)

	valid, message = validateTopUpAmount(1_500_000, 500_000)
	assert.True(t, valid)
	assert.Empty(t, message)

	_, err := getTopUpQuota(1_250_000)
	require.EqualError(t, err, "充值数量必须为完整额度单位")
}

func TestValidateTopUpQuotaReturnsMaximumAmount(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	maxAmount := decimal.NewFromInt(common.MaxWalletQuota).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		Floor().IntPart()

	_, err := validateTopUpQuota(maxAmount)
	require.NoError(t, err)
	_, err = validateTopUpQuota(maxAmount + 1)
	require.EqualError(t, err, fmt.Sprintf("单笔充值数量不能大于 %d", maxAmount))
}

func TestTokenTopUpMaximumIsLargestCompleteQuotaUnit(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500_000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	wantMax := decimal.NewFromInt(common.MaxWalletQuota).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		Floor().
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		IntPart()
	assert.Equal(t, wantMax, getMaxTopUpAmount())

	quota, err := getTopUpQuota(wantMax)
	require.NoError(t, err)
	assert.Equal(t, int(wantMax), quota)

	_, err = getTopUpQuota(wantMax + int64(common.QuotaPerUnit))
	require.Error(t, err)
}

func TestRequestAmountRejectsAmountAboveRequestLimit(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldPaymentSetting := *operation_setting.GetPaymentSetting()
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		*operation_setting.GetPaymentSetting() = oldPaymentSetting
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/amount",
		strings.NewReader(`{"amount":10001}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"message":"error","data":"充值数量不能大于 10000"}`, recorder.Body.String())
}

func TestRequestAmountRejectsTopUpThatWouldOverflowWallet(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldPaymentSetting := *operation_setting.GetPaymentSetting()
	oldDB := model.DB
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		*operation_setting.GetPaymentSetting() = oldPaymentSetting
		model.DB = oldDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	require.NoError(t, model.DB.Create(&model.User{
		Id:       42,
		Username: "topup_capacity_user",
		Quota:    common.MaxWalletQuota - 100_000,
		Status:   common.UserStatusEnabled,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 42)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/amount",
		strings.NewReader(`{"amount":1}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"message":"error","data":"top-up quota limit exceeded"}`, recorder.Body.String())
}

func TestValidateCreditedQuotaRejectsOverflow(t *testing.T) {
	_, err := validateCreditedQuota(decimal.NewFromInt(int64(common.MaxWalletQuota / 2)))
	require.NoError(t, err)
	_, err = validateCreditedQuota(decimal.Zero)
	require.EqualError(t, err, "充值额度必须大于 0")
	_, err = validateCreditedQuota(decimal.NewFromInt(common.MaxWalletQuota + 1))
	require.EqualError(
		t,
		err,
		"充值额度超出系统可表示范围",
	)
}

func TestStripeTopUpAmountsUseConsistentDisplayAndSettlementUnits(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldTopupGroupRatio := common.TopupGroupRatio2JSONString()
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":2}`))
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(oldTopupGroupRatio))
	})

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	amounts, err := calculateStripeTopUpAmounts(2147, "vip")
	require.NoError(t, err)
	assert.Equal(t, int64(2147), amounts.checkoutQuantity)
	assert.True(t, decimal.NewFromInt(2_147_000_000).Equal(amounts.creditedQuota))
	_, err = validateCreditedQuota(amounts.creditedQuota)
	require.NoError(t, err)

	amounts, err = calculateStripeTopUpAmounts(int64(common.MaxWalletQuota), "vip")
	require.Error(t, err)

	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"free":0}`))
	amounts, err = calculateStripeTopUpAmounts(1, "free")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(500000).Equal(amounts.creditedQuota))

	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":2}`))
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	amounts, err = calculateStripeTopUpAmounts(1_500_000, "vip")
	require.NoError(t, err)
	assert.Equal(t, int64(3), amounts.checkoutQuantity)
	assert.True(t, decimal.NewFromInt(3_000_000).Equal(amounts.creditedQuota))

	amounts, err = calculateStripeTopUpAmounts(5_000_000_000, "vip")
	require.NoError(t, err)
	assert.Equal(t, maxTopUpDisplayAmount, amounts.checkoutQuantity)
}

func TestStripePreviewMatchesStaticCheckoutQuantity(t *testing.T) {
	originalUnitPrice := setting.StripeUnitPrice
	originalDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()
	setting.StripeUnitPrice = 8
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{10: 0.5}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":1.2}`))
	t.Cleanup(func() {
		setting.StripeUnitPrice = originalUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	assert.Equal(t, float64(80), getStripePayMoney(10))
}
