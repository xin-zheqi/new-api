package controller

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	imageGenerationDisabledMessage       = "当前渠道不允许使用生图工具，请在工具调用中禁用生图选项。"
	imageGenerationPolicyOriginalBodyKey = "image_generation_policy_original_body"
)

func applyChannelImageGenerationPolicy(c *gin.Context, info *relaycommon.RelayInfo, channel *model.Channel) *types.NewAPIError {
	if c == nil || info == nil || channel == nil {
		return nil
	}
	if err := restoreOriginalRequestBodyForImagePolicy(c, info); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	settings := getChannelImageGenerationPolicySettings(c, channel)
	stripImageGenerationTool := settings.StripImageGenerationTool || settings.StripCodexImageTool
	if !settings.DisableImageGeneration && !stripImageGenerationTool {
		return nil
	}

	if settings.DisableImageGeneration {
		intent, err := requestHasImageGenerationIntent(c, info)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if intent {
			return imageGenerationDisabledError()
		}
	}

	if stripImageGenerationTool {
		if err := stripImageGenerationToolFromCurrentRequest(c, info); err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeBadRequestBody, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	return nil
}

func getChannelImageGenerationPolicySettings(c *gin.Context, channel *model.Channel) dto.ChannelOtherSettings {
	if c != nil {
		if settings, ok := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting); ok {
			return settings
		}
	}
	if channel == nil {
		return dto.ChannelOtherSettings{}
	}
	return channel.GetOtherSettings()
}

func imageGenerationDisabledError() *types.NewAPIError {
	return types.WithOpenAIError(types.OpenAIError{
		Message: imageGenerationDisabledMessage,
		Type:    "permission_error",
		Code:    "permission_error",
	}, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}

func requestHasImageGenerationIntent(c *gin.Context, info *relaycommon.RelayInfo) (bool, error) {
	if info == nil {
		return false, nil
	}
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		return true, nil
	}
	if isOpenAIImageGenerationModel(info.OriginModelName) {
		return true, nil
	}

	body, ok, err := currentJSONBodyBytes(c)
	if err != nil || !ok {
		return false, err
	}
	return jsonBodyHasImageGenerationIntent(body), nil
}

func currentJSONBodyBytes(c *gin.Context) ([]byte, bool, error) {
	if c == nil || c.Request == nil {
		return nil, false, nil
	}
	contentType := strings.ToLower(c.Request.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "json") {
		return nil, false, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, false, err
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, false, err
	}
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return nil, false, err
	}
	c.Request.Body = io.NopCloser(storage)
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil, false, nil
	}
	return body, true, nil
}

func jsonBodyHasImageGenerationIntent(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	if isOpenAIImageGenerationModel(gjson.GetBytes(body, "model").String()) {
		return true
	}
	if jsonToolsContainImageGeneration(gjson.GetBytes(body, "tools")) {
		return true
	}
	return jsonToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice"))
}

func jsonToolsContainImageGeneration(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	found := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if strings.TrimSpace(item.Get("type").String()) == "image_generation" {
			found = true
			return false
		}
		return true
	})
	return found
}

func jsonToolChoiceSelectsImageGeneration(choice gjson.Result) bool {
	if !choice.Exists() {
		return false
	}
	if choice.Type == gjson.String {
		return strings.TrimSpace(choice.String()) == "image_generation"
	}
	if !choice.IsObject() {
		return false
	}
	if strings.TrimSpace(choice.Get("type").String()) == "image_generation" {
		return true
	}
	if strings.TrimSpace(choice.Get("tool.type").String()) == "image_generation" {
		return true
	}
	return strings.TrimSpace(choice.Get("function.name").String()) == "image_generation"
}

func isOpenAIImageGenerationModel(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return strings.HasPrefix(modelName, "gpt-image-") || strings.HasPrefix(modelName, "chatgpt-image-")
}

func stripImageGenerationToolFromCurrentRequest(c *gin.Context, info *relaycommon.RelayInfo) error {
	body, ok, err := currentJSONBodyBytes(c)
	if err != nil || !ok {
		return err
	}
	cacheOriginalRequestBodyForImagePolicy(c, body)
	updated, changed, err := stripImageGenerationToolFromJSON(body)
	if err != nil || !changed {
		return err
	}
	return replaceCurrentRequestBody(c, info, updated)
}

