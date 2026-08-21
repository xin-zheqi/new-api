package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSafeMallURLAllowsOnlyHTTPSWithoutCredentials(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		applicationHost string
		expected        string
	}{
		{name: "cross-origin https", value: "https://shop.example.com/buy", applicationHost: "api.example.com", expected: "https://shop.example.com/buy"},
		{name: "cross-origin IDN", value: "https://商店.测试/buy", applicationHost: "例子.测试", expected: "https://xn--czrs0t.xn--0zwm56d/buy"},
		{name: "trimmed and normalized scheme", value: " HTTPS://shop.example.com/buy ", applicationHost: "api.example.com", expected: "https://shop.example.com/buy"},
		{name: "same host", value: "https://api.example.com/shop", applicationHost: "api.example.com"},
		{name: "same host with application port", value: "https://api.example.com/shop", applicationHost: "api.example.com:3000"},
		{name: "same host trailing dot", value: "https://API.EXAMPLE.COM./shop", applicationHost: "api.example.com"},
		{name: "same host multiple trailing dots", value: "https://API.EXAMPLE.COM../shop", applicationHost: "api.example.com."},
		{name: "same IDN host", value: "https://例子.测试/shop", applicationHost: "xn--fsqu00a.xn--0zwm56d"},
		{name: "same IDN host reversed", value: "https://xn--fsqu00a.xn--0zwm56d/shop", applicationHost: "例子.测试"},
		{name: "empty canonical host", value: "https://.../shop", applicationHost: "api.example.com"},
		{name: "http", value: "http://shop.example.com/buy", applicationHost: "api.example.com"},
		{name: "javascript", value: "javascript:alert(1)", applicationHost: "api.example.com"},
		{name: "credentials", value: "https://user:password@shop.example.com/buy", applicationHost: "api.example.com"},
		{name: "relative", value: "/buy", applicationHost: "api.example.com"},
		{name: "opaque", value: "https:shop.example.com/buy", applicationHost: "api.example.com"},
		{name: "invalid port", value: "https://shop.example.com:bad/buy", applicationHost: "api.example.com"},
		{name: "too long", value: "https://shop.example.com/" + strings.Repeat("a", maxMallURLLength), applicationHost: "api.example.com"},
		{name: "missing application host", value: "https://shop.example.com/buy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, safeMallURL(test.value, test.applicationHost))
		})
	}
}

