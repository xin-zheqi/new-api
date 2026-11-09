package aws

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsReasoningEffortMappingSurvivesSDKEnvelope(t *testing.T) {
	global := model_setting.GetGlobalSettings()
	previousPassthrough := global.PassThroughRequestEnabled
	global.PassThroughRequestEnabled = false
	t.Cleanup(func() { global.PassThroughRequestEnabled = previousPassthrough })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{
			ReasoningEffortMapping: map[string]string{"high": "max"},
		}},
	}
	input := []byte(`{"model":"claude-opus-4-6","output_config":{"effort":"high","format":{"type":"json_schema","schema":{"type":"object"}}},"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"hello"}],"max_tokens":1024}`)
	body, closer, err := relaycommon.NewChannelJSONBody(input, info)
	require.NoError(t, err)
	defer closer.Close()
	request, err := formatRequest(body, http.Header{})
	require.NoError(t, err)
	upstream, err := buildAwsRequestBody(c, info, request)
	require.NoError(t, err)
	var sent AwsClaudeRequest
	require.NoError(t, common.Unmarshal(upstream, &sent))
	assert.JSONEq(t, `{"effort":"max","format":{"type":"json_schema","schema":{"type":"object"}}}`, string(sent.OutputConfig))
	assert.Equal(t, "bedrock-2023-05-31", sent.AnthropicVersion)
	assert.Equal(t, "adaptive", sent.Thinking.Type)
	assert.Len(t, sent.Messages, 1)
	assert.Contains(t, string(input), `"effort":"high"`)
	// The replay reader must retain the mapped request used by either SDK mode.
	replay, err := body.NewReader()
	require.NoError(t, err)
	defer replay.Close()
	mapped, err := io.ReadAll(replay)
	require.NoError(t, err)
	assert.Contains(t, string(mapped), `"effort":"max"`)
}
