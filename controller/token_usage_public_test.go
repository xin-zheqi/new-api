package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPublicTokenUsageReturnsOnlyOwnSuccessfulRequestsWithoutLogin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "public-token-usage.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.SetDatabaseTypes(mainType, logType)
		connection, err := db.DB()
		if err == nil {
			_ = connection.Close()
		}
	})

	owner := model.User{Username: "balance-owner", Password: "test-password", Status: common.UserStatusEnabled, AffCode: "balance-owner-code"}
	other := model.User{Username: "balance-other", Password: "test-password", Status: common.UserStatusEnabled, AffCode: "balance-other-code"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)
	token := model.Token{
		UserId: owner.Id, Key: "balancekey", Name: "Public balance key",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
		QuotaPeriod: model.QuotaPeriodDay, QuotaPeriodLimit: 100,
		QuotaPeriodUsed: 100, QuotaPeriodStart: model.QuotaPeriodStart(model.QuotaPeriodDay, time.Now()),
	}
	otherToken := model.Token{
		UserId: other.Id, Key: "otherbalancekey", Name: "Other user's key",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.Create(&otherToken).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: owner.Id, TokenId: token.Id, Type: model.LogTypeConsume,
		CreatedAt: 100, ModelName: "visible-model", Quota: 750,
		PromptTokens: 20, CompletionTokens: 5,
		Content: "private prompt", Other: `{"cache_tokens":4}`,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: owner.Id, TokenId: token.Id, Type: model.LogTypeError,
		CreatedAt: 101, ModelName: "failed-model", Content: "private error",
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: other.Id, TokenId: otherToken.Id, Type: model.LogTypeConsume,
		CreatedAt: 102, ModelName: "other-model", Content: "other private prompt",
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/usage/token/", middleware.TokenAuthReadOnly(), GetTokenUsage)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/usage/token/", nil)
	request.Header.Set("Authorization", "Bearer sk-balancekey-default")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Code bool `json:"code"`
		Data struct {
			Name            string                     `json:"name"`
			PeriodRemaining int                        `json:"period_remaining"`
			RecentRequests  []model.TokenRecentRequest `json:"recent_requests"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Code)
	assert.Equal(t, token.Name, response.Data.Name)
	assert.Zero(t, response.Data.PeriodRemaining)
	require.Len(t, response.Data.RecentRequests, 1)
	assert.Equal(t, "visible-model", response.Data.RecentRequests[0].ModelName)
	assert.Equal(t, 750, response.Data.RecentRequests[0].Quota)
	assert.Equal(t, 4, response.Data.RecentRequests[0].CacheReadTokens)
	assert.NotContains(t, recorder.Body.String(), "private")
	assert.NotContains(t, recorder.Body.String(), "failed-model")
	assert.NotContains(t, recorder.Body.String(), "other-model")

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/usage/token/", nil))
	assert.Equal(t, http.StatusUnauthorized, unauthorized.Code)
}
