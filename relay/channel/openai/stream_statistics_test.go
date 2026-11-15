package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectStreamStatisticsPreservesModeErrors(t *testing.T) {
	for _, tc := range []struct {
		name, data, text string
		mode             int
		wantError        bool
		names            []string
	}{
		{name: "malformed chat", mode: relayconstant.RelayModeChatCompletions, data: `{"choices":`, wantError: true},
		{name: "chat field type discards partial tools", mode: relayconstant.RelayModeChatCompletions, data: `{"choices":[{"delta":{"tool_calls":[{"id":"call_a","function":{"name":"lookup","arguments":"{}"}}],"content":123}}]}`, wantError: true},
		{name: "legacy ignores invalid chat delta", mode: relayconstant.RelayModeCompletions, data: `{"choices":[{"text":"legacy","delta":{"content":false}}]}`, text: "legacy"},
		{name: "legacy field type retains tool collection", mode: relayconstant.RelayModeCompletions, data: `{"choices":[{"text":false,"delta":{"tool_calls":[{"id":"call_a","function":{"name":"lookup","arguments":"{}"}}]}}]}`, wantError: true, names: []string{"lookup"}},
		{name: "unrelated mode ignores malformed chat", mode: relayconstant.RelayModeEmbeddings, data: `{"choices":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var text strings.Builder
			var toolCount int
			var names []string
			err := collectStreamStatistics(tc.mode, tc.data, &text, &toolCount, make(map[string]struct{}), &names)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.text, text.String())
			assert.Zero(t, toolCount)
			assert.Equal(t, tc.names, names)
		})
	}
}

func runOaiStatisticsStream(t *testing.T, mode int, events []string) (*dto.Usage, *relaycommon.RelayInfo, string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "statistics-test-model",
		RelayFormat:     types.RelayFormatOpenAI,
		RelayMode:       mode,
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "statistics-test-model"},
	}
	info.SetEstimatePromptTokens(23)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: " + strings.Join(events, "\n\ndata: ") + "\n\ndata: [DONE]\n\n")),
	}
	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	return usage, info, w.Body.String()
}

func TestOaiStreamStatisticsPreservesFallbackAndToolBilling(t *testing.T) {
	const toolName = "statistics_lookup"
	operation_setting.SetToolPriceForTest(toolName, 5)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest(toolName) })
	events := []string{
		`{"choices":[{"index":0,"delta":{"content":"answer ","reasoning_content":"thinking ","tool_calls":[{"index":0,"function":{"name":"statistics_lookup","arguments":"{"}},{"index":1,"id":"call_b","function":{"name":"statistics_lookup","arguments":"{}"}}]}}],"extension":{"keep":"unchanged"}}`,
		`{"choices":[{"index":0,"delta":{"content":"tail","reasoning":"reasoned","tool_calls":[{"index":0,"id":"call_a","function":{"name":"statistics_lookup","arguments":"}"}}]}}]}`,
		`{"choices":[{"index":1,"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"statistics_lookup","arguments":""}}]}}]}`,
		`{"choices":[]}`,
	}
	usage, info, body := runOaiStatisticsStream(t, relayconstant.RelayModeChatCompletions, events)
	text := "answer thinking statistics_lookup{statistics_lookup{}tailreasonedstatistics_lookup}statistics_lookup"
	baseCompletionTokens := service.EstimateTokenByModel(info.UpstreamModelName, text)
	assert.Equal(t, 23, usage.PromptTokens)
	assert.Equal(t, baseCompletionTokens+14, usage.CompletionTokens, "retain the existing two-tool token allowance")
	assert.Equal(t, 23+baseCompletionTokens, usage.TotalTokens, "preserve the existing fallback usage contract")
	require.NotNil(t, info.ResponsesUsageInfo)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, toolName)
	assert.Equal(t, 3, info.ResponsesUsageInfo.BuiltInTools[toolName].CallCount, "delayed IDs dedupe; different choices remain distinct")
	assert.False(t, info.StreamStatus.HasErrors())
	for _, event := range events {
		assert.Contains(t, body, "data: "+event+"\n", "statistics must not reserialize forwarded events")
	}
}

func TestOaiStreamStatisticsPreservesUpstreamUsage(t *testing.T) {
	usageEvent := `{"choices":[],"usage":{"prompt_tokens":101,"completion_tokens":17,"total_tokens":118,"prompt_tokens_details":{"cached_tokens":88}}}`
	for _, penultimate := range []bool{false, true} {
		name := "final usage"
		if penultimate {
			name = "penultimate usage"
		}
		t.Run(name, func(t *testing.T) {
			events := []string{`{"choices":[{"delta":{"content":"answer","reasoning_content":"thinking"}}]}`, usageEvent}
			if penultimate {
				events = append(events, `{"choices":[]}`)
			}
			usage, info, body := runOaiStatisticsStream(t, relayconstant.RelayModeChatCompletions, events)
			assert.Equal(t, 101, usage.PromptTokens)
			assert.Equal(t, 17, usage.CompletionTokens)
			assert.Equal(t, 118, usage.TotalTokens)
			assert.Equal(t, 88, usage.PromptTokensDetails.CachedTokens)
			assert.False(t, info.StreamStatus.HasErrors())
			assert.Contains(t, body, `"reasoning_content":"thinking"`)
		})
	}
}

func TestOaiStreamStatisticsRecordsMalformedEventOnce(t *testing.T) {
	for _, malformed := range []string{`{"choices":`, `{"choices":[{"delta":{"content":123}}]}`} {
		t.Run(malformed, func(t *testing.T) {
			events := []string{
				`{"choices":[{"delta":{"content":"answer"}}]}`,
				malformed,
				`{"choices":[],"usage":{"prompt_tokens":101,"completion_tokens":17,"total_tokens":118}}`,
			}
			usage, info, _ := runOaiStatisticsStream(t, relayconstant.RelayModeChatCompletions, events)
			assert.Equal(t, 118, usage.TotalTokens)
			assert.Equal(t, 1, info.StreamStatus.TotalErrorCount())
			assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
		})
	}
}

func TestOaiStreamStatisticsPreservesLegacyCompletionText(t *testing.T) {
	events := []string{
		`{"choices":[{"index":0,"text":"legacy "},{"index":1,"text":"alternate "}]}`,
		`{"choices":[{"index":0,"text":"completion"}]}`,
	}
	usage, info, body := runOaiStatisticsStream(t, relayconstant.RelayModeCompletions, events)
	wantCompletion := service.EstimateTokenByModel(info.UpstreamModelName, "legacy alternate completion")
	assert.Equal(t, 23, usage.PromptTokens)
	assert.Equal(t, wantCompletion, usage.CompletionTokens)
	assert.Equal(t, 23+wantCompletion, usage.TotalTokens)
	assert.False(t, info.StreamStatus.HasErrors())
	assert.Contains(t, body, "data: "+events[0]+"\n")
}
