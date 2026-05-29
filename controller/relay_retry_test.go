package controller

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
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
