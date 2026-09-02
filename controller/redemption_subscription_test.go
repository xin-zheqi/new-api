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

func TestSubscriptionRedemptionHandlersAcceptProtocolZeroQuota(t *testing.T) {
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalMainDatabaseType, originalLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "subscription-redemption.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.Redemption{}, &model.SubscriptionPlan{}))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	confirmPaymentComplianceForTest(t)
	gin.SetMode(gin.TestMode)

	admin := model.User{Username: "redemption-admin", Password: "password", AffCode: "redemption-admin"}
	require.NoError(t, db.Create(&admin).Error)
	plan := model.SubscriptionPlan{
		Title: "Handler redemption plan", PriceAmount: 9.99, Currency: "USD",
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: 1000,
	}
	require.NoError(t, db.Create(&plan).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	t.Cleanup(func() { model.InvalidateSubscriptionPlanCache(plan.Id) })

	createBody, err := common.Marshal(map[string]any{
		"name": "handler-subscription", "count": 1, "redeem_type": model.RedemptionTypeSubscription,
		"quota": 0, "subscription_plan_id": plan.Id,
	})
	require.NoError(t, err)
	createRecorder := httptest.NewRecorder()
	createContext, _ := gin.CreateTestContext(createRecorder)
	createContext.Set("id", admin.Id)
	createContext.Request = httptest.NewRequest(http.MethodPost, "/api/redemption/", bytes.NewReader(createBody))
	createContext.Request.Header.Set("Content-Type", "application/json")
	AddRedemption(createContext)

	var createResponse struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(createRecorder.Body.Bytes(), &createResponse))
	require.True(t, createResponse.Success, createRecorder.Body.String())
	require.Len(t, createResponse.Data, 1)

	var redemption model.Redemption
	require.NoError(t, db.Where("key = ?", createResponse.Data[0]).First(&redemption).Error)
	assert.Zero(t, redemption.Quota)
	assert.Equal(t, plan.Id, redemption.SubscriptionPlanId)

	updateBody, err := common.Marshal(map[string]any{
		"id": redemption.Id, "name": "handler-sub-updated",
		"redeem_type": model.RedemptionTypeSubscription, "quota": 0,
		"subscription_plan_id": plan.Id,
	})
	require.NoError(t, err)
	updateRecorder := httptest.NewRecorder()
	updateContext, _ := gin.CreateTestContext(updateRecorder)
	updateContext.Set("id", admin.Id)
	updateContext.Request = httptest.NewRequest(http.MethodPut, "/api/redemption/", bytes.NewReader(updateBody))
	updateContext.Request.Header.Set("Content-Type", "application/json")
	UpdateRedemption(updateContext)

	var updateResponse struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(updateRecorder.Body.Bytes(), &updateResponse))
	require.True(t, updateResponse.Success, updateRecorder.Body.String())
	require.NoError(t, db.First(&redemption, redemption.Id).Error)
	assert.Equal(t, "handler-sub-updated", redemption.Name)
	assert.Zero(t, redemption.Quota)
}
