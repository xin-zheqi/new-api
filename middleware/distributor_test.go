package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelFromJSONBodyRejectsOverlongGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"model":"gpt-test","group":"` + strings.Repeat("a", model.MaxTokenGroupConfigLength+1) + `"}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	req, err := getModelFromJSONBody(ctx)

	require.Nil(t, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field group is too long")
}

func TestGetModelRequestRejectsOverlongModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"model":"` + strings.Repeat("a", model.MaxRelayModelNameLength+1) + `"}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	req, shouldSelect, err := getModelRequest(ctx)

	require.Nil(t, req)
	require.False(t, shouldSelect)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field model is too long")
}

func TestChannelMatchesExpectedTaskPluginUsesGenericChannelSetting(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeTaskPlugin}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "generic-alpha"})

	assert.True(t, channelMatchesExpectedTaskPlugin(nil, channel, "generic-alpha", channel.GetSetting))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, "generic-beta", channel.GetSetting))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, "", channel.GetSetting))
}

func TestChannelMatchesExpectedTaskPluginUsesPinnedLegacyIndex(t *testing.T) {
	registry := jsplugin.NewRegistry()
	alpha, err := registry.Register(distributorTaskPluginSource("legacy-alpha", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)
	pinnedGeneration := registry.Generation()

	require.NoError(t, registry.Unregister("legacy-alpha"))
	_, err = registry.Register(distributorTaskPluginSource("legacy-beta", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: pinnedGeneration,
		Plugin:     alpha,
	})
	channel := &model.Channel{Type: constant.ChannelTypeKling}

	assert.True(t, channelMatchesExpectedTaskPlugin(c, channel, "legacy-alpha", channel.GetSetting))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, channel, "legacy-beta", channel.GetSetting))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "legacy-alpha", nil))
}

func TestChannelMatchesExpectedTaskPluginRejectsUnindexedLegacyChannel(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(distributorTaskPluginSource("legacy-alpha", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "legacy-alpha", nil))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: 0}, "legacy-alpha", nil))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "", nil))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, &model.Channel{Type: constant.ChannelTypeKling}, "legacy-alpha", nil))

	c.Set("expected_task_plugin_key", "legacy-alpha")
	rejected := &model.Channel{Type: constant.ChannelTypeJimeng, Setting: common.GetPointer("{invalid")}
	setupErr := SetupContextForSelectedChannel(c, rejected, "task-model")
	require.NotNil(t, setupErr)
	assert.Contains(t, setupErr.Error(), "does not match")
	assert.Equal(t, "{invalid", *rejected.Setting, "identity rejection must not parse or repair unrelated channel settings")
}

func TestSharedEndpointRebindsToSelectedLegacyProvider(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(distributorEndpointPluginSource("gemini-shared", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(distributorEndpointPluginSource("vertex-shared", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: candidates[0].Plugin})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
		Protocol:   candidates[0].Protocol,
		Operation:  candidates[0].Operation,
		Model:      "task-model",
		Candidates: candidates,
	})
	c.Set("expected_task_plugin_key", candidates[0].Plugin.Meta.Key)

	geminiChannel := &model.Channel{Id: 1, Type: constant.ChannelTypeGemini}
	vertexChannel := &model.Channel{Id: 2, Type: constant.ChannelTypeVertexAi}
	assert.True(t, channelMatchesExpectedTaskPlugin(c, geminiChannel, candidates[0].Plugin.Meta.Key, geminiChannel.GetSetting))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, vertexChannel, candidates[0].Plugin.Meta.Key, vertexChannel.GetSetting))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeKling}, candidates[0].Plugin.Meta.Key, nil))

	require.Nil(t, SetupContextForSelectedChannel(c, vertexChannel, "task-model"))
	pinnedValue, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint)
	require.True(t, exists)
	pinned, ok := pinnedValue.(jsplugin.PinnedEndpoint)
	require.True(t, ok)
	assert.Equal(t, "vertex-shared", pinned.Plugin.Meta.Key)
	assert.Equal(t, "vertex-shared", c.GetString("expected_task_plugin_key"))
	assert.Equal(t, "vertex-shared", c.GetString("task_plugin_key"))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, geminiChannel, "vertex-shared", geminiChannel.GetSetting), "a retry may select another declared provider")
}

