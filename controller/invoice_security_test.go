package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetSelfInvoiceCenterReturnsPaymentIdentifierAndDisplayName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalMethodSettings := operation_setting.GetPaymentSetting().MethodSettings
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invoice-payment-name.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(
		&model.Option{}, &model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{},
		&model.TopUp{}, &model.Redemption{}, &model.InvoiceApplication{}, &model.InvoiceApplicationItem{},
	))
	require.NoError(t, db.Create([]model.Option{
		{Key: "InvoiceEnabled", Value: "true"},
		{Key: "InvoiceApplicationDay", Value: "15"},
		{Key: "InvoiceLookbackDays", Value: "90"},
		{Key: "InvoiceMonthlyLimit", Value: "1"},
	}).Error)
	operation_setting.GetPaymentSetting().MethodSettings = []operation_setting.PaymentMethodSetting{
		{Type: model.PaymentMethodWechat, Name: "微信扫码", Enabled: common.GetPointer(true)},
	}
	t.Cleanup(func() {
		model.DB = originalDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		operation_setting.GetPaymentSetting().MethodSettings = originalMethodSettings
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	user := model.User{
		Username: "invoice-wechat-user", Password: "password", AffCode: "invoice-wechat-user",
		Identity: model.UserIdentityEnterprise,
	}
	require.NoError(t, db.Create(&user).Error)
	now := common.GetTimestamp()
	topUp := model.TopUp{
		UserId: user.Id, Amount: 100, Money: 10, TradeNo: "invoice-wechat-trade",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		Source: model.TopUpSourceRecharge, Status: common.TopUpStatusSuccess,
		CompleteTime: now, CreateTime: now, InvoiceAmountMicros: 10_000_000, InvoiceCurrency: "CNY",
	}
	require.NoError(t, db.Create(&topUp).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", user.Id)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/invoice", nil)

	GetSelfInvoiceCenter(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Subscriptions []model.InvoiceEligibleSubscription `json:"subscriptions"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.Subscriptions, 1)
	item := response.Data.Subscriptions[0]
	assert.Equal(t, model.PaymentMethodWechat, item.PaymentMethod)
	assert.Equal(t, "微信扫码", item.PaymentMethodName)
	assert.NotEqual(t, model.PaymentMethodWechat, item.PlanTitle)

	_, err = model.CreateInvoiceApplication(user.Id, model.InvoiceSettings{
		LookbackDays: 90, MonthlyLimit: 1, SystemRechargeEnabled: true,
	}, model.InvoiceApplicationInput{
		InvoiceTitle: "微信充值发票", TaxpayerId: "91310000MA1K123456",
	}, []int{-topUp.Id})
	require.NoError(t, err)

	historyRecorder := httptest.NewRecorder()
	historyContext, _ := gin.CreateTestContext(historyRecorder)
	historyContext.Set("id", user.Id)
	historyContext.Request = httptest.NewRequest(http.MethodGet, "/api/user/invoice", nil)
	GetSelfInvoiceCenter(historyContext)

	var historyResponse struct {
		Data struct {
			Applications []model.InvoiceApplication `json:"applications"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(historyRecorder.Body.Bytes(), &historyResponse))
	require.Len(t, historyResponse.Data.Applications, 1)
	require.Len(t, historyResponse.Data.Applications[0].Items, 1)
	historyItem := historyResponse.Data.Applications[0].Items[0]
	assert.Equal(t, model.PaymentMethodWechat, historyItem.PaymentMethod)
	assert.Equal(t, "微信扫码", historyItem.PaymentMethodName)
	assert.Empty(t, historyItem.PlanTitle)
}

func TestInvoiceApplicationAdminResponseExcludesSensitiveUserFields(t *testing.T) {
	application := model.InvoiceApplication{
		Id: 1,
		User: &model.User{
			Id: 2, Username: "invoice-user", DisplayName: "Invoice User",
			Email: "invoice@example.com", Identity: model.UserIdentityEnterprise,
			Password: "password-hash", GitHubId: "github-id", StripeCustomer: "stripe-customer",
		},
	}

	payload, err := common.Marshal(newInvoiceApplicationAdminResponse(application))
	require.NoError(t, err)
	responseBody := string(payload)
	assert.Contains(t, responseBody, "invoice-user")
	assert.NotContains(t, responseBody, "password")
	assert.NotContains(t, responseBody, "password-hash")
	assert.NotContains(t, responseBody, "github-id")
	assert.NotContains(t, responseBody, "stripe-customer")
}

func TestPendingIdentityReviewCanReadHistoryButCannotApplyForInvoice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invoice-identity.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(
		&model.Option{}, &model.User{}, &model.SubscriptionPlan{}, &model.UserSubscription{},
		&model.InvoiceApplication{}, &model.InvoiceApplicationItem{},
	))
	require.NoError(t, db.Create([]model.Option{
		{Key: "InvoiceEnabled", Value: "true"},
		{Key: "InvoiceApplicationDay", Value: "15"},
		{Key: "InvoiceLookbackDays", Value: "90"},
		{Key: "InvoiceMonthlyLimit", Value: "1"},
	}).Error)

	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"InvoiceEnabled":        "true",
		"InvoiceApplicationDay": "15",
		"InvoiceLookbackDays":   "90",
		"InvoiceMonthlyLimit":   "1",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB = originalDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	user := model.User{
		Username: "pending-invoice-user", Password: "password", AffCode: "pending-invoice-user",
		Identity: model.UserIdentityPersonal, IdentityRequested: model.UserIdentityUniversity,
		IdentityReviewStatus: "pending",
	}
	require.NoError(t, db.Create(&user).Error)

	historicalApplication := model.InvoiceApplication{
		UserId: user.Id, ApplicationMonth: time.Now().Format("2006-01"),
		InvoiceTitle: "Historical invoice", TotalAmountMicros: 1_000_000,
		Currency: "CNY", Status: model.InvoiceApplicationStatusCompleted,
	}
	require.NoError(t, db.Create(&historicalApplication).Error)

	t.Run("GET history without candidate access", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", user.Id)
		context.Request = httptest.NewRequest(http.MethodGet, "/api/user/invoice", nil)

		GetSelfInvoiceCenter(context)

		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				IdentityEligible bool                                `json:"identity_eligible"`
				Subscriptions    []model.InvoiceEligibleSubscription `json:"subscriptions"`
				Applications     []model.InvoiceApplication          `json:"applications"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.True(t, response.Success)
		assert.False(t, response.Data.IdentityEligible)
		assert.Empty(t, response.Data.Subscriptions)
		require.Len(t, response.Data.Applications, 1)
		assert.Equal(t, historicalApplication.Id, response.Data.Applications[0].Id)
	})

	t.Run("POST forged subscription", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", user.Id)
		context.Request = httptest.NewRequest(
			http.MethodPost,
			"/api/user/invoice/apply",
			bytes.NewBufferString(`{"invoice_title":"Pending review","subscription_ids":[999999]}`),
		)
		context.Request.Header.Set("Content-Type", "application/json")

		CreateInvoiceApplication(context)

		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "university or enterprise")
	})
}
