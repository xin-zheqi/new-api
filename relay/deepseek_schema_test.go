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

func TestPrepareResponsesDeepSeekSchemaAfterOverridesAndRetry(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "deepseek-flash")
	request := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"deepseek-flash","reasoning":{"effort":"none"},"input":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"original thought"}],"summary":[]}],"tools":[{"type":"function","name":"run","parameters":{"type":"object","properties":{"args":{"type":["array","null"],"items":{"type":"string"}}}}}]}`, request))
	original, err := common.Marshal(request)
	require.NoError(t, err)
	info := relaycommon.GenRelayInfoResponses(c, request)
	for _, channelType := range []int{constant.ChannelTypeDeepSeek, constant.ChannelTypeOpenAI, constant.ChannelTypeDeepSeek} {
		common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
		common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ReasoningContentRepair: true, ToolOutputRepair: true})
		var override map[string]any
		require.NoError(t, common.UnmarshalJsonStr(`{"text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object","properties":{"rows":{"type":["array","null"],"items":{"type":"integer"}}}}}}}`, &override))
		common.SetContextKey(c, constant.ContextKeyChannelParamOverride, override)
		_, body, closer, apiErr := PrepareResponsesRequest(c, info, request)
		require.Nil(t, apiErr)
		wire, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, closer.Close())
		for _, path := range []string{"tools.0.parameters.properties.args", "text.format.schema.properties.rows"} {
			if channelType == constant.ChannelTypeDeepSeek {
				assert.Equal(t, "array", gjson.GetBytes(wire, path+".anyOf.0.type").String())
			} else {
				assert.Equal(t, `["array","null"]`, gjson.GetBytes(wire, path+".type").Raw)
			}
		}
		assert.JSONEq(t, gjson.GetBytes(original, "input").Raw, gjson.GetBytes(wire, "input").Raw)
		after, err := common.Marshal(request)
		require.NoError(t, err)
		assert.Equal(t, original, after)
	}
}
