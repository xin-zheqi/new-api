package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupManualTopUpHandlerTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalMainDatabaseType, originalLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "manual-topup.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = database, database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}, &model.TopUp{}))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		sqlDB, dbErr := database.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	gin.SetMode(gin.TestMode)
	return database
}

func callAdminCreateManualTopUp(t *testing.T, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup/manual", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	AdminCreateManualTopUp(context)
	return recorder
}

func TestAdminCreateManualTopUpPersistsPaymentAndInvoiceCurrencies(t *testing.T) {
	database := setupManualTopUpHandlerTest(t)
	user := model.User{Username: "manual-topup-user", Password: "password", AffCode: "manual-topup-user"}
	require.NoError(t, database.Create(&user).Error)

	recorder := callAdminCreateManualTopUp(t, map[string]any{
		"user_id": user.Id, "payment_method": "bank_transfer",
		"amount": 2, "money": 3.25, "currency": "cny",
		"invoice_amount": 2.75, "invoice_currency": "usd",
		"create_time": 1710000000, "credit_balance": false,
	})

	var response struct {
		Success bool        `json:"success"`
		Data    model.TopUp `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	assert.Equal(t, "CNY", response.Data.Currency)
	assert.Equal(t, 3.25, response.Data.Money)
	assert.Equal(t, int64(2_750_000), response.Data.InvoiceAmountMicros)
	assert.Equal(t, "USD", response.Data.InvoiceCurrency)

	var stored model.TopUp
	require.NoError(t, database.First(&stored, response.Data.Id).Error)
	assert.Equal(t, "bank_transfer", stored.PaymentMethod)
	assert.Equal(t, "CNY", stored.Currency)
	assert.Equal(t, 3.25, stored.Money)
	assert.Equal(t, int64(2_750_000), stored.InvoiceAmountMicros)
	assert.Equal(t, "USD", stored.InvoiceCurrency)
}

func TestAdminCreateManualTopUpDefaultsLegacyRequestCurrencyToCNY(t *testing.T) {
	database := setupManualTopUpHandlerTest(t)
	user := model.User{Username: "manual-topup-legacy", Password: "password", AffCode: "manual-topup-legacy"}
	require.NoError(t, database.Create(&user).Error)

	recorder := callAdminCreateManualTopUp(t, map[string]any{
		"user_id": user.Id, "payment_method": "bank_transfer",
		"amount": 2, "money": 3.25,
		"create_time": 1710000000, "credit_balance": false,
	})

	var response struct {
		Success bool        `json:"success"`
		Data    model.TopUp `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	assert.Equal(t, "CNY", response.Data.Currency)

	var stored model.TopUp
	require.NoError(t, database.First(&stored, response.Data.Id).Error)
	assert.Equal(t, "CNY", stored.Currency)
}

func TestAdminCreateManualTopUpRejectsInvalidPaymentCurrency(t *testing.T) {
	database := setupManualTopUpHandlerTest(t)
	user := model.User{Username: "manual-topup-invalid", Password: "password", AffCode: "manual-topup-invalid"}
	require.NoError(t, database.Create(&user).Error)

	for _, currency := range []string{"XXX", "not-a-currency"} {
		recorder := callAdminCreateManualTopUp(t, map[string]any{
			"user_id": user.Id, "payment_method": "bank_transfer",
			"amount": 2, "money": 3.25, "currency": currency,
			"create_time": 1710000000, "credit_balance": false,
		})
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success, "currency %q must be rejected", currency)
	}

	var count int64
	require.NoError(t, database.Model(&model.TopUp{}).Count(&count).Error)
	assert.Zero(t, count)
}
