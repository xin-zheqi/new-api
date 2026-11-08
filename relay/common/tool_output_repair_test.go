package common

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestToolOutputRepairPreservesRealResultsAndClosesEachBatch(t *testing.T) {
	body := []byte(`{"reasoning":{"effort":"none"},"input":[
		{"type":"function_call","id":"fc_a","call_id":"a","name":"read","arguments":"{}"},
		{"type":"function_call","call_id":"b","name":"write","arguments":"{}"},
		{"type":"function_call_output","call_id":"a","output":"","extension":42},
		{"role":"user","content":"continue"},
		{"type":"custom_tool_call","call_id":"c","name":"patch","input":"actual patch"}
	],"unknown":{"preserve":true}}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	got, err := RepairToolOutputRequest(body, info)
	require.NoError(t, err)
	require.True(t, gjson.ValidBytes(got))
	items := gjson.GetBytes(got, "input").Array()
	require.Len(t, items, 7)
	assert.Equal(t, `{"type":"function_call_output","call_id":"a","output":"","extension":42}`, items[2].Raw)
	assert.Equal(t, "b", items[3].Get("call_id").String())
	assert.Equal(t, "function_call_output", items[3].Get("type").String())
	assert.Equal(t, "user", items[4].Get("role").String())
	assert.Equal(t, "c", items[6].Get("call_id").String())
	assert.Equal(t, "custom_tool_call_output", items[6].Get("type").String())
	for _, index := range []int{3, 6} {
		result := gjson.Parse(items[index].Get("output").String())
		assert.Equal(t, "tool_output_unavailable", result.Get("error.code").String())
		assert.Contains(t, result.Get("error.message").String(), "Execution status is unknown")
		assert.Contains(t, result.Get("error.message").String(), "Do not assume success")
	}
	assert.True(t, gjson.GetBytes(got, "unknown.preserve").Bool())
	assert.Equal(t, &ToolOutputRepairStats{MissingOutputs: 2}, info.ToolOutputRepairStats)
	assert.NotContains(t, string(body), "tool_output_unavailable", "retry source is immutable")
	again, err := RepairToolOutputRequest(got, info)
	require.NoError(t, err)
	assert.Equal(t, got, again, "repair must be idempotent")
	assert.Nil(t, info.ToolOutputRepairStats)
}

func TestToolOutputRepairRelinksOnlyUnambiguousExistingResults(t *testing.T) {
	for _, reference := range []string{`"call_id":"fc_a"`, `"tool_call_id":"a"`} {
		t.Run(reference, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"function_call","id":"fc_a","call_id":"a","name":"read","arguments":"{}"},{"type":"function_call","call_id":"b","name":"read","arguments":"{}"},{"type":"function_call_output",` + reference + `,"output":[{"type":"input_text","text":"REAL RESULT"}],"extension":0}]}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
			got, err := RepairToolOutputRequest(body, info)
			require.NoError(t, err)
			require.True(t, gjson.ValidBytes(got))
			assert.Equal(t, "a", gjson.GetBytes(got, "input.2.call_id").String())
			assert.Equal(t, `[{"type":"input_text","text":"REAL RESULT"}]`, gjson.GetBytes(got, "input.2.output").Raw)
			assert.Equal(t, "0", gjson.GetBytes(got, "input.2.extension").Raw)
			assert.Equal(t, "b", gjson.GetBytes(got, "input.3.call_id").String())
			assert.Equal(t, &ToolOutputRepairStats{MissingOutputs: 1, RelinkedOutputs: 1}, info.ToolOutputRepairStats)
		})
	}
}

func TestToolOutputRepairLeavesUnsafeAndCompleteHistoryUnchanged(t *testing.T) {
	call := `{"type":"function_call","id":"fc_a","call_id":"a","name":"read","arguments":"{}"}`
	for _, tc := range []struct{ name, input, extra string }{
		{"real result", call + `,{"type":"function_call_output","call_id":"a","output":"actual result"}`, ""},
		{"item reference", call + `,{"type":"item_reference","id":"item_1"}`, ""},
		{"still running", strings.Replace(call, `"name":`, `"status":"in_progress","name":`, 1), ""},
		{"truncated arguments", strings.Replace(call, `"{}"`, `"{"`, 1), ""},
		{"duplicate call", call + "," + call, ""},
		{"conflicting alias", call + `,{"type":"function_call","call_id":"fc_a","name":"read","arguments":"{}"}`, ""},
		{"unknown result id", call + `,{"type":"function_call_output","call_id":"unknown","output":"real result"}`, ""},
		{"malformed primary id", call + `,{"type":"function_call_output","call_id":123,"tool_call_id":"a","output":"real result"}`, ""},
		{"malformed legacy id", strings.Replace(call, `"call_id":"a"`, `"call_id":"123"`, 1) + `,{"type":"function_call_output","tool_call_id":123,"output":"real result"}`, ""},
		{"signed result identity", call + `,{"type":"function_call_output","call_id":"fc_a","output":"real result","signature":"signed"}`, ""},
		{"wrong result type", call + `,{"type":"custom_tool_call_output","call_id":"a","output":"real result"}`, ""},
		{"missing result value", call + `,{"type":"function_call_output","call_id":"a"}`, ""},
		{"duplicate result", call + `,{"type":"function_call_output","call_id":"a","output":"one"},{"type":"function_call_output","call_id":"a","output":"two"}`, ""},
		{"out of order result", `{"type":"function_call_output","call_id":"a","output":"real"},` + call, ""},
		{"result across user turn", call + `,{"role":"user","content":"continue"},{"type":"function_call_output","call_id":"a","output":"real"}`, ""},
		{"hosted tool", `{"type":"web_search_call","id":"search_1","status":"completed"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"input":[` + tc.input + `]` + tc.extra + `}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
			got, err := RepairToolOutputRequest(body, info)
			require.NoError(t, err)
			assert.Equal(t, body, got)
			assert.Nil(t, info.ToolOutputRepairStats)
		})
	}
}

