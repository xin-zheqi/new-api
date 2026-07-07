package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newImageGenerationPolicyTestContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", io.NopCloser(stringsReader(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func stringsReader(s string) io.Reader {
	return strings.NewReader(s)
}

func newPolicyChannel(settings dto.ChannelOtherSettings) *model.Channel {
	ch := &model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeCodex,
		Name:   "codex",
		Models: "gpt-5",
		Group:  "default",
	}
	ch.SetOtherSettings(settings)
	return ch
}

func newPolicyRelayInfo(request dto.Request) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponses,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		OriginModelName: "gpt-5",
		Request:         request,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeCodex,
		},
	}
	if responsesReq, ok := request.(*dto.OpenAIResponsesRequest); ok {
		info.ResponsesUsageInfo = &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{},
		}
		for _, tool := range responsesReq.GetToolsMap() {
			toolType := common.Interface2String(tool["type"])
			info.ResponsesUsageInfo.BuiltInTools[toolType] = &relaycommon.BuildInToolInfo{
				ToolName: toolType,
			}
		}
	}
	return info
}

func TestApplyChannelImageGenerationPolicy_DisabledRejectsImageTool(t *testing.T) {
	body := `{"model":"gpt-5","input":"draw","tools":[{"type":"image_generation"}]}`
	c := newImageGenerationPolicyTestContext(t, body)
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	ch := newPolicyChannel(dto.ChannelOtherSettings{DisableImageGeneration: true})

	err := applyChannelImageGenerationPolicy(c, info, ch)

	require.NotNil(t, err)
	require.Equal(t, http.StatusForbidden, err.StatusCode)
	require.Equal(t, types.ErrorCode("permission_error"), err.GetErrorCode())
	require.Contains(t, err.ToOpenAIError().Message, imageGenerationDisabledMessage)
}

func TestApplyChannelImageGenerationPolicy_UsesContextChannelSettings(t *testing.T) {
	body := `{"model":"gpt-5","input":"draw","tools":[{"type":"image_generation"}]}`
	c := newImageGenerationPolicyTestContext(t, body)
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{DisableImageGeneration: true})
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	ch := newPolicyChannel(dto.ChannelOtherSettings{})

	err := applyChannelImageGenerationPolicy(c, info, ch)

	require.NotNil(t, err)
	require.Equal(t, http.StatusForbidden, err.StatusCode)
	require.Contains(t, err.ToOpenAIError().Message, imageGenerationDisabledMessage)
}

func TestApplyChannelImageGenerationPolicy_DisabledRejectsImageModel(t *testing.T) {
	body := `{"model":"gpt-image-1","input":"draw"}`
	c := newImageGenerationPolicyTestContext(t, body)
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	info.OriginModelName = "gpt-image-1"
	ch := newPolicyChannel(dto.ChannelOtherSettings{DisableImageGeneration: true})

	err := applyChannelImageGenerationPolicy(c, info, ch)

	require.NotNil(t, err)
	require.Equal(t, http.StatusForbidden, err.StatusCode)
	require.Equal(t, types.ErrorCode("permission_error"), err.GetErrorCode())
}

func TestApplyChannelImageGenerationPolicy_StripsImageGenerationTool(t *testing.T) {
	body := `{"model":"gpt-5","input":"draw","tools":[{"type":"function","name":"shell"},{"type":"image_generation"}],"tool_choice":{"type":"image_generation"}}`
	c := newImageGenerationPolicyTestContext(t, body)
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	ch := newPolicyChannel(dto.ChannelOtherSettings{StripImageGenerationTool: true})

	err := applyChannelImageGenerationPolicy(c, info, ch)

	require.Nil(t, err)
	storage, storageErr := common.GetBodyStorage(c)
	require.NoError(t, storageErr)
	updated, readErr := storage.Bytes()
	require.NoError(t, readErr)
	require.False(t, gjson.GetBytes(updated, `tools.#(type=="image_generation")`).Exists())
	require.True(t, gjson.GetBytes(updated, `tools.#(type=="function")`).Exists())
	require.False(t, gjson.GetBytes(updated, "tool_choice").Exists())
	require.False(t, gjson.GetBytes(req.Tools, `#(type=="image_generation")`).Exists())
	_, imageToolTracked := info.ResponsesUsageInfo.BuiltInTools["image_generation"]
	require.False(t, imageToolTracked)
	reqJSON, marshalErr := common.Marshal(req)
	require.NoError(t, marshalErr)
	require.False(t, gjson.GetBytes(reqJSON, "tool_choice").Exists())
}

