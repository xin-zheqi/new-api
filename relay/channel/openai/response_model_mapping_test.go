package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func mappedResponseTestInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: "public-alias",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "provider-model",
			IsModelMapped:     true,
		},
		DisablePing: true,
	}
}

func TestOaiResponsesHandlerHidesMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := []byte(`{"id":"resp_1","model":"provider-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}

	_, apiErr := OaiResponsesHandler(c, mappedResponseTestInfo(), resp)
	require.Nil(t, apiErr)
	require.Equal(t, "public-alias", gjson.Get(w.Body.String(), "model").String())
}

func TestOaiResponsesStreamHandlerHidesMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	events := []string{
		`{"type":"response.created","response":{"id":"resp_1","model":"provider-model","status":"in_progress"}}`,
		`{"type":"response.completed","response":{"id":"resp_1","model":"provider-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: " + strings.Join(events, "\n\ndata: ") + "\n\n")),
	}

	_, apiErr := OaiResponsesStreamHandler(c, mappedResponseTestInfo(), resp)
	require.Nil(t, apiErr)
	modelEvents := 0
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		require.True(t, gjson.Valid(data))
		assert.Equal(t, "public-alias", gjson.Get(data, "response.model").String())
		modelEvents++
	}
	assert.Equal(t, 2, modelEvents, "check both the first and completed event")
	require.NotContains(t, w.Body.String(), "provider-model")
}

func TestClientResponseModelUsesClientNameWithoutLocalMapping(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "public-alias", ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: false}}
	require.Equal(t, "public-alias", relaycommon.ClientResponseModel(info, "provider-model"))
}

func TestConvertedResponsesStreamKeepsClientModelAndCachedUsage(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini} {
		t.Run(string(format), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := mappedResponseTestInfo()
			info.RelayFormat = format
			info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
			info.IsStream, info.ShouldIncludeUsage = true, true
			events := []string{
				`{"type":"response.created","response":{"id":"resp_1","model":"provider-model","status":"in_progress"}}`,
				`{"type":"response.output_text.delta","delta":"provider-model"}`,
				`{"type":"response.completed","response":{"id":"resp_1","model":"provider-model","status":"completed","output":[],"usage":{"input_tokens":4096,"input_tokens_details":{"cached_tokens":3072},"output_tokens":32,"total_tokens":4128}}}`,
			}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("data: " + strings.Join(events, "\n\ndata: ") + "\n\n")),
			}
			usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 4096, usage.PromptTokens)
			assert.Equal(t, 3072, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, 32, usage.CompletionTokens)
			modelEvents := 0
			for _, line := range strings.Split(w.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data:") {
					continue
				}
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data == "[DONE]" {
					continue
				}
				require.True(t, gjson.Valid(data))
				for _, path := range []string{"model", "message.model", "modelVersion"} {
					if model := gjson.Get(data, path); model.Exists() {
						assert.Equal(t, info.OriginModelName, model.String())
						modelEvents++
					}
				}
			}
			if format != types.RelayFormatGemini {
				assert.Positive(t, modelEvents)
			}
			assert.Contains(t, w.Body.String(), `"provider-model"`, "answer text remains unchanged")
			assert.Equal(t, "provider-model", info.UpstreamModelName)
			require.NotNil(t, info.ResponseModelMismatch())
			assert.Equal(t, []string{"provider-model"}, info.ResponseModelMismatch().ResponseModels)
		})
	}
}

func TestTranscriptionNormalizesMetadataButPreservesJSONShapedPlainText(t *testing.T) {
	body := `{"model":"spoken-model-name","text":"spoken-model-name","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`
	for _, format := range []string{"json", "verbose_json", "text", "srt", "vtt"} {
		t.Run(format, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
			info := mappedResponseTestInfo()
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
			apiErr, usage := OpenaiSTTHandler(c, resp, info, format)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 5, usage.TotalTokens)
			if format == "json" || format == "verbose_json" {
				assert.Equal(t, info.OriginModelName, gjson.Get(w.Body.String(), "model").String())
				assert.Equal(t, "spoken-model-name", gjson.Get(w.Body.String(), "text").String())
				require.NotNil(t, info.ResponseModelMismatch())
			} else {
				assert.Equal(t, body, w.Body.String())
				assert.Nil(t, info.ResponseModelMismatch())
			}
		})
	}
}