func TestToolOutputRepairInspectsExplicitHistoryWithStatefulIdentifiers(t *testing.T) {
	for _, state := range []string{`"previous_response_id":"resp_1"`, `"conversation":{"id":"conv_1"}`} {
		t.Run(state, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"function_call","call_id":"a","name":"read","arguments":"{}"}],` + state + `}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
			got, err := RepairToolOutputRequest(body, info)
			require.NoError(t, err)
			assert.Equal(t, "a", gjson.GetBytes(got, "input.1.call_id").String())
			assert.Equal(t, "tool_output_unavailable", gjson.Parse(gjson.GetBytes(got, "input.1.output").String()).Get("error.code").String())
		})
	}
}

func TestRepairToolOutputFromUpstreamErrorUsesProviderCallID(t *testing.T) {
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	body := []byte(`{"previous_response_id":"resp_remote","input":[{"role":"user","content":"continue"}]}`)
	got, repaired, err := RepairToolOutputFromUpstreamError(body, []byte(`{"error":{"message":"No tool output found for tool call call_remote_01"}}`), info)
	require.NoError(t, err)
	assert.Equal(t, 1, repaired)
	assert.Equal(t, "call_remote_01", gjson.GetBytes(got, "input.1.call_id").String())
	assert.Equal(t, "tool_output_unavailable", gjson.Parse(gjson.GetBytes(got, "input.1.output").String()).Get("error.code").String())
	assert.Equal(t, &ToolOutputRepairStats{MissingOutputs: 1}, info.ToolOutputRepairStats)

	unchanged, repaired, err := RepairToolOutputFromUpstreamError(got, []byte(`{"error":{"message":"No tool output found for tool call call_remote_01"}}`), info)
	require.NoError(t, err)
	assert.Equal(t, 0, repaired)
	assert.Equal(t, got, unchanged)
}

func TestRepairToolOutputFromUpstreamErrorIgnoresUnrelatedErrors(t *testing.T) {
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	body := []byte(`{"input":[{"role":"user","content":"continue"}]}`)
	got, repaired, err := RepairToolOutputFromUpstreamError(body, []byte(`{"error":{"message":"invalid tool choice"}}`), info)
	require.NoError(t, err)
	assert.Equal(t, 0, repaired)
	assert.Equal(t, body, got)
}

func TestRepairToolOutputFromUpstreamErrorReplacesMalformedOutput(t *testing.T) {
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	body := []byte(`{"input":[{"type":"function_call","call_id":"call_bad","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_bad"}]}`)
	got, repaired, err := RepairToolOutputFromUpstreamError(body, []byte(`No tool output found for tool call call_bad`), info)
	require.NoError(t, err)
	assert.Equal(t, 1, repaired)
	assert.Equal(t, "tool_output_unavailable", gjson.Parse(gjson.GetBytes(got, "input.1.output").String()).Get("error.code").String())
}

