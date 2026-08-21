package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreateUserPersistsAdminSelectedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalQuotaForNewUser := common.QuotaForNewUser
	originalRedisEnabled := common.RedisEnabled
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "create-user-identity.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.CasbinRule{}))
	model.DB = db
	model.LOG_DB = db
	common.QuotaForNewUser = 0
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.QuotaForNewUser = originalQuotaForNewUser
		common.RedisEnabled = originalRedisEnabled
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("role", common.RoleRootUser)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/",
		strings.NewReader(`{"username":"invoice-org","password":"password","display_name":"Invoice Org","identity":"enterprise","role":10}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	CreateUser(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Truef(t, response.Success, "response: %s", recorder.Body.String())
	var created model.User
	require.NoError(t, db.Where("username = ?", "invoice-org").First(&created).Error)
	assert.Equal(t, model.UserIdentityEnterprise, created.Identity)
}

func TestUpdateUserPreservesPendingIdentityReviewWhenIdentityIsOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "update-user-identity.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	original := model.User{
		Username:             "pending-edit-user",
		Password:             "password",
		Role:                 common.RoleAdminUser,
		Status:               common.UserStatusEnabled,
		Identity:             model.UserIdentityPersonal,
		IdentityRequested:    model.UserIdentityUniversity,
		IdentityReviewStatus: "pending",
	}
	require.NoError(t, db.Create(&original).Error)

	request := httptest.NewRequest(http.MethodPut, "/api/user/", strings.NewReader(
		`{"id":`+strconv.Itoa(original.Id)+`,"username":"pending-edit-user","display_name":"Pending Edit","role":10}`,
	))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("role", common.RoleRootUser)
	context.Request = request

	UpdateUser(context)
	assert.Equalf(t, http.StatusOK, recorder.Code, "response: %s", recorder.Body.String())

	var updated model.User
	require.NoError(t, db.First(&updated, original.Id).Error)
	assert.Equal(t, model.UserIdentityUniversity, updated.IdentityRequested)
	assert.Equal(t, "pending", updated.IdentityReviewStatus)

	request = httptest.NewRequest(http.MethodPut, "/api/user/", strings.NewReader(
		`{"id":`+strconv.Itoa(original.Id)+`,"username":"pending-edit-user","display_name":"Changed Identity","identity":"enterprise","role":10}`,
	))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(recorder)
	context.Set("role", common.RoleRootUser)
	context.Request = request
	UpdateUser(context)
	assert.Equalf(t, http.StatusOK, recorder.Code, "response: %s", recorder.Body.String())
	require.NoError(t, db.First(&updated, original.Id).Error)
	assert.Equal(t, model.UserIdentityEnterprise, updated.Identity)
	assert.Empty(t, updated.IdentityRequested)
	assert.Empty(t, updated.IdentityReviewStatus)
}