func TestSetupChannelSettingsRefreshOnRetryAndRemainRequestLocal(t *testing.T) {
	first := &model.Channel{Id: 42, Type: constant.ChannelTypeOpenAI, Key: "fixture"}
	first.SetSetting(dto.ChannelSettings{Proxy: "http://first.invalid", ReasoningEffortMapping: map[string]string{"high": "max"}})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Nil(t, SetupContextForSelectedChannel(c, first, "requested-model"))
	setting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	require.True(t, ok)
	assert.Equal(t, "http://first.invalid", setting.Proxy)
	setting.ReasoningEffortMapping["high"] = "low"

	// Retrying the same channel reloads its stored configuration rather than
	// inheriting an adaptor's in-request changes to a map in the previous DTO.
	require.Nil(t, SetupContextForSelectedChannel(c, first, "requested-model"))
	setting, ok = common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	require.True(t, ok)
	assert.Equal(t, "max", setting.ReasoningEffortMapping["high"])

	updated := &model.Channel{Id: first.Id, Type: first.Type, Key: "fixture"}
	updated.SetSetting(dto.ChannelSettings{Proxy: "http://updated.invalid", RejectEmptyResponse: true})
	require.Nil(t, SetupContextForSelectedChannel(c, updated, "requested-model"))
	setting, ok = common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	require.True(t, ok)
	assert.Equal(t, "http://updated.invalid", setting.Proxy)
	assert.True(t, setting.RejectEmptyResponse)
	assert.Empty(t, setting.ReasoningEffortMapping)

	nextRequest, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Nil(t, SetupContextForSelectedChannel(nextRequest, first, "requested-model"))
	nextSetting, ok := common.GetContextKeyType[dto.ChannelSettings](nextRequest, constant.ContextKeyChannelSetting)
	require.True(t, ok)
	assert.Equal(t, "http://first.invalid", nextSetting.Proxy)
	assert.Equal(t, "max", nextSetting.ReasoningEffortMapping["high"])
	assert.False(t, nextSetting.RejectEmptyResponse)
}

func TestSetupGenericTaskPluginSettingsFollowSelectedEndpoint(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(distributorEndpointPluginSource("first-shared", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(distributorEndpointPluginSource("second-shared", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: candidates[0].Plugin})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(), Plugin: candidates[0].Plugin, Candidates: candidates,
	})
	c.Set("expected_task_plugin_key", candidates[0].Plugin.Meta.Key)
	channel := &model.Channel{Id: 43, Type: constant.ChannelTypeTaskPlugin, Key: "fixture"}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: candidates[1].Plugin.Meta.Key, ForceFormat: true})

	require.Nil(t, SetupContextForSelectedChannel(c, channel, "task-model"))
	setting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	require.True(t, ok)
	assert.Equal(t, candidates[1].Plugin.Meta.Key, setting.TaskPluginKey)
	assert.True(t, setting.ForceFormat)
	assert.Equal(t, setting.TaskPluginKey, c.GetString("expected_task_plugin_key"))
	assert.Equal(t, setting.TaskPluginKey, c.GetString("task_plugin_key"))
}

func TestSetupTaskPluginSettingsKeepMalformedSettingRepairBehavior(t *testing.T) {
	database := setupTokenAuthTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	channel := &model.Channel{
		Type: constant.ChannelTypeTaskPlugin, Key: "fixture",
		Setting: common.GetPointer(`{"task_plugin_key":"generic-alpha","force_format":"invalid"}`),
	}
	require.NoError(t, database.Create(channel).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("expected_task_plugin_key", "generic-alpha")

	require.Nil(t, SetupContextForSelectedChannel(c, channel, "task-model"))
	setting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	require.True(t, ok)
	assert.Empty(t, setting.TaskPluginKey)
	assert.Empty(t, c.GetString("task_plugin_key"))
	assert.Nil(t, channel.Setting)
	var stored model.Channel
	require.NoError(t, database.First(&stored, channel.Id).Error)
	assert.Nil(t, stored.Setting)
}

func distributorTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType)
}

func distributorEndpointPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["stream", "sync", "background"]}],
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) { return {kind: "submit", model: "task-model", requestBody: ctx.body.value}; },
  renderEvents: function() { return {events: [], state: null, done: false}; },
  renderFinal: function() { return {output: []}; },
}};
`, key, key, channelType)
}

// /v1/tools/search and /v1/tools/fetch always route and bill by the DeepSeek
// routing names. A model named in the body is ignored: a chat model there
// would price the call at zero tokens.
func TestGetModelRequestAssignsDeepSeekToolRoutingNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ path, body, want string }{
		{"/v1/tools/search", `{"query":"deepseek","count":3}`, constant.DeepSeekSearchModel},
		{"/v1/tools/fetch", `{"url":"https://example.com/"}`, constant.DeepSeekFetchModel},
		{"/v1/tools/search", `{"query":"deepseek","model":"deepseek-flash"}`, constant.DeepSeekSearchModel},
		{"/v1/tools/fetch", `{"url":"https://example.com/","model":"deepseek-v4-pro"}`, constant.DeepSeekFetchModel},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		require.NoError(t, err, tc.body)
		assert.True(t, shouldSelectChannel, tc.body)
		assert.Equal(t, tc.want, modelRequest.Model, tc.body)
	}
}

func TestNoAvailableChannelMessageNamesClaimingTaskPlugin(t *testing.T) {
	require.NoError(t, i18n.Init())
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(distributorTaskPluginSource("claimer", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	pinned, _ := gin.CreateTestContext(nil)
	pinned.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	pinned.Request.Header.Set("Accept-Language", "en")
	pinned.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
	message := noAvailableChannelMessage(pinned, "default", "kling-v1")
	assert.Contains(t, message, `"claimer"`)
	assert.Contains(t, message, "disable or override")
	assert.Contains(t, message, "kling-v1")

	plain, _ := gin.CreateTestContext(nil)
	plain.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plain.Request.Header.Set("Accept-Language", "en")
	generic := noAvailableChannelMessage(plain, "default", "gpt-4o")
	assert.NotContains(t, generic, "task plugin")
	assert.Contains(t, generic, "gpt-4o")
}
