package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildAlphaSearchRequestBodyPreservesUnknownFields(t *testing.T) {
	raw := []byte(`{
		"id":"req_1",
		"model":"gpt-5.1",
		"input":[{"role":"user","content":"hi"}],
		"commands":{"search_query":[{"q":"weather","recency":1}]},
		"settings":{"locale":"en"},
		"future_field":{"nested":true}
	}`)

	out, err := buildAlphaSearchRequestBody(raw, "gpt-5.1", "gpt-5.1-mapped")
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, common.Unmarshal(out, &body))
	assert.Equal(t, "gpt-5.1-mapped", body["model"])
	assert.Equal(t, "req_1", body["id"])
	require.Contains(t, body, "commands")
	require.Contains(t, body, "settings")
	require.Contains(t, body, "future_field")
	require.Contains(t, body, "input")

	commands, ok := body["commands"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, commands, "search_query")

	future, ok := body["future_field"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, future["nested"])
}

func TestBuildAlphaSearchRequestBodyNoMappingKeepsRawBytes(t *testing.T) {
	raw := []byte(`{"model":"gpt-5.1","commands":{"search_query":[{"q":"x"}]},"future_field":1}`)
	out, err := buildAlphaSearchRequestBody(raw, "gpt-5.1", "gpt-5.1")
	require.NoError(t, err)
	assert.Equal(t, raw, out)
}

