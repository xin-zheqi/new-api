package ollama

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaReasoningEffortMappingAfterNativeConversion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  int
		input string
		want  string
	}{
		{"chat effort", relayconstant.RelayModeChatCompletions, `{"model":"gpt-oss","reasoning_effort":"high","messages":[{"role":"user","content":"hello"}]}`, `"max"`},
		{"chat boolean flag", relayconstant.RelayModeChatCompletions, `{"model":"gpt-oss","think":true,"messages":[{"role":"user","content":"hello"}]}`, `true`},
		{"legacy completions native effort", relayconstant.RelayModeCompletions, `{"model":"gpt-oss","think":"high","prompt":"hello"}`, `"max"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAI, RelayMode: tc.mode,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:    constant.ChannelTypeOllama,
					ChannelSetting: dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"high": "max", "true": "max"}},
				},
			}
			request := &dto.GeneralOpenAIRequest{}
			require.NoError(t, common.Unmarshal([]byte(tc.input), request))
			adaptor := &Adaptor{}
			converted, err := adaptor.ConvertOpenAIRequest(c, info, request)
			require.NoError(t, err)
			relaycommon.AppendRequestConversionFromRequest(info, converted)
			encoded, err := common.Marshal(converted)
			require.NoError(t, err)
			body, closer, err := relaycommon.NewChannelJSONBody(encoded, info)
			require.NoError(t, err)
			defer closer.Close()
			upstream, err := io.ReadAll(body)
			require.NoError(t, err)
			var sent struct {
				Think any `json:"think"`
			}
			require.NoError(t, common.Unmarshal(upstream, &sent))
			actual, err := common.Marshal(sent.Think)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(actual))
		})
	}
}
