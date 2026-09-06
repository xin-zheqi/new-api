package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPaymentMethodSettingGateAndLabelResolution(t *testing.T) {
	paymentSetting := operation_setting.GetPaymentSetting()
	original := paymentSetting.MethodSettings
	t.Cleanup(func() { paymentSetting.MethodSettings = original })

	paymentSetting.MethodSettings = []operation_setting.PaymentMethodSetting{
		{Type: "stripe", Name: "信用卡", Enabled: boolPointerForTest(false)},
		{Type: "waffo", Name: "国际卡", Enabled: boolPointerForTest(false)},
	}

	assert.False(t, paymentMethodEnabled("stripe"))
	assert.False(t, paymentMethodEnabled("waffo:0"))
	assert.True(t, paymentMethodEnabled("creem"))

	method := map[string]string{"type": " stripe ", "name": "Stripe"}
	require.False(t, applyPaymentMethodSetting(method))
	assert.Equal(t, "stripe", method["type"])

	method = map[string]string{"type": "creem", "name": "Creem"}
	require.True(t, applyPaymentMethodSetting(method))
	assert.Equal(t, "Creem", method["name"])
}

func TestRequirePaymentMethodEnabledRejectsEveryDisabledGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paymentSetting := operation_setting.GetPaymentSetting()
	original := paymentSetting.MethodSettings
	t.Cleanup(func() { paymentSetting.MethodSettings = original })

	for _, paymentType := range []string{
		"alipay",
		model.PaymentMethodStripe,
		model.PaymentMethodCreem,
		model.PaymentMethodWechat,
		model.PaymentMethodWaffo,
		model.PaymentMethodWaffoPancake,
	} {
		disabled := false
		paymentSetting.MethodSettings = []operation_setting.PaymentMethodSetting{{
			Type:    paymentType,
			Name:    paymentType,
			Enabled: &disabled,
		}}

		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/user/pay", nil)

		require.False(t, requirePaymentMethodEnabled(context, paymentType), paymentType)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "停用")
	}

	// A Waffo child identifier is normalized to the same gateway switch.
	disabled := false
	paymentSetting.MethodSettings = []operation_setting.PaymentMethodSetting{{
		Type:    "waffo",
		Name:    "Waffo",
		Enabled: &disabled,
	}}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/pay", nil)
	require.False(t, requirePaymentMethodEnabled(context, " waffo:0 "))
}

func TestGetTopUpInfoAppliesPaymentMethodAvailabilityAndNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)

	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "payment-method-settings.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create([]model.Option{
		{Key: "payment_setting.mall_enabled", Value: "false"},
		{Key: "MallURL", Value: ""},
	}).Error)
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	paymentSetting := operation_setting.GetPaymentSetting()
	originalMethodSettings := paymentSetting.MethodSettings
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	originalMinTopUp := operation_setting.MinTopUp
	originalStripeAPISecret := setting.StripeApiSecret
	originalStripeWebhookSecret := setting.StripeWebhookSecret
	originalStripePriceID := setting.StripePriceId
	t.Cleanup(func() {
		paymentSetting.MethodSettings = originalMethodSettings
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
		operation_setting.MinTopUp = originalMinTopUp
		setting.StripeApiSecret = originalStripeAPISecret
		setting.StripeWebhookSecret = originalStripeWebhookSecret
		setting.StripePriceId = originalStripePriceID
	})

	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"
	operation_setting.PayMethods = []map[string]string{
		{"name": "Alipay", "type": "alipay"},
		{"name": "WeChat Pay", "type": "wxpay"},
	}
	operation_setting.MinTopUp = 5
	setting.StripeApiSecret = "sk_test"
	setting.StripeWebhookSecret = "whsec_test"
	setting.StripePriceId = "price_test"
	paymentSetting.MethodSettings = []operation_setting.PaymentMethodSetting{
		{Type: "alipay", Name: "企业支付宝", Enabled: boolPointerForTest(true)},
		{Type: "wxpay", Name: "易支付微信", Enabled: boolPointerForTest(false)},
		{Type: model.PaymentMethodStripe, Name: "信用卡", Enabled: boolPointerForTest(false)},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "https://api.example.com/api/user/topup/info", nil)
	GetTopUpInfo(context)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			EnableOnlineTopUp     bool                                     `json:"enable_online_topup"`
			EnableStripeTopUp     bool                                     `json:"enable_stripe_topup"`
			PayMethods            []map[string]string                      `json:"pay_methods"`
			PaymentMethodSettings []operation_setting.PaymentMethodSetting `json:"payment_method_settings"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.True(t, response.Data.EnableOnlineTopUp)
	assert.False(t, response.Data.EnableStripeTopUp)
	require.Len(t, response.Data.PayMethods, 1)
	assert.Equal(t, "alipay", response.Data.PayMethods[0]["type"])
	assert.Equal(t, "企业支付宝", response.Data.PayMethods[0]["name"])
	assert.Equal(t, "5", response.Data.PayMethods[0]["min_topup"])
	require.Len(t, response.Data.PaymentMethodSettings, 3)
	assert.Equal(t, "企业支付宝", response.Data.PaymentMethodSettings[0].Name)

	// Disabling the last available Epay entry must also clear the aggregate
	// capability flag; otherwise older clients still offer a dead checkout.
	paymentSetting.MethodSettings = []operation_setting.PaymentMethodSetting{
		{Type: "alipay", Name: "Alipay", Enabled: boolPointerForTest(false)},
		{Type: "wxpay", Name: "WeChat Pay", Enabled: boolPointerForTest(false)},
		{Type: model.PaymentMethodStripe, Name: "Stripe", Enabled: boolPointerForTest(false)},
	}
	recorder = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "https://api.example.com/api/user/topup/info", nil)
	GetTopUpInfo(context)

	response = struct {
		Success bool `json:"success"`
		Data    struct {
			EnableOnlineTopUp     bool                                     `json:"enable_online_topup"`
			EnableStripeTopUp     bool                                     `json:"enable_stripe_topup"`
			PayMethods            []map[string]string                      `json:"pay_methods"`
			PaymentMethodSettings []operation_setting.PaymentMethodSetting `json:"payment_method_settings"`
		} `json:"data"`
	}{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.False(t, response.Data.EnableOnlineTopUp)
	assert.Empty(t, response.Data.PayMethods)
}

func boolPointerForTest(value bool) *bool {
	return &value
}