func TestDeepSeekToolOutputRepairInspectsExplicitHistoryWithStatefulIDs(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_remote","input":[{"type":"function_call","call_id":"a","name":"read","arguments":"{}"}]}`)
	original := string(body)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek, ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	got, err := RepairToolOutputRequest(body, info)
	require.NoError(t, err)
	items := gjson.GetBytes(got, "input").Array()
	require.Len(t, items, 2)
	assert.Equal(t, "a", items[1].Get("call_id").String())
	assert.Equal(t, "tool_output_unavailable", gjson.Parse(items[1].Get("output").String()).Get("error.code").String())
	assert.Equal(t, original, string(body), "the retry source must remain immutable")
	assert.NotContains(t, string(body), "tool_output_unavailable")
}

func TestToolOutputRepairGatesAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		enabled, passthrough bool
		format               types.RelayFormat
	}{
		{"default off", false, false, types.RelayFormatOpenAIResponses},
		{"passthrough", true, true, types.RelayFormatOpenAIResponses},
		{"chat body without messages", true, false, types.RelayFormatOpenAI},
		{"native claude", true, false, types.RelayFormatClaude},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"function_call","call_id":"a","name":"read","arguments":"{}"}]}`)
			info := &RelayInfo{RelayFormat: tc.format, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: tc.enabled, PassThroughBodyEnabled: tc.passthrough}}, ToolOutputRepairStats: &ToolOutputRepairStats{MissingOutputs: 1}}
			got, err := RepairToolOutputRequest(body, info)
			require.NoError(t, err)
			assert.Equal(t, body, got)
			assert.Nil(t, info.ToolOutputRepairStats, "previous attempt audit must not leak")
		})
	}
	var input []map[string]any
	for i := range 257 {
		input = append(input, map[string]any{"type": "function_call", "call_id": strconv.Itoa(i), "name": "read", "arguments": "{}"})
	}
	body, err := common.Marshal(map[string]any{"input": input})
	require.NoError(t, err)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	got, err := RepairToolOutputRequest(body, info)
	require.NoError(t, err)
	assert.Equal(t, body, got, "oversized history must not be partially repaired")
}

func TestChatToolOutputRepairClosesEachBatchAndPreservesResults(t *testing.T) {
	body := []byte(`{"model":"arbitrary-chat-model","messages":[
		{"role":"assistant","content":null,"tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"write","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"a","content":"REAL RESULT","extension":42},
		{"role":"user","content":"continue"},
		{"role":"assistant","content":null,"tool_calls":[{"id":"c","type":"function","function":{"name":"patch","arguments":"{}"}}]},
		{"role":"assistant","content":"next answer"},
		{"role":"user","content":"finish"}
	],"unknown":{"preserve":true}}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	got, err := RepairToolOutputRequest(body, info)
	require.NoError(t, err)
	require.True(t, gjson.ValidBytes(got))
	messages := gjson.GetBytes(got, "messages").Array()
	require.Len(t, messages, 8)
	assert.Equal(t, "a", messages[1].Get("tool_call_id").String())
	assert.Equal(t, "REAL RESULT", messages[1].Get("content").String())
	assert.Equal(t, "42", messages[1].Get("extension").Raw)
	assert.Equal(t, "b", messages[2].Get("tool_call_id").String())
	assert.Equal(t, "user", messages[3].Get("role").String())
	assert.Equal(t, "c", messages[5].Get("tool_call_id").String())
	assert.Equal(t, "next answer", messages[6].Get("content").String())
	assert.True(t, gjson.GetBytes(got, "unknown.preserve").Bool())
	for _, index := range []int{2, 5} {
		assert.Equal(t, "tool", messages[index].Get("role").String())
		result := gjson.Parse(messages[index].Get("content").String())
		assert.Equal(t, "tool_output_unavailable", result.Get("error.code").String())
		assert.Contains(t, result.Get("error.message").String(), "Execution status is unknown")
	}
	assert.Equal(t, &ToolOutputRepairStats{MissingOutputs: 2}, info.ToolOutputRepairStats)
	assert.NotContains(t, string(body), "tool_output_unavailable", "retry source is immutable")
	again, err := RepairToolOutputRequest(got, info)
	require.NoError(t, err)
	assert.Equal(t, got, again, "repair must be idempotent")
	assert.Nil(t, info.ToolOutputRepairStats)
}

