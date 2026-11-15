package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenaiHandlerUsageAndForceFormat(t *testing.T) {
	const toolName = "nonstream_usage_lookup"
	operation_setting.SetToolPriceForTest(toolName, 5)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest(toolName) })
	for _, forceFormat := range []bool{false, true} {
		for _, tc := range []struct {
			name, upstreamUsage string
			prompt, output      int
			cache               int
		}{
			{name: "upstream usage", upstreamUsage: `{"prompt_tokens":91,"completion_tokens":17,"total_tokens":108,"prompt_tokens_details":{"cached_tokens":77},"completion_tokens_details":{"reasoning_tokens":3}}`, prompt: 91, output: 17, cache: 77},
			{name: "local usage", upstreamUsage: `{}`, prompt: 23, cache: 11},
			{name: "local input with upstream output", upstreamUsage: `{"completion_tokens":18}`, prompt: 23, output: 18, cache: 11},
		} {
			t.Run(fmt.Sprintf("%s/force=%t", tc.name, forceFormat), func(t *testing.T) {
				body := fmt.Sprintf(`{"id":"chat_usage","object":"chat.completion","created":123,"model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"provider-model","reasoning_content":"step one","tool_calls":[{"id":"call_1","type":"function","function":{"name":"nonstream_usage_lookup","arguments":"{\"model\":\"provider-model\",\"n\":0}"}}]},"finish_reason":"tool_calls"}],"usage":%s,"timings":{"cache_n":11},"vendor_extension":{"keep":"opaque"}}`, tc.upstreamUsage)
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				info := mappedResponseTestInfo()
				info.RelayFormat = types.RelayFormatOpenAI
				info.ChannelType = constant.ChannelTypeOpenAI
				info.ChannelSetting.ForceFormat = forceFormat
				info.SetEstimatePromptTokens(23)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}

				usage, apiErr := OpenaiHandler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				wantOutput := tc.output
				if wantOutput == 0 {
					wantOutput = service.CountTextToken("provider-modelstep one", info.UpstreamModelName)
				}
				assert.Equal(t, tc.prompt, usage.PromptTokens)
				assert.Equal(t, wantOutput, usage.CompletionTokens)
				assert.Equal(t, tc.prompt+wantOutput, usage.TotalTokens)
				assert.Equal(t, tc.cache, usage.PromptTokensDetails.CachedTokens)
				assert.Equal(t, info.OriginModelName, gjson.Get(w.Body.String(), "model").String())
				assert.Equal(t, "provider-model", gjson.Get(w.Body.String(), "choices.0.message.content").String())
				assert.Equal(t, "step one", gjson.Get(w.Body.String(), "choices.0.message.reasoning_content").String())
				assert.Equal(t, `{"model":"provider-model","n":0}`, gjson.Get(w.Body.String(), "choices.0.message.tool_calls.0.function.arguments").String())
				require.NotNil(t, info.ResponsesUsageInfo)
				require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, toolName)
				assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[toolName].CallCount)
				require.NotNil(t, info.ResponseModelMismatch())
				assert.Equal(t, []string{"provider-model"}, info.ResponseModelMismatch().ResponseModels)

				var downstream dto.OpenAITextResponse
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &downstream))
				assert.Equal(t, *usage, downstream.Usage, "the client and settlement must use identical usage")
				if forceFormat {
					var expected dto.OpenAITextResponse
					require.NoError(t, common.UnmarshalJsonStr(body, &expected))
					expected.Model = info.OriginModelName
					expected.Usage = *usage
					encoded, err := common.Marshal(expected)
					require.NoError(t, err)
					assert.Equal(t, string(encoded), w.Body.String(), "ForceFormat must retain its exact serialized response")
				} else {
					assert.Equal(t, "opaque", gjson.Get(w.Body.String(), "vendor_extension.keep").String())
				}
			})
		}
	}
}

func TestOpenaiHandlerForceFormatKeepsBodyValidation(t *testing.T) {
	for _, body := range []string{
		`{"choices":`,
		`"not an object"`,
		`[]`,
		`{"usage":{"prompt_tokens":"wrong type"}}`,
		// The typed DTO ignores this extension, but the existing generic-map
		// validation rejects numbers outside float64 even in unknown fields.
		`{"choices":[{"message":{"content":"answer"}}],"vendor_extension":{"huge":1e400}}`,
	} {
		t.Run(body, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := mappedResponseTestInfo()
			info.RelayFormat = types.RelayFormatOpenAI
			info.ChannelSetting.ForceFormat = true
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
			usage, apiErr := OpenaiHandler(c, info, resp)
			require.NotNil(t, apiErr)
			assert.Nil(t, usage)
			assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
			assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
			assert.False(t, c.Writer.Written(), "invalid responses must not reach the client or settlement")
		})
	}
}
