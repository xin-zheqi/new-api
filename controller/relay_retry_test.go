package controller

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
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