func TestNewChannelJSONBodyAppliesChatToolOutputRepair(t *testing.T) {
	body := []byte(`{"model":"arbitrary-chat-model","messages":[{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]},{"role":"user","content":"continue"}]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	requestBody, closer, err := NewChannelJSONBody(body, info)
	require.NoError(t, err)
	defer closer.Close()
	got, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	messages := gjson.GetBytes(got, "messages").Array()
	require.Len(t, messages, 3)
	assert.Equal(t, "tool", messages[1].Get("role").String())
	assert.Equal(t, "a", messages[1].Get("tool_call_id").String())
	assert.Equal(t, &ToolOutputRepairStats{MissingOutputs: 1}, info.ToolOutputRepairStats)
}

func TestChatToolOutputRepairReplacesMissingResultContent(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]},{"role":"tool","tool_call_id":"a","content":null,"extra":"keep"}]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	got, err := RepairToolOutputRequest(body, info)
	require.NoError(t, err)
	messages := gjson.GetBytes(got, "messages").Array()
	require.Len(t, messages, 2)
	result := gjson.Parse(messages[1].Get("content").String())
	assert.Equal(t, "tool_output_unavailable", result.Get("error.code").String())
	assert.Equal(t, "keep", messages[1].Get("extra").String())
	assert.Equal(t, &ToolOutputRepairStats{MissingOutputs: 1}, info.ToolOutputRepairStats)
}

func TestChatToolOutputRepairLeavesUnsafeHistoryUnchanged(t *testing.T) {
	call := `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]}`
	for _, tc := range []struct{ name, messages string }{
		{"complete", call + `,{"role":"tool","tool_call_id":"a","content":"real"}`},
		{"duplicate call id", `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}},{"id":"a","type":"function","function":{"name":"write","arguments":"{}"}}]}`},
		{"malformed call id", `{"role":"assistant","tool_calls":[{"id":123,"type":"function","function":{"name":"read","arguments":"{}"}}]}`},
		{"orphan result", `{"role":"tool","tool_call_id":"a","content":"real"}` + `,` + call},
		{"duplicate result", call + `,{"role":"tool","tool_call_id":"a","content":"one"},{"role":"tool","tool_call_id":"a","content":"two"}`},
		{"result after user", call + `,{"role":"user","content":"continue"},{"role":"tool","tool_call_id":"a","content":"real"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"messages":[` + tc.messages + `]}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
			got, err := RepairToolOutputRequest(body, info)
			require.NoError(t, err)
			assert.Equal(t, body, got)
			assert.Nil(t, info.ToolOutputRepairStats)
		})
	}
}

func TestChatToolOutputRepairAllowsIDsReusedAcrossBatches(t *testing.T) {
	call := `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]}`
	body := []byte(`{"messages":[` + call + `,{"role":"tool","tool_call_id":"a","content":"first result"},{"role":"user","content":"again"},` + call + `,{"role":"user","content":"continue"}]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	got, err := RepairToolOutputRequest(body, info)
	require.NoError(t, err)
	messages := gjson.GetBytes(got, "messages").Array()
	require.Len(t, messages, 6)
	assert.Equal(t, "first result", messages[1].Get("content").String())
	assert.Equal(t, "a", messages[4].Get("tool_call_id").String())
	assert.Equal(t, "user", messages[5].Get("role").String())
	assert.Equal(t, &ToolOutputRepairStats{MissingOutputs: 1}, info.ToolOutputRepairStats)
}

func TestChatToolOutputRepairBoundsCallCount(t *testing.T) {
	calls := make([]map[string]any, 0, 257)
	for i := range 257 {
		calls = append(calls, map[string]any{"id": strconv.Itoa(i), "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}})
	}
	body, err := common.Marshal(map[string]any{"messages": []any{map[string]any{"role": "assistant", "tool_calls": calls}}})
	require.NoError(t, err)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	got, err := RepairToolOutputRequest(body, info)
	require.NoError(t, err)
	assert.Equal(t, body, got, "oversized Chat history must not be partially repaired")
	assert.Nil(t, info.ToolOutputRepairStats)
}

func BenchmarkToolOutputRepairRequest(b *testing.B) {
	body := []byte(`{"input":[{"role":"user","content":"` + strings.Repeat("context ", 16384) + `"}]}`)
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled_128KiB_no_repair"
		}
		b.Run(name, func(b *testing.B) {
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: enabled}}}
			b.ReportAllocs()
			for b.Loop() {
				_, err := RepairToolOutputRequest(body, info)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
