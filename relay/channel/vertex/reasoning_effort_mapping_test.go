package vertex

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVertexClaudeReasoningEffortMappingAfterConversion(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		t.Run(string(format), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{
				RelayFormat: format,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       constant.ChannelTypeVertexAi,
					UpstreamModelName: "claude-opus-4-6",
					ChannelSetting: dto.ChannelSettings{
						ReasoningEffortMapping: map[string]string{"high": "max"},
					},
				},
			}
			info.InitRequestConversionChain()
			adaptor := &Adaptor{RequestMode: RequestModeClaude}
			var converted any
			var err error
			if format == types.RelayFormatClaude {
				request := &dto.ClaudeRequest{}
				require.NoError(t, common.Unmarshal([]byte(`{"model":"claude-opus-4-6","max_tokens":1024,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"messages":[{"role":"user","content":"hello"}]}`), request))
				converted, err = adaptor.ConvertClaudeRequest(c, info, request)
			} else {
				request := &dto.GeneralOpenAIRequest{}
				require.NoError(t, common.Unmarshal([]byte(`{"model":"claude-opus-4-6","max_tokens":1024,"reasoning_effort":"high","messages":[{"role":"user","content":"hello"}]}`), request))
				converted, err = adaptor.ConvertOpenAIRequest(c, info, request)
			}
			require.NoError(t, err)
			require.IsType(t, &VertexAIClaudeRequest{}, converted)
			relaycommon.AppendRequestConversionFromRequest(info, converted)
			require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
			encoded, err := common.Marshal(converted)
			require.NoError(t, err)
			body, closer, err := relaycommon.NewChannelJSONBody(encoded, info)
			require.NoError(t, err)
			defer closer.Close()
			upstream, err := io.ReadAll(body)
			require.NoError(t, err)
			var sent VertexAIClaudeRequest
			require.NoError(t, common.Unmarshal(upstream, &sent))
			assert.JSONEq(t, `{"effort":"max"}`, string(sent.OutputConfig))
			assert.JSONEq(t, `{"effort":"high"}`, string(converted.(*VertexAIClaudeRequest).OutputConfig))
			assert.Equal(t, anthropicVersion, sent.AnthropicVersion)
			assert.Len(t, sent.Messages, 1)
			assert.Equal(t, "max", info.ReasoningEffort)
		})
	}
}