// Exercise validation, channel selection metadata, the real HTTP adaptor and
// settlement together: search is not a Chat/Responses completion and has no
// token usage. Unknown request/response fields must survive the relay.
func TestAlphaSearchHelperOpenAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	batch, logConsume := common.BatchUpdateEnabled, common.LogConsumeEnabled
	common.BatchUpdateEnabled, common.LogConsumeEnabled = true, false
	t.Cleanup(func() { common.BatchUpdateEnabled, common.LogConsumeEnabled = batch, logConsume })

	const path = "/v1/alpha/search?session=a%2Fb&feature=1"
	const raw = `{ "model":"o1", "commands":{"search_query":[{"q":"weather"}]}, "stream":false, "future":{"id":9007199254740993,"model":"keep-nested"} }`
	const response = `{ "results":[{"title":"Weather","url":"https://example.com/"}], "future_id":9007199254740993 }`
	for _, tc := range []struct {
		name        string
		channelType int
		mapped      bool
		override    bool
		status      int
	}{
		{name: "OpenAI raw passthrough", channelType: constant.ChannelTypeOpenAI, status: http.StatusOK},
		{name: "OpenAI model mapping preserves large numbers", channelType: constant.ChannelTypeOpenAI, mapped: true, status: http.StatusOK},
		{name: "OpenAI channel overrides still apply", channelType: constant.ChannelTypeOpenAI, override: true, status: http.StatusOK},
		{name: "OpenAI upstream error is not settled", channelType: constant.ChannelTypeOpenAI, status: http.StatusBadGateway},
		{name: "Sub2API remains supported", channelType: constant.ChannelTypeSub2API, status: http.StatusOK},
		{name: "NewAPI remains supported", channelType: constant.ChannelTypeNewAPI, status: http.StatusOK},
		{name: "unsupported channel remains retryable", channelType: constant.ChannelTypeGemini},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotURI, gotMethod, gotAuth, gotContentType, gotAccept, gotOverride string
			var gotBody []byte
			var readErr error
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotURI, gotMethod = r.RequestURI, r.Method
				gotAuth, gotContentType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
				gotAccept, gotOverride = r.Header.Get("Accept"), r.Header.Get("X-Search-Feature")
				gotBody, readErr = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(tc.status)
				if tc.status != http.StatusOK {
					_, _ = io.WriteString(w, `{"error":{"type":"upstream_error","message":"search unavailable"}}`)
					return
				}
				_, _ = io.WriteString(w, response)
			}))
			t.Cleanup(upstream.Close)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(raw))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("Accept", "application/json")
			c.Request.Header.Set("Authorization", "Bearer downstream-token")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "o1")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyChannelType, tc.channelType)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "sk-upstream-test")
			if tc.mapped {
				c.Set("model_mapping", `{"o1":"provider-model"}`)
			}
			if tc.override {
				common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"locale": "zh"})
				common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, map[string]any{"X-Search-Feature": "enabled"})
			}
			request, err := helper.GetAndValidateRequest(c, types.RelayFormatOpenAIAlphaSearch)
			require.NoError(t, err)
			info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAIAlphaSearch, request, nil)
			require.NoError(t, err)
			info.UpstreamHTTPClient = upstream.Client()
			info.PriceData.GroupRatioInfo.GroupRatio = 1
			settlement := &recordedSettlement{}
			info.Billing = settlement

			apiErr := AlphaSearchHelper(c, info)
			if tc.status == 0 {
				require.NotNil(t, apiErr)
				assert.False(t, types.IsSkipRetryError(apiErr))
				assert.Empty(t, gotURI)
				assert.Empty(t, settlement.settled)
				return
			}
			require.NoError(t, readErr)
			wantURI := path
			if tc.channelType != constant.ChannelTypeOpenAI {
				// Existing NewAPI/Sub2API adaptors use a fixed endpoint path.
				wantURI = strings.Split(path, "?")[0]
			}
			assert.Equal(t, wantURI, gotURI)
			assert.Equal(t, http.MethodPost, gotMethod)
			assert.Equal(t, "Bearer sk-upstream-test", gotAuth)
			assert.Equal(t, "application/json", gotContentType)
			assert.Equal(t, "application/json", gotAccept)
			if tc.override {
				assert.Equal(t, "zh", gjson.GetBytes(gotBody, "locale").String())
				assert.Equal(t, "enabled", gotOverride)
				assert.Equal(t, "weather", gjson.GetBytes(gotBody, "commands.search_query.0.q").String())
			} else {
				want := raw
				if tc.mapped {
					want = strings.Replace(raw, `"model":"o1"`, `"model":"provider-model"`, 1)
				}
				assert.Equal(t, want, string(gotBody))
			}
			if tc.status != http.StatusOK {
				require.NotNil(t, apiErr)
				assert.Equal(t, tc.status, apiErr.StatusCode)
				assert.Contains(t, apiErr.Error(), "search unavailable")
				assert.False(t, c.Writer.Written(), "leave errors to the existing retry/error handler")
				assert.Empty(t, settlement.settled)
				assert.Zero(t, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].CallCount)
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
			assert.Equal(t, response, recorder.Body.String())
			assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].CallCount)
			assert.Equal(t, []int{5000}, settlement.settled, "one search call and zero tokens at the default price")
		})
	}
}

type recordedSettlement struct {
	settled  []int
	refunded bool
}

func (s *recordedSettlement) Settle(quota int) error {
	s.settled = append(s.settled, quota)
	return nil
}
func (s *recordedSettlement) Refund(*gin.Context)    { s.refunded = true }
func (*recordedSettlement) NeedsRefund() bool        { return false }
func (*recordedSettlement) GetPreConsumedQuota() int { return 0 }
func (*recordedSettlement) Reserve(int) error        { return nil }

