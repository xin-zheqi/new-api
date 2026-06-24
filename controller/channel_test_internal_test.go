package controller

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type contextBlockingReadCloser struct {
	ctx context.Context
}

func (r *contextBlockingReadCloser) Read(_ []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (r *contextBlockingReadCloser) Close() error {
	return nil
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestShouldDisableChannelAfterTestAutomaticDisablesAnyError(t *testing.T) {
	err := types.NewOpenAIError(errors.New("temporary upstream failure"), "temporary_upstream_failure", http.StatusTooManyRequests)

	require.True(t, shouldDisableChannelAfterTest(err, false))
}

func TestShouldDisableChannelAfterTestManualUsesDisableRules(t *testing.T) {
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = false
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	err := types.NewOpenAIError(errors.New("temporary upstream failure"), "temporary_upstream_failure", http.StatusTooManyRequests)

	require.False(t, shouldDisableChannelAfterTest(err, true))
}

func TestShouldDisableChannelAfterTestIgnoresNilError(t *testing.T) {
	require.False(t, shouldDisableChannelAfterTest(nil, false))
	require.False(t, shouldDisableChannelAfterTest(nil, true))
}

func TestShouldDisableChannelAfterTestWithContextRequiresFirstResponseTimeoutSwitch(t *testing.T) {
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	err := types.NewOpenAIError(
		errors.New("upstream first response timeout after 30s"),
		types.ErrorCodeChannelFirstResponseTimeout,
		http.StatusBadGateway,
	)

	require.False(t, shouldDisableChannelAfterTestWithContext(ctx, err, false))
	require.False(t, shouldDisableChannelAfterTestWithContext(ctx, err, true))

	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		FirstResponseTimeoutAutoBan: true,
	})

	require.True(t, shouldDisableChannelAfterTestWithContext(ctx, err, false))
	require.True(t, shouldDisableChannelAfterTestWithContext(ctx, err, true))
}

func TestProcessChannelErrorSkipsFirstResponseTimeoutAutoDisableByDefault(t *testing.T) {
	originalErrorLogEnabled := constant.ErrorLogEnabled
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	constant.ErrorLogEnabled = false
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		constant.ErrorLogEnabled = originalErrorLogEnabled
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set("channel_name", "demo")
	ctx.Set("channel_id", 1)
	ctx.Set("channel_type", constant.ChannelTypeOpenAI)
	ctx.Set("id", 1)
	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		FirstResponseTimeoutSeconds: 30,
	})

	err := types.NewOpenAIError(
		errors.New("upstream first response timeout after 30s"),
		types.ErrorCodeChannelFirstResponseTimeout,
		http.StatusBadGateway,
	)

	require.False(t, shouldDisableChannelForRelayError(ctx, err))
}

func TestProcessChannelErrorDisablesFirstResponseTimeoutWhenEnabled(t *testing.T) {
	originalErrorLogEnabled := constant.ErrorLogEnabled
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	constant.ErrorLogEnabled = false
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		constant.ErrorLogEnabled = originalErrorLogEnabled
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set("channel_name", "demo")
	ctx.Set("channel_id", 1)
	ctx.Set("channel_type", constant.ChannelTypeOpenAI)
	ctx.Set("id", 1)
	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		FirstResponseTimeoutSeconds: 30,
		FirstResponseTimeoutAutoBan: true,
	})

	err := types.NewOpenAIError(
		errors.New("upstream first response timeout after 30s"),
		types.ErrorCodeChannelFirstResponseTimeout,
		http.StatusBadGateway,
	)

	require.True(t, shouldDisableChannelForRelayError(ctx, err))
}

func TestShouldSkipScheduledAutoTestChannelOnlyAutoDisabled(t *testing.T) {
	autoTestEnabled := true
	autoTestDisabled := false

	require.False(t, func() bool {
		skipped, _ := shouldSkipScheduledAutoTestChannel(&model.Channel{
			Status: common.ChannelStatusAutoDisabled,
		}, true)
		return skipped
	}())

	skipped, reason := shouldSkipScheduledAutoTestChannel(&model.Channel{
		Status: common.ChannelStatusEnabled,
	}, true)
	require.True(t, skipped)
	require.Equal(t, "only_auto_disabled", reason)

	channelOptedOut := &model.Channel{
		Status: common.ChannelStatusAutoDisabled,
	}
	channelOptedOut.SetOtherSettings(dto.ChannelOtherSettings{AutoTestEnabled: &autoTestDisabled})
	skipped, reason = shouldSkipScheduledAutoTestChannel(channelOptedOut, true)
	require.True(t, skipped)
	require.Equal(t, "auto_test_disabled", reason)

	channelOptedIn := &model.Channel{
		Status: common.ChannelStatusAutoDisabled,
	}
	channelOptedIn.SetOtherSettings(dto.ChannelOtherSettings{AutoTestEnabled: &autoTestEnabled})
	skipped, reason = shouldSkipScheduledAutoTestChannel(channelOptedIn, true)
	require.False(t, skipped)
	require.Empty(t, reason)
}

