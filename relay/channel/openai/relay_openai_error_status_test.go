package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatErrorStatusNormalization(t *testing.T) {
	previousRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 503, End: 503}}
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = previousRanges })

	for _, status := range []int{199, 200, 201, 204, 206, 299, 300, 400, 401, 429, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"上游返回了错误对象","type":"upstream_error","code":"upstream_error","param":"model"}}`)),
			}

			usage, apiErr := OpenaiHandler(c, mappedResponseTestInfo(), resp)
			require.NotNil(t, apiErr)
			assert.Nil(t, usage, "an upstream error must not reach usage settlement")
			wantStatus := status
			if status >= 200 && status < 300 {
				wantStatus = http.StatusServiceUnavailable
			}
			assert.Equal(t, wantStatus, apiErr.StatusCode)
			assert.Equal(t, status, resp.StatusCode, "preserve the original upstream response metadata")
			assert.Equal(t, types.OpenAIError{
				Message: "上游返回了错误对象", Type: "upstream_error", Code: "upstream_error", Param: "model",
			}, apiErr.ToOpenAIError())
			assert.False(t, c.Writer.Written(), "the retry decision must run before sending an error to the client")
			assert.Empty(t, w.Body.String())
			assert.Equal(t, wantStatus == http.StatusServiceUnavailable, service.ShouldRetryRelayError(c, apiErr, 1))
		})
	}
}

func TestOpenAIChatSuccessIsNotAnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "normal answer",
			body: `{"id":"chat_1","object":"chat.completion","model":"public-alias","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
		},
		{
			name: "null top level error",
			body: `{"id":"chat_1","model":"public-alias","error":null,"choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
		},
		{
			name: "error object in answer text",
			body: `{"id":"chat_1","model":"public-alias","choices":[{"index":0,"message":{"role":"assistant","content":"{\"error\":{\"type\":\"upstream_error\",\"message\":\"example\"}}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
		},
		{
			name: "tool arguments contain error",
			body: `{"id":"chat_1","model":"public-alias","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"report","arguments":"{\"error\":\"example\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
		},
		{
			name: "reasoning answer",
			body: `{"id":"chat_1","model":"public-alias","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"check the error example","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := mappedResponseTestInfo()
			info.RelayFormat = types.RelayFormatOpenAI
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			}

			usage, apiErr := OpenaiHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 10, usage.PromptTokens)
			assert.Equal(t, 3, usage.CompletionTokens)
			assert.Equal(t, 13, usage.TotalTokens)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.JSONEq(t, tc.body, w.Body.String())
		})
	}
}

func TestOpenAIChatNormalizedErrorHonorsRetryGuards(t *testing.T) {
	previousRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 503, End: 503}}
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = previousRanges })

	for _, tc := range []struct {
		name    string
		retries int
		setup   func(*gin.Context)
		want    service.PolicyDecision
	}{
		{name: "retry available", retries: 1, want: service.PolicyDecision{Action: "retry", Reason: "retry_status_matched", Source: "global"}},
		{name: "budget exhausted", retries: 0, want: service.PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}},
		{
			name: "client canceled", retries: 1,
			setup: func(c *gin.Context) {
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			},
			want: service.PolicyDecision{Action: "stop", Reason: "client_canceled", Source: "system"},
		},
		{
			name: "downstream already started", retries: 1,
			setup: func(c *gin.Context) {
				_, _ = c.Writer.WriteString(": PING\n\n")
				c.Writer.Flush()
			},
			want: service.PolicyDecision{Action: "stop", Reason: "response_started", Source: "system"},
		},
		{
			name: "channel pinned", retries: 1,
			setup: func(c *gin.Context) { c.Set("specific_channel_id", 76) },
			want:  service.PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if tc.setup != nil {
				tc.setup(c)
			}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporarily unavailable","type":"upstream_error","code":"upstream_error"}}`)),
			}
			_, apiErr := OpenaiHandler(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, resp)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
			assert.Equal(t, tc.want, service.DecideRelayRetry(c, apiErr, tc.retries))
		})
	}
}