func TestTopUpAmountValidationRejectsUnboundedRequestsInEveryDisplayMode(t *testing.T) {
	general := operation_setting.GetGeneralSetting()
	originalDisplayType := general.QuotaDisplayType
	originalQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() {
		general.QuotaDisplayType = originalDisplayType
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	general.QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	valid, _ := validateTopUpAmount(10_000, 1)
	assert.True(t, valid)
	valid, _ = validateTopUpAmount(10_001, 1)
	assert.False(t, valid)
	valid, _ = validateTopUpAmount(int64(^uint64(0)>>1), 1)
	assert.False(t, valid)

	general.QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	common.QuotaPerUnit = 100
	assert.EqualValues(t, 100, getMinTopup())
	valid, _ = validateTopUpAmount(1_000_000, getMinTopup())
	assert.True(t, valid)
	valid, _ = validateTopUpAmount(1_000_001, getMinTopup())
	assert.False(t, valid)
}

func TestCreemTopUpRequiresVerifiableWebhookConfiguration(t *testing.T) {
	payment := operation_setting.GetPaymentSetting()
	originalPayment := *payment
	originalApiKey := setting.CreemApiKey
	originalProducts := setting.CreemProducts
	originalSecret := setting.CreemWebhookSecret
	t.Cleanup(func() {
		*payment = originalPayment
		setting.CreemApiKey = originalApiKey
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalSecret
	})

	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	setting.CreemApiKey = "creem-test-key"
	setting.CreemProducts = `[{"productId":"product-1"}]`
	setting.CreemWebhookSecret = ""
	assert.False(t, isCreemTopUpEnabled())
	setting.CreemWebhookSecret = "webhook-secret"
	assert.True(t, isCreemTopUpEnabled())
}

func TestGetTopUpInfoPreservesMallModeAndExposesOnlyValidURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mall-info.db")), &gorm.Config{})
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

	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string, len(originalOptionMap)+2)
	for key, value := range originalOptionMap {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	tests := []struct {
		name            string
		enabled         string
		url             string
		expectedEnabled bool
		expectedURL     string
		expectError     bool
	}{
		{name: "enabled valid", enabled: "true", url: "https://shop.example.com/buy", expectedEnabled: true, expectedURL: "https://shop.example.com/buy"},
		{name: "disabled valid", enabled: "false", url: "https://shop.example.com/buy"},
		{name: "enabled invalid URL fails closed", enabled: "true", url: "javascript:alert(1)", expectError: true},
		{name: "enabled same-origin URL fails closed", enabled: "true", url: "https://api.example.com/shop", expectError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "payment_setting.mall_enabled").Update("value", test.enabled).Error)
			require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "MallURL").Update("value", test.url).Error)

			common.OptionMapRWMutex.Lock()
			common.OptionMap["payment_setting.mall_enabled"] = strconv.FormatBool(test.enabled != "true")
			common.OptionMap["MallURL"] = "https://stale.example.com"
			common.OptionMapRWMutex.Unlock()

			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, "https://api.example.com/api/user/self/topup/info", nil)
			GetTopUpInfo(context)

			var response struct {
				Success bool `json:"success"`
				Data    struct {
					MallEnabled bool   `json:"mall_enabled"`
					MallURL     string `json:"mall_url"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			if test.expectError {
				assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
				assert.False(t, response.Success)
				return
			}
			assert.True(t, response.Success)
			assert.Equal(t, test.expectedEnabled, response.Data.MallEnabled)
			assert.Equal(t, test.expectedURL, response.Data.MallURL)
		})
	}
}

func TestGetTopUpInfoFailsClosedWhenMallSettingsCannotBeRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mall-info-closed.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "https://api.example.com/api/user/self/topup/info", nil)
	GetTopUpInfo(context)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
}

func TestMallModeUsesCurrentDatabaseValueAndRequiresValidURL(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mall-mode.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create([]model.Option{
		{Key: "payment_setting.mall_enabled", Value: "true"},
		{Key: "MallURL", Value: "https://shop.example.com/buy"},
	}).Error)
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	originalValue, existed := common.OptionMap["payment_setting.mall_enabled"]
	common.OptionMap["payment_setting.mall_enabled"] = "false"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		if existed {
			common.OptionMap["payment_setting.mall_enabled"] = originalValue
		} else {
			delete(common.OptionMap, "payment_setting.mall_enabled")
		}
		common.OptionMapRWMutex.Unlock()
	})

	enabled, mallURL, err := mallTopUpInfo("api.example.com")
	require.NoError(t, err)
	assert.True(t, enabled)
	assert.Equal(t, "https://shop.example.com/buy", mallURL)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "MallURL").Update("value", "https://api.example.com/shop").Error)
	enabled, mallURL, err = mallTopUpInfo("api.example.com")
	require.Error(t, err)
	assert.False(t, enabled)
	assert.Empty(t, mallURL)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "payment_setting.mall_enabled").Update("value", "false").Error)
	enabled, mallURL, err = mallTopUpInfo("api.example.com")
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Empty(t, mallURL)
}

func TestMallModeHidesPlansAndRejectsEveryBuiltInSubscriptionPurchase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, appI18n.Init())
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mall-mode-routing.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create([]model.Option{
		{Key: "payment_setting.mall_enabled", Value: "true"},
		{Key: "MallURL", Value: "https://shop.example.com/buy"},
	}).Error)
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string, len(originalOptionMap)+1)
	for key, value := range originalOptionMap {
		common.OptionMap[key] = value
	}
	common.OptionMap["payment_setting.mall_enabled"] = "true"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	plansRecorder := httptest.NewRecorder()
	plansContext, _ := gin.CreateTestContext(plansRecorder)
	plansContext.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	GetSubscriptionPlans(plansContext)

	var plansResponse struct {
		Success bool                  `json:"success"`
		Data    []SubscriptionPlanDTO `json:"data"`
	}
	require.NoError(t, common.Unmarshal(plansRecorder.Body.Bytes(), &plansResponse))
	assert.True(t, plansResponse.Success)
	assert.Empty(t, plansResponse.Data)

	purchaseHandlers := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "balance", handler: SubscriptionRequestBalancePay},
		{name: "epay", handler: SubscriptionRequestEpay},
		{name: "stripe", handler: SubscriptionRequestStripePay},
		{name: "creem", handler: SubscriptionRequestCreemPay},
		{name: "waffo pancake", handler: SubscriptionRequestWaffoPancakePay},
	}
	for _, purchaseHandler := range purchaseHandlers {
		t.Run(purchaseHandler.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/pay", bytes.NewBufferString(`{"plan_id":1}`))
			context.Request.Header.Set("Content-Type", "application/json")
			context.Request.Header.Set("Accept-Language", "en")
			purchaseHandler.handler(context)

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, "The card-store mall is enabled. Purchase subscriptions through the mall.", response.Message)
		})
	}
}

func TestTopUpPaymentRequestsLimitJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "epay amount", handler: RequestAmount},
		{name: "epay checkout", handler: RequestEpay},
		{name: "stripe amount", handler: RequestStripeAmount},
		{name: "stripe checkout", handler: RequestStripePay},
		{name: "creem checkout", handler: RequestCreemPay},
		{name: "waffo amount", handler: RequestWaffoAmount},
		{name: "waffo pancake amount", handler: RequestWaffoPancakeAmount},
	}
	body := `{"amount":1,"payment_method":"stripe","product_id":"product","padding":"` +
		strings.Repeat("a", int(paymentRequestMaxBytes*2)) + `"}`
	for _, test := range handlers {
		t.Run(test.name, func(t *testing.T) {
			reader := strings.NewReader(body)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup/pay", reader)
			context.Request.Header.Set("Content-Type", "application/json")

			test.handler(context)

			assert.Positive(t, reader.Len(), "handler consumed the entire oversized request")
			assert.LessOrEqual(t, len(body)-reader.Len(), int(paymentRequestMaxBytes)+1)
		})
	}
}