func TestAutomaticTestAllChannelsDisablesErrorWithoutDisableRules(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalIsMasterNode := common.IsMasterNode
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	originalRequestInterval := common.RequestInterval
	originalDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	originalDisableKeywords := operation_setting.AutomaticDisableKeywords
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	t.Cleanup(func() {
		testAllChannelsLock.Lock()
		testAllChannelsRunning = false
		testAllChannelsLock.Unlock()

		if model.DB != nil && model.DB != originalDB {
			if sqlDB, err := model.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		model.DB = originalDB
		model.LOG_DB = originalLOGDB
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.IsMasterNode = originalIsMasterNode
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		common.AutomaticEnableChannelEnabled = originalAutomaticEnable
		common.RequestInterval = originalRequestInterval
		operation_setting.AutomaticDisableStatusCodeRanges = originalDisableRanges
		operation_setting.AutomaticDisableKeywords = originalDisableKeywords
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})

	testAllChannelsLock.Lock()
	testAllChannelsRunning = false
	testAllChannelsLock.Unlock()

	common.SQLitePath = "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.IsMasterNode = false
	common.AutomaticDisableChannelEnabled = false
	common.AutomaticEnableChannelEnabled = false
	common.RequestInterval = 0
	operation_setting.AutomaticDisableStatusCodeRanges = nil
	operation_setting.AutomaticDisableKeywords = nil
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":1}`))
	service.InitHttpClient()
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))

	var requestWasStream atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := common.Unmarshal(body, &payload); err == nil {
			requestWasStream.Store(payload["stream"] == true)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"temporary upstream failure","type":"server_error","code":"temporary"}}`))
	}))
	t.Cleanup(upstream.Close)

	require.NoError(t, model.DB.Create(&model.User{
		Id:       1,
		Username: "root",
		Password: "password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Quota:    1000000,
		Group:    "default",
	}).Error)

	autoBan := 1
	baseURL := upstream.URL
	channel := model.Channel{
		Id:      1,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		Name:    "auto-test-channel",
		BaseURL: &baseURL,
		Models:  "gpt-4o-mini",
		Group:   "default",
		AutoBan: &autoBan,
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	require.NoError(t, testAllChannels(false))

	require.Eventually(t, func() bool {
		var refreshed model.Channel
		if err := model.DB.First(&refreshed, channel.Id).Error; err != nil {
			return false
		}
		return refreshed.Status == common.ChannelStatusAutoDisabled && requestWasStream.Load()
	}, 3*time.Second, 25*time.Millisecond)
}

func TestScheduledAutomaticChannelTestCancelsUpstreamOnThreshold(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalIsMasterNode := common.IsMasterNode
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	originalRequestInterval := common.RequestInterval
	originalMonitorSetting := *operation_setting.GetMonitorSetting()
	originalChannelDisableThreshold := common.ChannelDisableThreshold
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	t.Cleanup(func() {
		testAllChannelsLock.Lock()
		testAllChannelsRunning = false
		testAllChannelsLock.Unlock()

		if model.DB != nil && model.DB != originalDB {
			if sqlDB, err := model.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		model.DB = originalDB
		model.LOG_DB = originalLOGDB
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.IsMasterNode = originalIsMasterNode
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		common.AutomaticEnableChannelEnabled = originalAutomaticEnable
		common.RequestInterval = originalRequestInterval
		common.ChannelDisableThreshold = originalChannelDisableThreshold
		*operation_setting.GetMonitorSetting() = originalMonitorSetting
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})

	testAllChannelsLock.Lock()
	testAllChannelsRunning = false
	testAllChannelsLock.Unlock()

	common.SQLitePath = "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.IsMasterNode = false
	common.AutomaticDisableChannelEnabled = false
	common.AutomaticEnableChannelEnabled = false
	common.RequestInterval = 0
	common.ChannelDisableThreshold = 0.05
	operation_setting.GetMonitorSetting().AutoTestOnlyAutoDisabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":1}`))
	service.InitHttpClient()
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))

	cancelObserved := make(chan struct{}, 1)
	connectionClosed := make(chan struct{}, 1)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		<-r.Context().Done()
		select {
		case cancelObserved <- struct{}{}:
		default:
		}
	}))
	upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case connectionClosed <- struct{}{}:
			default:
			}
		}
	}
	upstream.Start()
	t.Cleanup(upstream.Close)

	require.NoError(t, model.DB.Create(&model.User{
		Id:       1,
		Username: "root",
		Password: "password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Quota:    1000000,
		Group:    "default",
	}).Error)

	autoBan := 1
	autoTestEnabled := true
	baseURL := upstream.URL
	channel := model.Channel{
		Id:      1,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		Name:    "timed-auto-test-channel",
		BaseURL: &baseURL,
		Models:  "gpt-4o-mini",
		Group:   "default",
		AutoBan: &autoBan,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AutoTestEnabled: &autoTestEnabled})
	require.NoError(t, model.DB.Create(&channel).Error)

	result := testChannel(&channel, 1, "", "", true, true)

	require.NotNil(t, result.newAPIError)
	require.Equal(t, types.ErrorCodeChannelResponseTimeExceeded, result.newAPIError.GetErrorCode())
	select {
	case <-cancelObserved:
	case <-connectionClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not observe automatic test cancellation")
	}
}

