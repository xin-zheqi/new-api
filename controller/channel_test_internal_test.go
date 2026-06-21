package controller

import (
	"errors"
	"io"
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

func TestAutomaticTestAllChannelsDisablesErrorWithoutDisableRules(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalSQLitePath := common.SQLitePath
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
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
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
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
	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
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