func TestApplyChannelImageGenerationPolicy_StripsImageGenerationToolWithoutCodexContext(t *testing.T) {
	body := `{"model":"gpt-5","input":"draw","tools":[{"type":"function","name":"shell"},{"type":"image_generation"}],"tool_choice":{"type":"image_generation"}}`
	c := newImageGenerationPolicyTestContext(t, body)
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	info.ChannelMeta = nil
	ch := newPolicyChannel(dto.ChannelOtherSettings{StripImageGenerationTool: true})

	err := applyChannelImageGenerationPolicy(c, info, ch)

	require.Nil(t, err)
	storage, storageErr := common.GetBodyStorage(c)
	require.NoError(t, storageErr)
	updated, readErr := storage.Bytes()
	require.NoError(t, readErr)
	require.False(t, gjson.GetBytes(updated, `tools.#(type=="image_generation")`).Exists())
	require.True(t, gjson.GetBytes(updated, `tools.#(type=="function")`).Exists())
}

func TestApplyChannelImageGenerationPolicy_StripsImageGenerationToolWithLegacyCodexSetting(t *testing.T) {
	body := `{"model":"gpt-5","input":"draw","tools":[{"type":"image_generation"}]}`
	c := newImageGenerationPolicyTestContext(t, body)
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	ch := newPolicyChannel(dto.ChannelOtherSettings{StripCodexImageTool: true})

	err := applyChannelImageGenerationPolicy(c, info, ch)

	require.Nil(t, err)
	storage, storageErr := common.GetBodyStorage(c)
	require.NoError(t, storageErr)
	updated, readErr := storage.Bytes()
	require.NoError(t, readErr)
	require.False(t, gjson.GetBytes(updated, `tools.#(type=="image_generation")`).Exists())
}

func TestApplyChannelImageGenerationPolicy_DisableTakesPriorityOverStrip(t *testing.T) {
	body := `{"model":"gpt-5","input":"draw","tools":[{"type":"image_generation"}]}`
	c := newImageGenerationPolicyTestContext(t, body)
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	ch := newPolicyChannel(dto.ChannelOtherSettings{
		DisableImageGeneration:   true,
		StripImageGenerationTool: true,
	})

	err := applyChannelImageGenerationPolicy(c, info, ch)

	require.NotNil(t, err)
	storage, storageErr := common.GetBodyStorage(c)
	require.NoError(t, storageErr)
	updated, readErr := storage.Bytes()
	require.NoError(t, readErr)
	require.True(t, gjson.GetBytes(updated, `tools.#(type=="image_generation")`).Exists())
}

func TestApplyChannelImageGenerationPolicy_RetryRestoresOriginalBeforeDisable(t *testing.T) {
	body := `{"model":"gpt-5","input":"draw","tools":[{"type":"image_generation"}]}`
	c := newImageGenerationPolicyTestContext(t, body)
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	stripChannel := newPolicyChannel(dto.ChannelOtherSettings{StripImageGenerationTool: true})
	disableChannel := newPolicyChannel(dto.ChannelOtherSettings{DisableImageGeneration: true})

	stripErr := applyChannelImageGenerationPolicy(c, info, stripChannel)
	require.Nil(t, stripErr)
	storage, storageErr := common.GetBodyStorage(c)
	require.NoError(t, storageErr)
	stripped, readErr := storage.Bytes()
	require.NoError(t, readErr)
	require.False(t, gjson.GetBytes(stripped, `tools.#(type=="image_generation")`).Exists())

	disableErr := applyChannelImageGenerationPolicy(c, info, disableChannel)

	require.NotNil(t, disableErr)
	require.Equal(t, http.StatusForbidden, disableErr.StatusCode)
	storage, storageErr = common.GetBodyStorage(c)
	require.NoError(t, storageErr)
	restored, readErr := storage.Bytes()
	require.NoError(t, readErr)
	require.True(t, gjson.GetBytes(restored, `tools.#(type=="image_generation")`).Exists())
}

func TestApplyChannelImageGenerationPolicy_RetryRestoresOriginalForChannelWithoutStrip(t *testing.T) {
	body := `{"model":"gpt-5","input":"draw","tools":[{"type":"image_generation"}]}`
	c := newImageGenerationPolicyTestContext(t, body)
	req := &dto.OpenAIResponsesRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), req))
	info := newPolicyRelayInfo(req)
	stripChannel := newPolicyChannel(dto.ChannelOtherSettings{StripImageGenerationTool: true})
	defaultChannel := newPolicyChannel(dto.ChannelOtherSettings{})

	stripErr := applyChannelImageGenerationPolicy(c, info, stripChannel)
	require.Nil(t, stripErr)
	nextErr := applyChannelImageGenerationPolicy(c, info, defaultChannel)

	require.Nil(t, nextErr)
	storage, storageErr := common.GetBodyStorage(c)
	require.NoError(t, storageErr)
	restored, readErr := storage.Bytes()
	require.NoError(t, readErr)
	require.True(t, gjson.GetBytes(restored, `tools.#(type=="image_generation")`).Exists())
	require.True(t, gjson.GetBytes(req.Tools, `#(type=="image_generation")`).Exists())
	_, imageToolTracked := info.ResponsesUsageInfo.BuiltInTools["image_generation"]
	require.True(t, imageToolTracked)
}
