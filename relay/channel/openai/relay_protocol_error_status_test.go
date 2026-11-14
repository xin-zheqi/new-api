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
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIProtocolErrorStatusNormalization(t *testing.T) {
	previousRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 503, End: 503}}
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = previousRanges })

	for _, handler := range []struct {
		name string
		run  func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{name: "responses to chat", run: OaiResponsesToChatHandler},
		{name: "chat to responses", run: OaiChatToResponsesHandler},
		{name: "native responses", run: OaiResponsesHandler},
		{name: "responses compaction", run: OaiResponsesCompactionHandler},
	} {
		t.Run(handler.name, func(t *testing.T) {
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

					usage, apiErr := handler.run(c, mappedResponseTestInfo(), resp)
					require.NotNil(t, apiErr)
					assert.Nil(t, usage, "an identified error must not reach usage settlement")
					wantStatus := status
					if status >= http.StatusOK && status < http.StatusMultipleChoices {
						wantStatus = http.StatusServiceUnavailable
					}
					assert.Equal(t, wantStatus, apiErr.StatusCode)
					assert.Equal(t, status, resp.StatusCode, "preserve the upstream HTTP metadata")
					assert.Equal(t, types.OpenAIError{
						Message: "上游返回了错误对象", Type: "upstream_error", Code: "upstream_error", Param: "model",
					}, apiErr.ToOpenAIError())
					assert.False(t, c.Writer.Written(), "retry must remain possible before sending a response")
					assert.Empty(t, w.Body.String())
					assert.Equal(t, wantStatus == http.StatusServiceUnavailable, service.ShouldRetryRelayError(c, apiErr, 1))
				})
			}
		})
	}
}

func TestOpenAIProtocolSuccessIsNotAnError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		run    func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
		format types.RelayFormat
		body   string
		path   string
		want   string
	}{
		{
			name: "responses to chat keeps error example as text", run: OaiResponsesToChatHandler, format: types.RelayFormatOpenAI,
			body: `{"id":"resp_1","model":"public-alias","status":"completed","error":null,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"error\":{\"type\":\"upstream_error\"}}"}]}],"usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}`,
			path: "choices.0.message.content", want: `{"error":{"type":"upstream_error"}}`,
		},
		{
			name: "responses to chat keeps tool error argument", run: OaiResponsesToChatHandler, format: types.RelayFormatOpenAI,
			body: `{"id":"resp_1","model":"public-alias","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"report","arguments":"{\"error\":\"example\"}"}],"usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}`,
			path: "choices.0.message.tool_calls.0.function.arguments", want: `{"error":"example"}`,
		},
		{
			name: "chat to responses keeps error example as text", run: OaiChatToResponsesHandler, format: types.RelayFormatOpenAIResponses,
			body: `{"id":"chat_1","model":"public-alias","error":null,"choices":[{"index":0,"message":{"role":"assistant","content":"{\"error\":{\"type\":\"upstream_error\"}}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
			path: "output.0.content.0.text", want: `{"error":{"type":"upstream_error"}}`,
		},
		{
			name: "chat to responses keeps tool error argument", run: OaiChatToResponsesHandler, format: types.RelayFormatOpenAIResponses,
			body: `{"id":"chat_1","model":"public-alias","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"report","arguments":"{\"error\":\"example\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
			path: "output.0.arguments", want: `{"error":"example"}`,
		},
		{
			name: "native responses keeps opaque reasoning", run: OaiResponsesHandler, format: types.RelayFormatOpenAIResponses,
			body: `{"id":"resp_1","model":"public-alias","status":"completed","error":null,"output":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"gAAAAAB+/=error"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}`,
			path: "output.0.encrypted_content", want: "gAAAAAB+/=error",
		},
		{
			name: "compaction keeps opaque output", run: OaiResponsesCompactionHandler, format: types.RelayFormatOpenAIResponses,
			body: `{"id":"cmp_1","object":"response.compaction","error":null,"output":[{"type":"compaction","id":"cmp_item_1","encrypted_content":"gAAAAAB+/=error"}],"usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}`,
			path: "output.0.encrypted_content", want: "gAAAAAB+/=error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := mappedResponseTestInfo()
			info.RelayFormat = tc.format
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			}

			usage, apiErr := tc.run(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 7, usage.PromptTokens)
			assert.Equal(t, 3, usage.CompletionTokens)
			assert.Equal(t, 10, usage.TotalTokens)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tc.want, gjson.Get(w.Body.String(), tc.path).String())
		})
	}
}

func TestResponsesToChatNormalizedErrorHonorsRetryGuards(t *testing.T) {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if tc.setup != nil {
				tc.setup(c)
			}
			before := w.Body.String()
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporarily unavailable","type":"upstream_error","code":"upstream_error"}}`)),
			}

			usage, apiErr := OaiResponsesToChatHandler(c, mappedResponseTestInfo(), resp)
			require.NotNil(t, apiErr)
			assert.Nil(t, usage)
			require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
			assert.Equal(t, before, w.Body.String())
			assert.Equal(t, tc.want, service.DecideRelayRetry(c, apiErr, tc.retries))
		})
	}
}
