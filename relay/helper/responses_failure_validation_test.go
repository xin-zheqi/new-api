package helper

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateResponsesFailureBeforeOutput(t *testing.T) {
	tests := []struct {
		name, payload, code string
		status              int
		skipRetry           bool
	}{
		{
			name:    "reported upstream failure without error type",
			payload: `{"type":"response.failed","sequence_number":0,"response":{"object":"response","status":"failed","output":[],"error":{"code":"upstream_error","message":"Upstream request failed"}}}`,
			code:    "upstream_error", status: http.StatusBadGateway,
		},
		{
			name: "flat Responses error event", payload: `{"type":"error","code":"rate_limit_exceeded","message":"try later","param":"model"}`,
			code: "rate_limit_exceeded", status: http.StatusTooManyRequests,
		},
		{
			name: "request error stays non retryable", payload: `{"type":"response.failed","response":{"error":{"code":"context_length_exceeded","type":"server_error","message":"too long"}}}`,
			code: "context_length_exceeded", status: http.StatusBadRequest, skipRetry: true,
		},
		{
			name: "safety rejection stays non retryable", payload: `{"type":"response.failed","response":{"error":{"code":"content_policy_violation","message":"blocked"}}}`,
			code: "content_policy_violation", status: http.StatusBadRequest, skipRetry: true,
		},
		{
			name: "unknown provider code is not assumed transient", payload: `{"type":"response.failed","response":{"error":{"code":"provider_specific_failure","type":"server_error","message":"details"}}}`,
			code: "provider_specific_failure", status: http.StatusBadGateway, skipRetry: true,
		},
		{
			name: "type only error classification", payload: `{"type":"response.failed","response":{"error":{"type":"overloaded_error","message":"busy"}}}`,
			code: "response_failed", status: http.StatusServiceUnavailable,
		},
		{
			name: "failed event without error details", payload: `{"type":"response.failed","response":{"status":"failed","output":[]}}`,
			code: "response_failed", status: http.StatusBadGateway, skipRetry: true,
		},
		{
			name: "nonstream Responses failure", payload: `{"object":"response","status":"failed","output":[],"error":{"code":"upstream_error","message":"Upstream request failed"}}`,
			code: "upstream_error", status: http.StatusBadGateway,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, stream := range []bool{false, true} {
				body, contentType := tt.payload, "application/json"
				if stream {
					body = "data: {\"type\":\"response.created\",\"response\":{\"status\":\"in_progress\"}}\n\n" + "data: " + tt.payload + "\n\n"
					contentType = "text/event-stream"
				}
				info := emptyResponseTestInfo(true, stream)
				info.RelayMode = relayconstant.RelayModeResponses
				apiErr := ValidateUpstreamTextResponse(emptyResponseTestHTTPResponse(body, contentType), info)
				require.NotNil(t, apiErr)
				assert.Equal(t, types.ErrorCode(tt.code), apiErr.GetErrorCode())
				assert.Equal(t, tt.status, apiErr.StatusCode)
				assert.Equal(t, tt.skipRetry, types.IsSkipRetryError(apiErr))
				assert.True(t, types.IsRecordErrorLog(apiErr))
				assert.NotEmpty(t, apiErr.Error())
			}
		})
	}
}

func TestValidateResponsesFailurePreservesGeneratedOutputAndOtherOutcomes(t *testing.T) {
	failed := `{"type":"response.failed","response":{"status":"failed","error":{"code":"upstream_error","message":"failed"}}}`
	tests := []struct {
		name, body string
		enabled    bool
	}{
		{"disabled retains original stream", "data: " + failed + "\n\n", false},
		{"text before failure is not replayed", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: " + failed + "\n\n", true},
		{"tool call before failure is not replayed", "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"name\":\"charge_card\",\"arguments\":\"{}\"}}\n\ndata: " + failed + "\n\n", true},
		{"failure with generated usage is preserved", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"upstream_error\"},\"usage\":{\"output_tokens\":3}}}\n\n", true},
		{"failure with output is preserved", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"upstream_error\"},\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"partial answer\"}]}]}}\n\n", true},
		{"incomplete is not a transient failure", "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n", true},
		{"cancelled is preserved", "data: {\"type\":\"response.cancelled\"}\n\n", true},
		{"canceled is preserved", "data: {\"type\":\"response.canceled\"}\n\n", true},
		{"provider error envelope stays with adaptor", "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n", true},
		{"error inside output is content", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"error\\\":\\\"upstream_error\\\"}\"}\n\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := emptyResponseTestInfo(tt.enabled, true)
			info.RelayMode = relayconstant.RelayModeResponses
			resp := emptyResponseTestHTTPResponse(tt.body, "text/event-stream")
			require.Nil(t, ValidateUpstreamTextResponse(resp, info))
			preserved, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.body, string(preserved))
		})
	}
}

func TestValidateResponsesFailurePreservesOriginalErrorAndClosesStream(t *testing.T) {
	body := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"upstream_error\",\"message\":\"Upstream request failed\"},\"output\":[]}}\n\n"
	resp := emptyResponseTestHTTPResponse(body, "text/event-stream")
	reader := &failureStreamBody{Reader: strings.NewReader(body)}
	resp.Body = reader
	apiErr := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(true, true))
	require.NotNil(t, apiErr)
	assert.True(t, reader.closed)
	assert.Equal(t, "Upstream request failed", apiErr.Error())
	assert.Equal(t, "upstream_error", apiErr.ToOpenAIError().Code)
	encoded, err := common.Marshal(apiErr.ToOpenAIError())
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"message":"Upstream request failed"`)
}

type failureStreamBody struct {
	io.Reader
	closed bool
}

func (b *failureStreamBody) Close() error {
	b.closed = true
	return nil
}
