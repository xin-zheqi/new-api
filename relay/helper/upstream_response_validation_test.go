package helper

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emptyResponseTestInfo(enabled, stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		IsStream:  stream,
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{RejectEmptyResponse: enabled},
		},
	}
}

func emptyResponseTestHTTPResponse(body, contentType string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestValidateUpstreamTextResponseNonStream(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		enabled   bool
		wantCode  types.ErrorCode
		wantPass  bool
		wantRetry bool
	}{
		{
			name:     "disabled keeps historical behavior",
			body:     `{"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`,
			wantPass: true,
		},
		{
			name:      "empty chat response",
			enabled:   true,
			body:      `{"choices":[{"message":{"role":"assistant","content":""}}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`,
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:      "whitespace is empty",
			enabled:   true,
			body:      `{"choices":[{"message":{"content":"  \n "}}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`,
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:     "text is substantive",
			enabled:  true,
			body:     `{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`,
			wantPass: true,
		},
		{
			name:     "tool call is substantive",
			enabled:  true,
			body:     `{"choices":[{"message":{"content":null,"tool_calls":[{"type":"function","function":{"name":"lookup","arguments":"{}"}}]}}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`,
			wantPass: true,
		},
		{
			name:      "empty chat tool call placeholder is not substantive",
			enabled:   true,
			body:      `{"choices":[{"message":{"content":null,"tool_calls":[{"index":0,"type":"function","function":{"name":"","arguments":""}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`,
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:      "empty Responses function call placeholder is not substantive",
			enabled:   true,
			body:      `{"status":"completed","output":[{"id":"fc-empty","type":"function_call","status":"completed","name":"","arguments":""}],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:     "refusal is substantive",
			enabled:  true,
			body:     `{"choices":[{"message":{"content":null,"refusal":"I cannot help with that."}}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`,
			wantPass: true,
		},
		{
			name:     "nonzero upstream usage is not empty",
			enabled:  true,
			body:     `{"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":0,"total_tokens":12}}`,
			wantPass: true,
		},
		{
			name:      "zero primary usage is not overridden by cache counters",
			enabled:   true,
			body:      `{"id":"msg-empty","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":846,"output_tokens":0}}`,
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:     "responses output text",
			enabled:  true,
			body:     `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,
			wantPass: true,
		},
		{
			name:     "claude text",
			enabled:  true,
			body:     `{"content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":0,"output_tokens":0}}`,
			wantPass: true,
		},
		{
			name:     "gemini text",
			enabled:  true,
			body:     `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0}}`,
			wantPass: true,
		},
		{
			name:      "metadata strings are not content",
			enabled:   true,
			body:      `{"metadata":["request accepted"],"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0}}`,
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:      "echoed request configuration is not content",
			enabled:   true,
			body:      `{"status":"completed","output":[],"instructions":[{"content":"original prompt"}],"metadata":{"content":"request metadata","error":{"message":"metadata only"}},"tools":[{"type":"function","parameters":{"properties":{"error":{"type":"string"}},"examples":[{"answer":"example only"}]}}],"text":{"format":{"type":"json_schema","schema":{"properties":{"error":{"type":"string"}},"examples":[{"answer":"example only"}]}}},"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:      "token budget is not token usage",
			enabled:   true,
			body:      `{"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"token_budget":8192}}`,
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:     "nested provider output is substantive",
			enabled:  true,
			body:     `{"data":{"result":{"answer":"hello"}},"usage":{"input_tokens":0,"output_tokens":0}}`,
			wantPass: true,
		},
		{
			name:     "reasoning summary is substantive",
			enabled:  true,
			body:     `{"output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"working"}]}],"usage":{"input_tokens":0,"output_tokens":0}}`,
			wantPass: true,
		},
		{
			name:     "media output is substantive",
			enabled:  true,
			body:     `{"choices":[{"message":{"audio":{"id":"audio-1","data":"AA=="}}}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`,
			wantPass: true,
		},
		{
			name:     "structured upstream error remains adaptor responsibility",
			enabled:  true,
			body:     `{"error":{"code":"server_error","message":"try later"}}`,
			wantPass: true,
		},
		{
			name:     "gemini prompt block remains adaptor responsibility",
			enabled:  true,
			body:     `{"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0}}`,
			wantPass: true,
		},
		{
			name:      "empty newline delimited response",
			enabled:   true,
			body:      "{\"message\":{\"content\":\"\"}}\n{\"done\":true,\"prompt_eval_count\":0,\"eval_count\":0}\n",
			wantCode:  types.ErrorCodeEmptyResponse,
			wantRetry: true,
		},
		{
			name:      "cyber policy",
			enabled:   true,
			body:      `{"response":{"error":{"code":"cyber_policy","message":"blocked by network policy"}},"usage":{"input_tokens":0,"output_tokens":0}}`,
			wantCode:  types.ErrorCodeCyberPolicy,
			wantRetry: false,
		},
		{
			name:     "malformed response remains adaptor responsibility",
			enabled:  true,
			body:     `not-json`,
			wantPass: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp := emptyResponseTestHTTPResponse(test.body, "application/json")
			err := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(test.enabled, false))
			if test.wantPass {
				require.Nil(t, err)
				preserved, readErr := io.ReadAll(resp.Body)
				require.NoError(t, readErr)
				assert.Equal(t, test.body, string(preserved))
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, test.wantCode, err.GetErrorCode())
			assert.Equal(t, test.wantRetry, !types.IsSkipRetryError(err))
		})
	}
}

