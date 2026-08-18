package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestSelectChannelsForAutomaticTestSkipsChannelsWithAutoTestDisabled(t *testing.T) {
	disabled := false
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{
			Id:     2,
			Status: common.ChannelStatusEnabled,
			OtherSettings: string(mustMarshalForChannelTest(t, dto.ChannelOtherSettings{
				AutoTestEnabled: &disabled,
			})),
		},
		{Id: 3, Status: common.ChannelStatusAutoDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 3, selected[1].Id)
}

func TestChannelTestResponseTimeThresholdUsesAutomaticDisableSetting(t *testing.T) {
	originalEnabled := common.AutomaticDisableChannelEnabled
	originalThreshold := common.ChannelDisableThreshold
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalEnabled
		common.ChannelDisableThreshold = originalThreshold
	})

	common.AutomaticDisableChannelEnabled = false
	common.ChannelDisableThreshold = 0.01
	require.Equal(t, time.Duration(0), channelTestResponseTimeThreshold())

	common.AutomaticDisableChannelEnabled = true
	common.ChannelDisableThreshold = 0
	require.Equal(t, time.Duration(0), channelTestResponseTimeThreshold())

	common.ChannelDisableThreshold = 0.01
	require.Equal(t, 10*time.Millisecond, channelTestResponseTimeThreshold())
}

func TestChannelTestContextAppliesDeadline(t *testing.T) {
	ctx, cancel := channelTestContext(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, hasDeadline := ctx.Deadline()
	require.True(t, hasDeadline)

	select {
	case <-ctx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("channel test context did not expire")
	}
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

func TestChannelTestRejectsEmptyResponseWhenEnabled(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	service.InitHttpClient()
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":0.075}`))

	require.NoError(t, db.Create(&model.User{
		Id:       1003,
		Username: "empty-response-test-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)

	requestPaths := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/responses" {
			_, _ = w.Write([]byte(`{"id":"resp-empty","object":"response","status":"completed","output":[],"usage":{"input_tokens":0,"input_tokens_details":{"cached_tokens":0},"output_tokens":0,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":0}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":""}}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`))
	}))
	t.Cleanup(upstream.Close)

	channel := &model.Channel{
		Id:      99,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		Name:    "empty-response-channel",
		BaseURL: common.GetPointer(upstream.URL),
		Models:  "gpt-4o-mini",
		Group:   "default",
	}
	channel.SetSetting(dto.ChannelSettings{RejectEmptyResponse: true})

	tests := []struct {
		name         string
		endpointType constant.EndpointType
		wantPath     string
	}{
		{name: "chat completions", endpointType: constant.EndpointTypeOpenAI, wantPath: "/v1/chat/completions"},
		{name: "responses", endpointType: constant.EndpointTypeOpenAIResponse, wantPath: "/v1/responses"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := testChannel(context.Background(), channel, 1003, "gpt-4o-mini", string(test.endpointType), false)

			require.Error(t, result.localErr)
			require.NotNil(t, result.newAPIError)
			require.Equal(t, types.ErrorCodeEmptyResponse, result.newAPIError.GetErrorCode())
			require.False(t, types.IsSkipRetryError(result.newAPIError))
			select {
			case gotPath := <-requestPaths:
				require.Equal(t, test.wantPath, gotPath)
			default:
				t.Fatal("upstream did not receive the channel test request")
			}
		})
	}
}

func mustMarshalForChannelTest(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}
