package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldMainDatabaseType := common.MainDatabaseType()
	oldLogDatabaseType := common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCacheEnabled := common.MemoryCacheEnabled

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	t.Cleanup(func() {
		model.DB = oldDB
		common.SetDatabaseTypes(oldMainDatabaseType, oldLogDatabaseType)
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		if oldMemoryCacheEnabled && oldDB != nil {
			model.InitChannelCache()
		}
	})

	return db
}

func seedChannelSelectAbility(t *testing.T, db *gorm.DB, channelID int, group string, modelName string) {
	t.Helper()

	priority := int64(0)
	weight := uint(0)
	require.NoError(t, db.Create(&model.Channel{
		Id:       channelID,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("test-key-%d", channelID),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", channelID),
		Group:    group,
		Models:   modelName,
		Priority: &priority,
		Weight:   &weight,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: channelID,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelMultiGroupReturnsNilForMissingModel(t *testing.T) {
	setupChannelSelectTestDB(t)
	model.InitChannelCache()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default,vip",
		ModelName:  "missing-model",
		Retry:      &retry,
	})

	require.NoError(t, err)
	require.Nil(t, channel)
	require.Equal(t, "default,vip", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelMultiGroupUsesFirstMatchingGroup(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	seedChannelSelectAbility(t, db, 1, "default", "same-model")
	seedChannelSelectAbility(t, db, 2, "vip", "same-model")
	model.InitChannelCache()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default,vip",
		ModelName:  "same-model",
		Retry:      &retry,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelAutoGroupUsesDefaultRoute(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	seedChannelSelectAbility(t, db, 1, "default", "same-model")
	model.InitChannelCache()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "same-model",
		Retry:      &retry,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelMultiGroupFallsThroughByOrder(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	seedChannelSelectAbility(t, db, 2, "vip", "vip-only-model")
	model.InitChannelCache()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default,vip",
		ModelName:  "vip-only-model",
		Retry:      &retry,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 2, channel.Id)
	require.Equal(t, "vip", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelMultiGroupCrossRetryMovesAfterRetryExhausted(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	seedChannelSelectAbility(t, db, 1, "default", "same-model")
	seedChannelSelectAbility(t, db, 2, "vip", "same-model")
	seedChannelSelectAbility(t, db, 3, "pro", "same-model")
	model.InitChannelCache()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = originalRetryTimes })

	retry := 0
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "default,vip,pro",
		ModelName:  "same-model",
		Retry:      &retry,
	}

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, "default", selectedGroup)

	param.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, "default", selectedGroup)

	param.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 2, channel.Id)
	require.Equal(t, "vip", selectedGroup)

	param.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 3, channel.Id)
	require.Equal(t, "pro", selectedGroup)

	param.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Nil(t, channel)
	require.Equal(t, "default,vip,pro", selectedGroup)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyCrossGroupExhausted))
}

func TestCacheGetRandomSatisfiedChannelMultiGroupCrossRetrySkipsMissingGroups(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	seedChannelSelectAbility(t, db, 1, "default", "same-model")
	seedChannelSelectAbility(t, db, 3, "pro", "same-model")
	model.InitChannelCache()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = originalRetryTimes })

	retry := 0
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "default,vip,pro",
		ModelName:  "same-model",
		Retry:      &retry,
	}

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, "default", selectedGroup)

	param.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, "default", selectedGroup)

	param.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 3, channel.Id)
	require.Equal(t, "pro", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelMultiGroupWithoutCrossRetryStaysOnGroup(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	seedChannelSelectAbility(t, db, 1, "default", "same-model")
	seedChannelSelectAbility(t, db, 2, "vip", "same-model")
	model.InitChannelCache()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	retry := 0
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "default,vip",
		ModelName:  "same-model",
		Retry:      &retry,
	}

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, "default", selectedGroup)

	param.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelRespectsPriorityOrder(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	highPriority := int64(10)
	lowPriority := int64(1)
	weight := uint(1)
	require.NoError(t, db.Create(&model.Channel{
		Id:       11,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "priority-high",
		Status:   common.ChannelStatusEnabled,
		Name:     "priority-high",
		Group:    "default",
		Models:   "same-model",
		Priority: &highPriority,
		Weight:   &weight,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "same-model",
		ChannelId: 11,
		Enabled:   true,
		Priority:  &highPriority,
		Weight:    weight,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:       12,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "priority-low",
		Status:   common.ChannelStatusEnabled,
		Name:     "priority-low",
		Group:    "default",
		Models:   "same-model",
		Priority: &lowPriority,
		Weight:   &weight,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "same-model",
		ChannelId: 12,
		Enabled:   true,
		Priority:  &lowPriority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "same-model",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 11, channel.Id)
	require.Equal(t, "default", selectedGroup)

	retry = 1
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "same-model",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 12, channel.Id)
	require.Equal(t, "default", selectedGroup)
}
