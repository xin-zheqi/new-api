package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareResponsesToolOutputRepairKeepsRetryInputAndRespectsFinalOverrides(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "deepseek-v4.1-flash")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeDeepSeek)
	input := `[{"type":"function_call","call_id":"a","name":"read","arguments":"{}"}]`
	request := &dto.OpenAIResponsesRequest{Model: "deepseek-v4.1-flash", Input: []byte(input), Reasoning: &dto.Reasoning{Effort: "none"}}
	info := relaycommon.GenRelayInfoResponses(c, request)
	for _, tc := range []struct {
		name     string
		enabled  bool
		override map[string]any
		want     int64
	}{
		{"enabled without thinking", true, nil, 2},
		{"retry through disabled channel", false, nil, 1},
		{"native DeepSeek stateful override", true, map[string]any{"previous_response_id": "resp_remote"}, 2},
		{"next retry enabled again", true, nil, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ToolOutputRepair: tc.enabled})
			common.SetContextKey(c, constant.ContextKeyChannelParamOverride, tc.override)
			_, body, closer, apiErr := PrepareResponsesRequest(c, info, request)
			require.Nil(t, apiErr)
			defer closer.Close()
			wire, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, tc.want, gjson.GetBytes(wire, "input.#").Int())
			assert.Equal(t, input, string(request.Input))
			assert.Empty(t, request.PreviousResponseID)
			assert.False(t, info.ReasoningContentRepairActive)
			if tc.want == 2 {
				require.NotNil(t, info.ToolOutputRepairStats)
				assert.Equal(t, 1, info.ToolOutputRepairStats.MissingOutputs)
			} else {
				assert.Nil(t, info.ToolOutputRepairStats)
			}
		})
	}
}

func TestDoResponsesRequestRepairsProviderMissingToolOutputOnce(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"No tool output found for tool call call_remote"}}`))
			return
		}
		assert.Contains(t, string(body), `"call_id":"call_remote"`)
		assert.Contains(t, string(body), `tool_output_unavailable`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_ok","status":"completed"}`))
	}))
	defer server.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(""))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		RelayMode:               relayconstant.RelayModeResponses,
		RelayFormat:             types.RelayFormatOpenAIResponses,
		RequestURLPath:          "/v1/responses",
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: server.URL,
			ApiKey:         "test-key",
			ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true},
		},
	}
	body, closer, err := relaycommon.NewOutboundJSONBody([]byte(`{"previous_response_id":"resp_remote","input":[{"role":"user","content":"continue"}]}`))
	require.NoError(t, err)
	defer closer.Close()

	response, err := doResponsesRequest(c, info, &openaichannel.Adaptor{}, body)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, int32(2), attempts.Load())
	assert.Equal(t, http.StatusOK, response.(*http.Response).StatusCode)
	_ = response.(*http.Response).Body.Close()
}

func TestPrepareResponsesRepeatedToolCallIDsStayIsolatedAcrossChannels(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "deepseek-v4.1-flash")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeDeepSeek)
	pair := `{"type":"function_call","call_id":"fc_0","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"fc_0","output":"real result"}`
	input := `[` + pair + `,` + pair + `]`
	request := &dto.OpenAIResponsesRequest{Model: "deepseek-v4.1-flash", Input: []byte(input), Reasoning: &dto.Reasoning{Effort: "none"}}
	info := relaycommon.GenRelayInfoResponses(c, request)
	for _, enabled := range []bool{true, false, true} {
		common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ToolOutputRepair: enabled})
		adaptor, body, closer, apiErr := PrepareResponsesRequest(c, info, request)
		require.Nil(t, apiErr)
		wire, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, closer.Close())
		url, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(url, "/responses"), "DeepSeek uses its native Responses endpoint")
		assert.Equal(t, input, string(request.Input))
		assert.Equal(t, "none", gjson.GetBytes(wire, "reasoning.effort").String())
		items := gjson.GetBytes(wire, "input").Array()
		require.Len(t, items, 4)
		assert.Equal(t, items[2].Get("call_id").String(), items[3].Get("call_id").String())
		if enabled {
			assert.NotEqual(t, items[0].Get("call_id").String(), items[2].Get("call_id").String())
			assert.Equal(t, &relaycommon.ToolOutputRepairStats{RenamedCalls: 1}, info.ToolOutputRepairStats)
		} else {
			assert.Equal(t, "fc_0", items[2].Get("call_id").String())
			assert.Nil(t, info.ToolOutputRepairStats)
		}
	}
}
