package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRetry_UsesConfiguredStatusCodes(t *testing.T) {
	orig := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = orig })

	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 429, End: 429},
		{Start: 500, End: 599},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.True(t, shouldRetry(c, types.NewOpenAIError(errors.New("gateway timeout"), types.ErrorCodeBadResponseStatusCode, 504), 1))
	require.True(t, shouldRetry(c, types.NewOpenAIError(errors.New("cloudflare timeout"), types.ErrorCodeBadResponseStatusCode, 524), 1))
	require.False(t, shouldRetry(c, types.NewOpenAIError(errors.New("bad request"), types.ErrorCodeBadResponseStatusCode, 400), 1))
	require.False(t, shouldRetry(c, types.NewOpenAIError(errors.New("request timeout"), types.ErrorCodeBadResponseStatusCode, 408), 1))
}

func TestShouldRetry_AllowsCrossGroupAfterLocalRetriesExhausted(t *testing.T) {
	orig := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = orig })

	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 500, End: 599},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(c, constant.ContextKeyCrossGroupExhausted, false)

	err := types.NewOpenAIError(errors.New("gateway timeout"), types.ErrorCodeBadResponseStatusCode, 504)
	require.True(t, shouldRetry(c, err, 0))

	common.SetContextKey(c, constant.ContextKeyCrossGroupExhausted, true)
	require.False(t, shouldRetry(c, err, 0))
}

func TestShouldRetry_RetriesFirstResponseTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	err := types.NewOpenAIError(
		errors.New("upstream first response timeout after 30s"),
		types.ErrorCodeChannelFirstResponseTimeout,
		502,
	)

	require.True(t, shouldRetry(c, err, 1))
}

func TestShouldRetry_DoesNotRetryCyberPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	err := types.NewOpenAIError(
		errors.New("blocked by network policy"),
		types.ErrorCodeCyberPolicy,
		http.StatusInternalServerError,
	)

	require.False(t, shouldRetry(c, err, 1))
}

func TestShouldRetry_RetriesEmptyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	err := types.NewOpenAIError(
		errors.New("empty upstream response"),
		types.ErrorCodeEmptyResponse,
		http.StatusBadGateway,
	)

	require.True(t, shouldRetry(c, err, 1))
	require.False(t, shouldRetry(c, err, 0))
}

func TestShouldRetry_DoesNotRetryAfterClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)

	err := types.NewOpenAIError(
		errors.New("upstream error: do request failed"),
		types.ErrorCodeDoRequestFailed,
		500,
	)

	require.False(t, shouldRetry(c, err, 1))
	require.False(t, shouldRetry(c, newClientCanceledRelayError(c), 1))
}

func TestIsClientCanceledRelayError(t *testing.T) {
	require.True(t, isClientCanceledRelayError(newClientCanceledRelayError(nil)))
	require.True(t, isClientCanceledRelayError(types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed)))
	require.False(t, isClientCanceledRelayError(types.NewOpenAIError(errors.New("bad gateway"), types.ErrorCodeBadResponseStatusCode, 502)))
	require.False(t, isClientCanceledRelayError(nil))
}

func TestShouldRetry_ResponsesFailureHonorsRequestState(t *testing.T) {
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = originalRanges })
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		code         string
		retries      int
		started      bool
		canceled     bool
		pinned       bool
		disableRetry bool
		wantRetry    bool
	}{
		{name: "transient failure before output", code: "upstream_error", retries: 1, wantRetry: true},
		{name: "retry budget exhausted", code: "upstream_error"},
		{name: "response already committed", code: "upstream_error", retries: 1, started: true},
		{name: "downstream disconnected", code: "upstream_error", retries: 1, canceled: true},
		{name: "channel pinned", code: "upstream_error", retries: 1, pinned: true},
		{name: "configured retry disabled", code: "upstream_error", retries: 1, disableRetry: true},
		{name: "invalid input even when 400 configured", code: "context_length_exceeded", retries: 1},
		{name: "safety rejection even when 400 configured", code: "content_policy_violation", retries: 1},
		{name: "unknown failure stays terminal", code: "vendor_failure", retries: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 400, End: 599}}
			if tt.disableRetry {
				operation_setting.AutomaticRetryStatusCodeRanges = nil
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			requestContext, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)
			if tt.canceled {
				cancel()
			}
			if tt.pinned {
				c.Set("specific_channel_id", 1)
			}
			if tt.started {
				_, err := c.Writer.WriteString("data: already sent\n\n")
				require.NoError(t, err)
			}
			body := "data: {\"type\":\"response.created\",\"response\":{\"status\":\"in_progress\"}}\n\n" +
				"data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"output\":[],\"error\":{\"code\":\"" + tt.code + "\",\"message\":\"Upstream request failed\"}}}\n\n"
			response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
			info := &relaycommon.RelayInfo{
				IsStream: true, RelayMode: relayconstant.RelayModeResponses,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: kitdto.ChannelSettings{RejectEmptyResponse: true}},
			}
			apiErr := helper.ValidateUpstreamTextResponse(response, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, types.ErrorCode(tt.code), apiErr.GetErrorCode())
			assert.Equal(t, tt.wantRetry, shouldRetry(c, apiErr, tt.retries))
			if !tt.started {
				assert.False(t, c.Writer.Written(), "buffered failure must not commit the response before retry")
				assert.Empty(t, recorder.Body.String())
			}
		})
	}
}

func TestShouldRetryTaskRelay_UsesConfiguredStatusCodes(t *testing.T) {
	orig := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = orig })

	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 429, End: 429},
		{Start: 500, End: 599},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.True(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: 504}, 1))
	require.True(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: 524}, 1))
	require.False(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: 400}, 1))
	require.False(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: 408}, 1))
}

func TestShouldRetryTaskRelay_DoesNotRetryLocalErrors(t *testing.T) {
	orig := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = orig })

	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 500, End: 599},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.False(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: 504, LocalError: true}, 1))
}

func TestGetActualLogGroup_PrefersSelectedAutoGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default,vip")
	common.SetContextKey(c, constant.ContextKeyAutoGroup, "vip")

	require.Equal(t, "vip", getActualLogGroup(c))
}

func TestGetActualLogGroup_FallsBackToUsingGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")

	require.Equal(t, "default", getActualLogGroup(c))
}

func TestWriteRelayErrorResponse_SkipsIfStreamAlreadyStarted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	helper.SetEventStreamHeaders(c)
	err := helper.StringData(c, `{"type":"message_start"}`)
	require.NoError(t, err)

	apiErr := types.NewOpenAIError(errors.New("upstream stream broken"), types.ErrorCodeBadResponseBody, 502)
	writeRelayErrorResponse(c, types.RelayFormatClaude, nil, apiErr)

	body := rec.Body.String()
	require.Contains(t, body, `data: {"type":"message_start"}`)
	require.False(t, strings.Contains(body, `"type":"error"`), "must not append JSON error after SSE stream started")
}

func TestWriteRelayErrorResponse_WritesClaudeErrorBeforeResponseStarts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	apiErr := types.NewOpenAIError(errors.New("bad gateway"), types.ErrorCodeBadResponseStatusCode, 502)
	writeRelayErrorResponse(c, types.RelayFormatClaude, nil, apiErr)

	require.Equal(t, 502, rec.Code)
	require.Contains(t, rec.Body.String(), `"type":"error"`)
	require.Contains(t, rec.Body.String(), `"error"`)
}