func TestBufferedResponsesAuditKeepsModelFromEarlierEvent(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := mappedResponseTestInfo()
	info.IsModelMapped = false
	info.RelayFormat = types.RelayFormatOpenAI
	info.UpstreamModelName = info.OriginModelName
	info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: " + `{"type":"response.created","response":{"id":"resp_1","model":"provider-only-in-first-event"}}` + "\n\ndata: " + `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}` + "\n\n")),
	}
	usage, apiErr := OaiResponsesToChatBufferedStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.TotalTokens)
	assert.Equal(t, info.OriginModelName, gjson.Get(w.Body.String(), "model").String())
	require.NotNil(t, info.ResponseModelMismatch())
	assert.Equal(t, []string{"provider-only-in-first-event"}, info.ResponseModelMismatch().ResponseModels)
}

func TestChatAndCompactModelMappingPreservesPayloadAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, body, payloadPath string
		format                  types.RelayFormat
		handler                 func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{
			name: "Chat", format: types.RelayFormatOpenAI, handler: OpenaiHandler,
			body:        `{"id":"chat_1","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"provider-model"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5},"extension":{"model":"keep-nested"}}`,
			payloadPath: "choices.0.message.content",
		},
		{
			name: "Compact", format: types.RelayFormatOpenAIResponsesCompaction, handler: OaiResponsesCompactionHandler,
			body:        `{"id":"cmp_1","object":"response.compaction","model":"provider-model","output":[{"type":"compaction","encrypted_content":"provider-model"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5},"extension":{"model":"keep-nested"}}`,
			payloadPath: "output.0.encrypted_content",
		},
	} {
		for _, mapped := range []bool{true, false} {
			name := tc.name + "/unmapped"
			if mapped {
				name = tc.name + "/mapped"
			}
			t.Run(name, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				info := mappedResponseTestInfo()
				info.RelayFormat, info.IsModelMapped = tc.format, mapped
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}
				usage, apiErr := tc.handler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, 2, usage.PromptTokens)
				assert.Equal(t, 3, usage.CompletionTokens)
				assert.Equal(t, 5, usage.TotalTokens)
				wantModel := "public-alias"
				require.NotNil(t, info.ResponseModelMismatch())
				assert.Equal(t, []string{"provider-model"}, info.ResponseModelMismatch().ResponseModels)
				assert.Equal(t, wantModel, gjson.Get(w.Body.String(), "model").String())
				assert.Equal(t, "provider-model", gjson.Get(w.Body.String(), tc.payloadPath).String())
				assert.Equal(t, "keep-nested", gjson.Get(w.Body.String(), "extension.model").String())
				assert.Equal(t, "provider-model", info.UpstreamModelName, "routing and accounting metadata must remain intact")
			})
		}
	}
}

func TestChatStreamModelMappingIncludesFinalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, forceFormat := range []bool{false, true} {
		name := "raw"
		if forceFormat {
			name = "force format"
		}
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := mappedResponseTestInfo()
			info.RelayFormat = types.RelayFormatOpenAI
			info.IsStream, info.ShouldIncludeUsage = true, true
			info.ChannelSetting.ForceFormat = forceFormat
			events := []string{
				`{"id":"chat_1","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","content":"provider-model"}}]}`,
				`{"id":"chat_1","model":"provider-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`{"id":"chat_1","model":"provider-model","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
				`[DONE]`,
			}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("data: " + strings.Join(events, "\n\ndata: ") + "\n\n")),
			}
			usage, apiErr := OaiStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 2, usage.PromptTokens)
			assert.Equal(t, 3, usage.CompletionTokens)
			assert.Equal(t, 5, usage.TotalTokens)
			modelEvents, usageEvents := 0, 0
			for _, line := range strings.Split(w.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data:") {
					continue
				}
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data == "[DONE]" {
					continue
				}
				require.True(t, gjson.Valid(data), "SSE frame must remain valid JSON: %s", data)
				assert.Equal(t, "public-alias", gjson.Get(data, "model").String())
				modelEvents++
				if gjson.Get(data, "usage.total_tokens").Int() == 5 {
					usageEvents++
				}
			}
			assert.GreaterOrEqual(t, modelEvents, 2)
			assert.Equal(t, 1, usageEvents)
			assert.Contains(t, w.Body.String(), `"content":"provider-model"`, "answer text must not be rewritten")
			assert.Equal(t, "provider-model", info.UpstreamModelName)
		})
	}
}