// /v1/tools/search and /v1/tools/fetch follow /v1/alpha/search: the raw body
// reaches /v1/tools/* unchanged and the response returns unchanged. With only
// the built-in prices, a response DeepSeek bills (results, page content)
// settles one call at the routing name's request price and no tokens, an
// empty one releases the reservation, an upstream error keeps its status, and
// a channel that cannot relay the endpoint fails retryably without sending
// anything.
func TestDeepSeekToolsHelper(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)
	batch, logConsume := common.BatchUpdateEnabled, common.LogConsumeEnabled
	common.BatchUpdateEnabled, common.LogConsumeEnabled = true, false
	t.Cleanup(func() { common.BatchUpdateEnabled, common.LogConsumeEnabled = batch, logConsume })

	const search, fetch = "/v1/tools/search", "/v1/tools/fetch"
	const results = `{"results":[{"title":"DeepSeek","url":"https://deepseek.com/","rerank_score":2.1}]}`
	tests := []struct {
		name, path, body, response string
		channelType, status        int
		price                      float64
		settled                    []int
		refunded                   bool
	}{
		{name: "search with results", path: search, body: `{"query":"deepseek","count":2,"future_field":{"nested":true}}`,
			response: results, channelType: constant.ChannelTypeDeepSeek, status: http.StatusOK, settled: []int{375}},
		{name: "search with configured request price", path: search, body: `{"query":"deepseek"}`,
			response: results, channelType: constant.ChannelTypeDeepSeek, status: http.StatusOK, price: 0.002, settled: []int{1000}},
		{name: "search without results", path: search, body: `{"query":"site:none.invalid deepseek"}`,
			response: `{"results":[]}`, channelType: constant.ChannelTypeDeepSeek, status: http.StatusOK, refunded: true},
		{name: "fetch with page content", path: fetch, body: `{"url":"https://example.com/"}`,
			response: `{"title":"Example Domain","content":"# Example Domain"}`, channelType: constant.ChannelTypeDeepSeek, status: http.StatusOK, settled: []int{375}},
		{name: "fetch failure keeps its status", path: fetch, body: `{"url":"https://example.com/missing"}`,
			response:    `{"error":{"message":"failed to fetch URL","type":"not_found_error","param":null,"code":"invalid_request_error"}}`,
			channelType: constant.ChannelTypeDeepSeek, status: http.StatusNotFound},
		{name: "search through a New API channel", path: search, body: `{"query":"deepseek"}`,
			response: results, channelType: constant.ChannelTypeNewAPI, status: http.StatusOK, settled: []int{375}},
		{name: "channel without the endpoint", path: search, body: `{"query":"deepseek"}`, channelType: constant.ChannelTypeOpenAI},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			var gotBody []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			t.Cleanup(upstream.Close)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			routingName := map[string]string{search: constant.DeepSeekSearchModel, fetch: constant.DeepSeekFetchModel}[tc.path]
			common.SetContextKey(c, constant.ContextKeyOriginalModel, routingName)
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyChannelType, tc.channelType)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "sk-test")

			request, err := helper.GetAndValidateRequest(c, types.RelayFormatDeepSeekTools)
			require.NoError(t, err)
			info, err := relaycommon.GenRelayInfo(c, types.RelayFormatDeepSeekTools, request, nil)
			require.NoError(t, err)
			info.BillingRequestInput = &billingexpr.RequestInput{Body: []byte(tc.body)}
			_, err = helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
			require.NoError(t, err, "the routing name is priced out of the box")
			if tc.price != 0 {
				info.TieredBillingSnapshot = nil
				info.PriceData.UsePrice = true
				info.PriceData.ModelPrice = tc.price
			}
			settlement := &recordedSettlement{}
			info.Billing = settlement

			apiErr := DeepSeekToolsHelper(c, info)
			if tc.status == 0 {
				require.NotNil(t, apiErr)
				assert.False(t, types.IsSkipRetryError(apiErr))
				assert.Empty(t, gotPath)
				return
			}
			assert.Equal(t, tc.path, gotPath)
			assert.Equal(t, tc.body, string(gotBody))
			assert.Equal(t, tc.settled, settlement.settled)
			assert.Equal(t, tc.refunded, settlement.refunded)
			if tc.status != http.StatusOK {
				require.NotNil(t, apiErr)
				assert.Equal(t, tc.status, apiErr.StatusCode)
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, tc.status, recorder.Code)
			assert.Equal(t, tc.response, recorder.Body.String())
		})
	}
}
