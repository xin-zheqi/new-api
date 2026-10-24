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

// The gateway cannot validate provider ciphertext. Its contract is to preserve
// complete opaque history items across mapping, field filtering and retries.
func TestPrepareResponsesPreservesEncryptedHistory(t *testing.T) {
	const input = `[{"type":"reasoning","id":"rs_fixture","summary":[],"encrypted_content":"gAAAAAB-model+/=\\opaque\u0026"},{"type":"compaction","id":"cmp_fixture","encrypted_content":"gAAAAAB-compact+/="},{"type":"message","role":"assistant","id":"amsg_fixture","content":[{"type":"output_text","text":"original"}],"encrypted_content":"gAAAAAB-message+/="},{"role":"user","content":"continue"}]`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "public-alias")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	request := &dto.OpenAIResponsesRequest{
		Model: "public-alias", Input: []byte(input),
		Reasoning: &dto.Reasoning{Effort: "high"},
		Include:   []byte(`["reasoning.encrypted_content"]`), Store: []byte(`false`),
		ServiceTier: "auto",
	}
	info := relaycommon.GenRelayInfoResponses(c, request)
	for _, tc := range []struct {
		name, mapping, model string
		repair               bool
	}{
		{"off and unmapped", "", "public-alias", false},
		{"on and mapped", `{"public-alias":"gpt-test"}`, "gpt-test", true},
		{"retry to off channel", `{"public-alias":"gpt-test"}`, "gpt-test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ReasoningContentRepair: tc.repair, ToolOutputRepair: tc.repair})
			common.SetContextKey(c, constant.ContextKeyChannelModelMapping, tc.mapping)
			common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"max_output_tokens": 256})
			_, body, closer, apiErr := PrepareResponsesRequest(c, info, request)
			require.Nil(t, apiErr)
			defer closer.Close()
			wire, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.JSONEq(t, input, gjson.GetBytes(wire, "input").Raw)
			for _, path := range []string{"0.encrypted_content", "1.encrypted_content", "2.encrypted_content"} {
				assert.Equal(t, gjson.Get(input, path).String(), gjson.GetBytes(wire, "input."+path).String())
			}
			assert.Equal(t, tc.model, gjson.GetBytes(wire, "model").String())
			assert.Equal(t, tc.repair, info.ReasoningContentRepairActive)
			assert.Equal(t, int64(256), gjson.GetBytes(wire, "max_output_tokens").Int())
			assert.False(t, gjson.GetBytes(wire, "service_tier").Exists())
			assert.Equal(t, "false", gjson.GetBytes(wire, "store").Raw)
			assert.Equal(t, "reasoning.encrypted_content", gjson.GetBytes(wire, "include.0").String())
			assert.Equal(t, input, string(request.Input), "later attempts must use the original history")
		})
	}
}
