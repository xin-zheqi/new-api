package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareResponsesReasoningRepairRespectsOverridesAndRetries(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "deepseek-v4.1-flash")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeDeepSeek)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ReasoningContentRepair: true})
	original := `[{"type":"reasoning","id":"r1","reasoning_text":"actual thought","vendor":"keep"}]`
	request := &dto.OpenAIResponsesRequest{Model: "deepseek-v4.1-flash", Input: []byte(original), Reasoning: &dto.Reasoning{Effort: "high"}}
	info := relaycommon.GenRelayInfoResponses(c, request)
	for _, tc := range []struct {
		name, effort  string
		enabled, want bool
	}{
		{"enabled", "high", true, true},
		{"override disables", "none", true, false},
		{"next channel disabled", "high", false, false},
		{"retry enabled again", "high", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ReasoningContentRepair: tc.enabled})
			common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"reasoning": map[string]any{"effort": tc.effort}})
			_, body, closer, apiErr := PrepareResponsesRequest(c, info, request)
			require.Nil(t, apiErr)
			defer closer.Close()
			wire, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, tc.want, gjson.GetBytes(wire, "input.0.content.0.text").Exists())
			assert.Equal(t, tc.want, info.ReasoningContentRepairActive)
			assert.Equal(t, "keep", gjson.GetBytes(wire, "input.0.vendor").String())
			assert.Equal(t, original, string(request.Input))
			assert.Equal(t, "high", request.Reasoning.Effort)
		})
	}
}

func TestPrepareResponsesBothHistoryRepairsRespectModelMappingAndRetries(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "local-alias")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	input := `[{"type":"reasoning","id":"old"},{"type":"function_call","call_id":"a","name":"read","arguments":"{}"}]`
	request := &dto.OpenAIResponsesRequest{Model: "local-alias", Input: []byte(input), Reasoning: &dto.Reasoning{Effort: "high"}, Tools: []byte(`[{"type":"function","name":"read","parameters":{"type":"object"}}]`)}
	info := relaycommon.GenRelayInfoResponses(c, request)
	for _, tc := range []struct {
		name, model string
		enabled     bool
		wantNotice  bool
	}{
		{"OpenAI channel mapped to DeepSeek", "deepseek-v4.1-flash", true, true},
		{"retry to disabled channel", "deepseek-v4.1-flash", false, false},
		{"other model only gets schema repair", "gpt-test", true, false},
		{"retry to DeepSeek again", "deepseek-v4.1-flash", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ReasoningContentRepair: tc.enabled, ToolOutputRepair: tc.enabled})
			common.SetContextKey(c, constant.ContextKeyChannelModelMapping, `{"local-alias":"`+tc.model+`"}`)
			_, body, closer, apiErr := PrepareResponsesRequest(c, info, request)
			require.Nil(t, apiErr)
			defer closer.Close()
			wire, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, tc.model, gjson.GetBytes(wire, "model").String())
			assert.Equal(t, tc.wantNotice, gjson.GetBytes(wire, "input.0.content.0.text").Exists())
			assert.Equal(t, input, string(request.Input))
			assert.Equal(t, gjson.Get(input, "1").Raw, gjson.GetBytes(wire, "input.1").Raw)
			if tc.enabled {
				assert.Equal(t, "[]", gjson.GetBytes(wire, "input.0.summary").Raw)
				assert.Equal(t, "a", gjson.GetBytes(wire, "input.2.call_id").String())
				assert.Contains(t, gjson.GetBytes(wire, "input.2.output").String(), "tool_output_unavailable")
			} else {
				assert.Equal(t, input, gjson.GetBytes(wire, "input").Raw)
			}
		})
	}
}