func TestAutomaticTestAllChannelsTestsAutoDisabledChannelsByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalIsMasterNode := common.IsMasterNode
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	originalRequestInterval := common.RequestInterval
	originalMonitorSetting := *operation_setting.GetMonitorSetting()
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	t.Cleanup(func() {
		testAllChannelsLock.Lock()
		testAllChannelsRunning = false
		testAllChannelsLock.Unlock()

		if model.DB != nil && model.DB != originalDB {
			if sqlDB, err := model.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		model.DB = originalDB
		model.LOG_DB = originalLOGDB
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.IsMasterNode = originalIsMasterNode
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		common.AutomaticEnableChannelEnabled = originalAutomaticEnable
		common.RequestInterval = originalRequestInterval
		*operation_setting.GetMonitorSetting() = originalMonitorSetting
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})

	testAllChannelsLock.Lock()
	testAllChannelsRunning = false
	testAllChannelsLock.Unlock()

	common.SQLitePath = "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.IsMasterNode = false
	common.AutomaticDisableChannelEnabled = false
	common.AutomaticEnableChannelEnabled = false
	common.RequestInterval = 0
	operation_setting.GetMonitorSetting().AutoTestOnlyAutoDisabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":1}`))
	service.InitHttpClient()
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))

	var requestCount atomic.Int32
	requestWasStream := atomic.Bool{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := common.Unmarshal(body, &payload); err == nil {
			requestWasStream.Store(payload["stream"] == true)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(upstream.Close)

	require.NoError(t, model.DB.Create(&model.User{
		Id:       1,
		Username: "root",
		Password: "password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Quota:    1000000,
		Group:    "default",
	}).Error)

	autoBan := 1
	baseURL := upstream.URL
	channel := model.Channel{
		Id:      1,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		Status:  common.ChannelStatusAutoDisabled,
		Name:    "auto-disabled-channel",
		BaseURL: &baseURL,
		Models:  "gpt-4o-mini",
		Group:   "default",
		AutoBan: &autoBan,
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	require.NoError(t, testAllChannels(false))

	require.Eventually(t, func() bool {
		return requestCount.Load() == 1 && requestWasStream.Load()
	}, 3*time.Second, 25*time.Millisecond)
}

func TestAutomaticTestAllChannelsOnlyAutoDisabledSkipsEnabledAndReenablesRecovered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalIsMasterNode := common.IsMasterNode
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	originalRequestInterval := common.RequestInterval
	originalMonitorSetting := *operation_setting.GetMonitorSetting()
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	t.Cleanup(func() {
		testAllChannelsLock.Lock()
		testAllChannelsRunning = false
		testAllChannelsLock.Unlock()

		if model.DB != nil && model.DB != originalDB {
			if sqlDB, err := model.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		model.DB = originalDB
		model.LOG_DB = originalLOGDB
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.IsMasterNode = originalIsMasterNode
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		common.AutomaticEnableChannelEnabled = originalAutomaticEnable
		common.RequestInterval = originalRequestInterval
		*operation_setting.GetMonitorSetting() = originalMonitorSetting
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})

	testAllChannelsLock.Lock()
	testAllChannelsRunning = false
	testAllChannelsLock.Unlock()

	common.SQLitePath = "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.IsMasterNode = false
	common.AutomaticDisableChannelEnabled = true
	common.AutomaticEnableChannelEnabled = true
	common.RequestInterval = 0
	operation_setting.GetMonitorSetting().AutoTestOnlyAutoDisabled = true
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":1}`))
	service.InitHttpClient()
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))

	var skippedEnabledRequests atomic.Int32
	skippedEnabledUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		skippedEnabledRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"enabled channel should have been skipped","type":"server_error","code":"temporary"}}`))
	}))
	t.Cleanup(skippedEnabledUpstream.Close)

	var recoveredRequests atomic.Int32
	recoveredWasStream := atomic.Bool{}
	recoveredUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recoveredRequests.Add(1)
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := common.Unmarshal(body, &payload); err == nil {
			recoveredWasStream.Store(payload["stream"] == true)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(recoveredUpstream.Close)

	require.NoError(t, model.DB.Create(&model.User{
		Id:       1,
		Username: "root",
		Password: "password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Quota:    1000000,
		Group:    "default",
	}).Error)

	autoBan := 1
	enabledURL := skippedEnabledUpstream.URL
	enabledChannel := model.Channel{
		Id:      1,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key-enabled",
		Status:  common.ChannelStatusEnabled,
		Name:    "enabled-channel",
		BaseURL: &enabledURL,
		Models:  "gpt-4o-mini",
		Group:   "default",
		AutoBan: &autoBan,
	}
	require.NoError(t, model.DB.Create(&enabledChannel).Error)

	recoveredURL := recoveredUpstream.URL
	recoveredChannel := model.Channel{
		Id:      2,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key-recovered",
		Status:  common.ChannelStatusAutoDisabled,
		Name:    "recovered-channel",
		BaseURL: &recoveredURL,
		Models:  "gpt-4o-mini",
		Group:   "default",
		AutoBan: &autoBan,
	}
	require.NoError(t, model.DB.Create(&recoveredChannel).Error)

	require.NoError(t, testAllChannels(false))

	require.Eventually(t, func() bool {
		var enabledRefreshed model.Channel
		var recoveredRefreshed model.Channel
		if err := model.DB.First(&enabledRefreshed, enabledChannel.Id).Error; err != nil {
			return false
		}
		if err := model.DB.First(&recoveredRefreshed, recoveredChannel.Id).Error; err != nil {
			return false
		}
		return skippedEnabledRequests.Load() == 0 &&
			enabledRefreshed.Status == common.ChannelStatusEnabled &&
			recoveredRequests.Load() == 1 &&
			recoveredWasStream.Load() &&
			recoveredRefreshed.Status == common.ChannelStatusEnabled
	}, 3*time.Second, 25*time.Millisecond)
}

func TestManualTestAllChannelsIncludesAutoDisabledChannels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalIsMasterNode := common.IsMasterNode
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	originalAutomaticEnable := common.AutomaticEnableChannelEnabled
	originalRequestInterval := common.RequestInterval
	originalMonitorSetting := *operation_setting.GetMonitorSetting()
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	t.Cleanup(func() {
		testAllChannelsLock.Lock()
		testAllChannelsRunning = false
		testAllChannelsLock.Unlock()

		if model.DB != nil && model.DB != originalDB {
			if sqlDB, err := model.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		model.DB = originalDB
		model.LOG_DB = originalLOGDB
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.IsMasterNode = originalIsMasterNode
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		common.AutomaticEnableChannelEnabled = originalAutomaticEnable
		common.RequestInterval = originalRequestInterval
		*operation_setting.GetMonitorSetting() = originalMonitorSetting
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})

	testAllChannelsLock.Lock()
	testAllChannelsRunning = false
	testAllChannelsLock.Unlock()

	common.SQLitePath = "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.IsMasterNode = false
	common.AutomaticDisableChannelEnabled = false
	common.AutomaticEnableChannelEnabled = false
	common.RequestInterval = 0
	operation_setting.GetMonitorSetting().AutoTestOnlyAutoDisabled = true
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":1}`))
	service.InitHttpClient()
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))

	var requestCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(upstream.Close)

	require.NoError(t, model.DB.Create(&model.User{
		Id:       1,
		Username: "root",
		Password: "password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Quota:    1000000,
		Group:    "default",
	}).Error)

	autoBan := 1
	autoTestDisabled := false
	baseURL := upstream.URL
	channel := model.Channel{
		Id:      1,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		Status:  common.ChannelStatusAutoDisabled,
		Name:    "auto-disabled-channel",
		BaseURL: &baseURL,
		Models:  "gpt-4o-mini",
		Group:   "default",
		AutoBan: &autoBan,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AutoTestEnabled: &autoTestDisabled,
	})
	require.NoError(t, model.DB.Create(&channel).Error)

	require.NoError(t, testAllChannels(true))

	require.Eventually(t, func() bool {
		return requestCount.Load() == 1
	}, 3*time.Second, 25*time.Millisecond)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}