func syncParsedRequestAfterBodyMutation(info *relaycommon.RelayInfo, updated []byte) error {
	if info == nil || info.Request == nil {
		return nil
	}
	switch req := info.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		var next dto.OpenAIResponsesRequest
		if err := common.Unmarshal(updated, &next); err != nil {
			return err
		}
		*req = next
		rebuildResponsesUsageInfo(info, req)
		return nil
	case *dto.GeneralOpenAIRequest:
		var next dto.GeneralOpenAIRequest
		if err := common.Unmarshal(updated, &next); err != nil {
			return err
		}
		*req = next
		return nil
	default:
		return nil
	}
}

func cacheOriginalRequestBodyForImagePolicy(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	if _, exists := c.Get(imageGenerationPolicyOriginalBodyKey); exists {
		return
	}
	c.Set(imageGenerationPolicyOriginalBodyKey, append([]byte(nil), body...))
}

func restoreOriginalRequestBodyForImagePolicy(c *gin.Context, info *relaycommon.RelayInfo) error {
	if c == nil {
		return nil
	}
	value, exists := c.Get(imageGenerationPolicyOriginalBodyKey)
	if !exists {
		return nil
	}
	body, ok := value.([]byte)
	if !ok || len(body) == 0 {
		return nil
	}
	return replaceCurrentRequestBody(c, info, body)
}

func replaceCurrentRequestBody(c *gin.Context, info *relaycommon.RelayInfo, body []byte) error {
	if c == nil || c.Request == nil {
		return nil
	}
	storage, err := common.CreateBodyStorage(body)
	if err != nil {
		return err
	}
	if oldStorage, exists := c.Get(common.KeyBodyStorage); exists {
		if oldBodyStorage, ok := oldStorage.(common.BodyStorage); ok {
			_ = oldBodyStorage.Close()
		}
	}
	c.Set(common.KeyBodyStorage, storage)
	c.Request.Body = io.NopCloser(storage)
	c.Request.ContentLength = int64(len(body))
	c.Request.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	return syncParsedRequestAfterBodyMutation(info, body)
}

func rebuildResponsesUsageInfo(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	if info == nil || request == nil || info.ResponsesUsageInfo == nil {
		return
	}
	info.ResponsesUsageInfo.BuiltInTools = make(map[string]*relaycommon.BuildInToolInfo)
	if len(request.Tools) == 0 {
		return
	}
	for _, tool := range request.GetToolsMap() {
		toolType := common.Interface2String(tool["type"])
		info.ResponsesUsageInfo.BuiltInTools[toolType] = &relaycommon.BuildInToolInfo{
			ToolName:  toolType,
			CallCount: 0,
		}
		if toolType == dto.BuildInToolWebSearchPreview {
			searchContextSize := common.Interface2String(tool["search_context_size"])
			if searchContextSize == "" {
				searchContextSize = "medium"
			}
			info.ResponsesUsageInfo.BuiltInTools[toolType].SearchContextSize = searchContextSize
		}
	}
}

func stripImageGenerationToolFromJSON(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	if !jsonToolsContainImageGeneration(gjson.GetBytes(body, "tools")) &&
		!jsonToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice")) {
		return body, false, nil
	}

	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return body, false, err
	}

	changed := false
	if rawTools, ok := payload["tools"]; ok {
		if tools, ok := rawTools.([]any); ok {
			filtered := make([]any, 0, len(tools))
			for _, rawTool := range tools {
				if tool, ok := rawTool.(map[string]any); ok && strings.TrimSpace(firstString(tool["type"])) == "image_generation" {
					changed = true
					continue
				}
				filtered = append(filtered, rawTool)
			}
			if changed {
				if len(filtered) == 0 {
					delete(payload, "tools")
				} else {
					payload["tools"] = filtered
				}
			}
		}
	}

	if anyToolChoiceSelectsImageGeneration(payload["tool_choice"]) {
		delete(payload, "tool_choice")
		changed = true
	}
	if !changed {
		return body, false, nil
	}
	updated, err := common.Marshal(payload)
	if err != nil {
		return body, false, err
	}
	return updated, true, nil
}

func anyToolChoiceSelectsImageGeneration(choice any) bool {
	switch v := choice.(type) {
	case string:
		return strings.TrimSpace(v) == "image_generation"
	case map[string]any:
		if strings.TrimSpace(firstString(v["type"])) == "image_generation" {
			return true
		}
		if tool, ok := v["tool"].(map[string]any); ok && strings.TrimSpace(firstString(tool["type"])) == "image_generation" {
			return true
		}
		if fn, ok := v["function"].(map[string]any); ok && strings.TrimSpace(firstString(fn["name"])) == "image_generation" {
			return true
		}
	}
	return false
}

func firstString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