func TestValidateUpstreamTextResponseSupportsResponsesCompact(t *testing.T) {
	resp := emptyResponseTestHTTPResponse(
		`{"output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,
		"application/json",
	)
	info := emptyResponseTestInfo(true, false)
	info.RelayMode = relayconstant.RelayModeResponsesCompact

	err := ValidateUpstreamTextResponse(resp, info)

	require.NotNil(t, err)
	assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
}

func TestValidateUpstreamTextResponseRejectsEmptyResponsesPayload(t *testing.T) {
	resp := emptyResponseTestHTTPResponse(
		`{"id":"resp-empty","object":"response","status":"completed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,
		"application/json",
	)
	info := emptyResponseTestInfo(true, false)
	info.RelayMode = relayconstant.RelayModeResponses

	err := ValidateUpstreamTextResponse(resp, info)

	require.NotNil(t, err)
	assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	assert.False(t, types.IsSkipRetryError(err))
}

func TestValidateUpstreamTextResponseRejectsNilBody(t *testing.T) {
	info := emptyResponseTestInfo(true, false)
	resp := &http.Response{StatusCode: http.StatusOK}

	err := ValidateUpstreamTextResponse(resp, info)

	require.NotNil(t, err)
	assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
}

func TestValidateUpstreamTextResponseStream(t *testing.T) {
	validStream := strings.Join([]string{
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"role":"assistant","content":""}}]}`,
		"",
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"hello"}}]}`,
		"",
		`data: {"id":"chatcmpl-1","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	emptyStream := strings.Join([]string{
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"role":"assistant","content":""}}]}`,
		"",
		`data: {"id":"chatcmpl-1","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	cyberStream := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp-1"}}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","response":{"error":{"code":"cyber_policy","message":"blocked"},"usage":{"input_tokens":0,"output_tokens":0}}}`,
		"",
	}, "\n")
	errorStream := strings.Join([]string{
		`event: response.failed`,
		`data: {"type":"response.failed","response":{"error":{"code":"server_error","message":"try later"}}}`,
		"",
	}, "\n")
	malformedStream := "data: not-json\n\ndata: [DONE]\n\n"
	responsesEmptyStream := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp-empty"}}`,
		"",
		`event: response.done`,
		`data: {"type":"response.done","response":{"status":"completed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		"",
	}, "\n")
	responsesTextStream := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		"",
		`data: {"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		"",
	}, "\n")
	claudeCachedEmptyStream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg-empty","content":[],"usage":{"input_tokens":0,"cache_read_input_tokens":846,"output_tokens":0}}}`,
		"",
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")
	responsesEmptyCallStream := strings.Join([]string{
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added","item":{"id":"fc-empty","type":"function_call","status":"in_progress","name":"","arguments":""}}`,
		"",
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"id":"fc-empty","type":"function_call","status":"completed","name":"","arguments":""}],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		"",
	}, "\n")
	responsesMultilineEmptyStream := strings.Join([]string{
		`event: response.completed`,
		`data: {`,
		`data:   "type": "response.completed",`,
		`data:   "response": {`,
		`data:     "status": "completed",`,
		`data:     "output": [],`,
		`data:     "usage": {"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}`,
		`data:   }`,
		`data: }`,
		"",
	}, "\n")
	responsesMultilineTextStream := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {`,
		`data:   "type": "response.output_text.delta",`,
		`data:   "delta": "hello"`,
		`data: }`,
		"",
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		"",
	}, "\n")
	incrementalToolCallStream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"","arguments":""}}]}}]}`,
		"",
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":""}}]}}]}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	t.Run("releases complete prefix after first content", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(validStream, "text/event-stream")
		err := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(true, true))
		require.Nil(t, err)
		preserved, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)
		assert.Equal(t, validStream, string(preserved))
	})

	t.Run("preserves explicit stream errors for the adaptor", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(errorStream, "text/event-stream")
		err := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(true, true))
		require.Nil(t, err)
		preserved, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)
		assert.Equal(t, errorStream, string(preserved))
	})

	t.Run("preserves unknown stream formats for the adaptor", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(malformedStream, "text/event-stream")
		err := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(true, true))
		require.Nil(t, err)
		preserved, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)
		assert.Equal(t, malformedStream, string(preserved))
	})

	t.Run("rejects empty stream before downstream write", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(emptyStream, "text/event-stream")
		err := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(true, true))
		require.NotNil(t, err)
		assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
		assert.False(t, types.IsSkipRetryError(err))
	})

	t.Run("cyber stream is explicit and non retryable", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(cyberStream, "text/event-stream")
		err := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(true, true))
		require.NotNil(t, err)
		assert.Equal(t, types.ErrorCodeCyberPolicy, err.GetErrorCode())
		assert.True(t, types.IsSkipRetryError(err))
	})

	t.Run("rejects empty Responses stream", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(responsesEmptyStream, "text/event-stream")
		info := emptyResponseTestInfo(true, true)
		info.RelayMode = relayconstant.RelayModeResponses

		err := ValidateUpstreamTextResponse(resp, info)

		require.NotNil(t, err)
		assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
		assert.False(t, types.IsSkipRetryError(err))
	})

	t.Run("allows substantive Responses stream with zero usage", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(responsesTextStream, "text/event-stream")
		info := emptyResponseTestInfo(true, true)
		info.RelayMode = relayconstant.RelayModeResponses

		err := ValidateUpstreamTextResponse(resp, info)

		require.Nil(t, err)
		preserved, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)
		assert.Equal(t, responsesTextStream, string(preserved))
	})

	t.Run("detects mislabeled empty Responses stream", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(responsesEmptyStream, "application/json")
		info := emptyResponseTestInfo(true, false)
		info.RelayMode = relayconstant.RelayModeResponses

		err := ValidateUpstreamTextResponse(resp, info)

		require.NotNil(t, err)
		assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	})

	t.Run("rejects cached Claude stream with zero primary usage", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(claudeCachedEmptyStream, "text/event-stream")
		err := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(true, true))

		require.NotNil(t, err)
		assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	})

	t.Run("rejects empty Responses function call stream", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(responsesEmptyCallStream, "text/event-stream")
		info := emptyResponseTestInfo(true, true)
		info.RelayMode = relayconstant.RelayModeResponses

		err := ValidateUpstreamTextResponse(resp, info)

		require.NotNil(t, err)
		assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	})

	t.Run("rejects empty multiline SSE event", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(responsesMultilineEmptyStream, "text/event-stream")
		info := emptyResponseTestInfo(true, true)
		info.RelayMode = relayconstant.RelayModeResponses

		err := ValidateUpstreamTextResponse(resp, info)

		require.NotNil(t, err)
		assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	})

	t.Run("allows substantive multiline SSE event", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(responsesMultilineTextStream, "text/event-stream")
		info := emptyResponseTestInfo(true, true)
		info.RelayMode = relayconstant.RelayModeResponses

		err := ValidateUpstreamTextResponse(resp, info)

		require.Nil(t, err)
		preserved, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)
		assert.Equal(t, responsesMultilineTextStream, string(preserved))
	})

	t.Run("buffers empty tool placeholder until call payload arrives", func(t *testing.T) {
		resp := emptyResponseTestHTTPResponse(incrementalToolCallStream, "text/event-stream")
		err := ValidateUpstreamTextResponse(resp, emptyResponseTestInfo(true, true))

		require.Nil(t, err)
		preserved, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)
		assert.Equal(t, incrementalToolCallStream, string(preserved))
	})
}
