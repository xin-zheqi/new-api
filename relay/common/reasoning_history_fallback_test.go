package common

import (
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMissingDeepSeekReasoningHistoryIsMarkedWithoutChangingExistingItems(t *testing.T) {
	for _, channel := range []int{constant.ChannelTypeDeepSeek, constant.ChannelTypeOpenAI} {
		for _, tc := range []struct {
			name, input string
			wantItems   int
		}{
			{"old assistant answer", `{"role":"assistant","content":"old answer"}`, 2},
			{"parallel calls", `{"type":"function_call","call_id":"a","name":"read","arguments":"{}"},{"type":"function_call","call_id":"b","name":"read","arguments":"{}"}`, 3},
			{"custom call", `{"type":"custom_tool_call","call_id":"a","name":"apply_patch","input":"actual patch"}`, 2},
		} {
			t.Run(tc.name+" channel "+strconv.Itoa(channel), func(t *testing.T) {
				body := []byte(`{"model":"deepseek-v4.1-flash","reasoning":{"effort":"high"},"tools":[{"type":"function","name":"read"}],"input":[` + tc.input + `]}`)
				info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: channel, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
				got, err := RepairReasoningRequest(body, info)
				require.NoError(t, err)
				require.True(t, gjson.ValidBytes(got))
				items := gjson.GetBytes(got, "input").Array()
				require.Len(t, items, tc.wantItems)
				assert.Equal(t, "reasoning", items[0].Get("type").String())
				assert.Equal(t, "[]", items[0].Get("summary").Raw)
				assert.Equal(t, "reasoning_text", items[0].Get("content.0.type").String())
				assert.Contains(t, items[0].Get("content.0.text").String(), "历史思考内容缺失")
				assert.Contains(t, items[0].Get("content.0.text").String(), "不是模型原始思考")
				original := gjson.GetBytes(body, "input").Array()
				for i := range original {
					assert.Equal(t, original[i].Raw, items[i+1].Raw)
				}
				again, err := RepairReasoningRequest(got, info)
				require.NoError(t, err)
				assert.Equal(t, got, again, "replayed history must not accumulate placeholders")
				assert.NotContains(t, string(body), "历史思考内容缺失", "the retry source stays immutable")
			})
		}
	}
}

func TestDeepSeekFallbackRepairsEachMissingTurnAndPreservesRealReasoning(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash","tools":[{"type":"function","name":"read"}],"input":[
		{"type":"reasoning","id":"real","content":[{"type":"reasoning_text","text":"original full reasoning"}],"extra":42},
		{"type":"function_call","call_id":"a","name":"read","arguments":"{}"},
		{"type":"function_call_output","call_id":"a","output":"REAL TOOL RESULT"},
		{"role":"assistant","content":"old answer without reasoning"},
		{"role":"user","content":"continue"},
		{"type":"reasoning","id":"summary_only","summary":[{"type":"summary_text","text":"not original reasoning"}]},
		{"type":"function_call","call_id":"b","name":"read","arguments":"{}"}
	]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
	got, err := RepairReasoningRequest(body, info)
	require.NoError(t, err)
	items := gjson.GetBytes(got, "input").Array()
	require.Len(t, items, 8)
	before := gjson.GetBytes(body, "input").Array()
	for i := range 3 {
		assert.Equal(t, before[i].Raw, items[i].Raw)
	}
	assert.Contains(t, items[3].Get("content.0.text").String(), "历史思考内容缺失")
	assert.Equal(t, before[3].Raw, items[4].Raw)
	assert.Equal(t, before[4].Raw, items[5].Raw)
	assert.Equal(t, before[5].Get("summary").Raw, items[6].Get("summary").Raw)
	assert.Contains(t, items[6].Get("content.0.text").String(), "历史思考内容缺失")
	assert.Equal(t, before[6].Raw, items[7].Raw)
}

func TestReasoningHistoryFallbackLeavesNormalOrIneligibleRequestsUnchanged(t *testing.T) {
	base := `{"model":"deepseek-v4.1-flash","reasoning":{"effort":"high"},"tools":[{"type":"function","name":"read"}],"input":[{"role":"assistant","content":"answer"}]}`
	for _, tc := range []struct {
		name, body            string
		disabled, passthrough bool
	}{
		{"off", base, true, false},
		{"body passthrough", base, false, true},
		{"thinking off", strings.Replace(base, `"high"`, `"none"`, 1), false, false},
		{"other model", strings.Replace(base, "deepseek-v4.1-flash", "gpt-test", 1), false, false},
		{"remote history", strings.Replace(base, `"input":`, `"previous_response_id":"remote","input":`, 1), false, false},
		{"item reference", strings.Replace(base, `"input":[`, `"input":[{"type":"item_reference","id":"remote"},`, 1), false, false},
		{"unfinished call", strings.Replace(base, `{"role":"assistant","content":"answer"}`, `{"type":"function_call","status":"in_progress","call_id":"a","name":"read","arguments":"{"}`, 1), false, false},
		{"real reasoning before answer", strings.Replace(base, `"input":[`, `"input":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"REAL"}]},`, 1), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: !tc.disabled, PassThroughBodyEnabled: tc.passthrough}}}
			got, err := RepairReasoningRequest([]byte(tc.body), info)
			require.NoError(t, err)
			assert.Equal(t, tc.body, string(got))
		})
	}
}

func TestDeepSeekFallbackDoesNotRequireToolsDeclaration(t *testing.T) {
	// ZCode/Sub2API can replay a prior assistant turn without carrying the
	// current tools array. DeepSeek still validates the prior reasoning item,
	// so the local history must be repaired from the turn itself.
	body := []byte(`{"model":"deepseek-v4.1-flash","reasoning":{"effort":"high"},"input":[{"role":"assistant","content":"old answer"},{"role":"user","content":"continue"}]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
	got, err := RepairReasoningRequest(body, info)
	require.NoError(t, err)
	items := gjson.GetBytes(got, "input").Array()
	require.Len(t, items, 3)
	assert.Equal(t, "reasoning", items[0].Get("type").String())
	assert.Contains(t, items[0].Get("content.0.text").String(), "历史思考内容缺失")
	assert.Equal(t, gjson.GetBytes(body, "input.0").Raw, items[1].Raw)
	assert.Equal(t, gjson.GetBytes(body, "input.1").Raw, items[2].Raw)
}

func TestReasoningItemsAlwaysKeepSummaryOrContentWhenRepairEnabled(t *testing.T) {
	for _, payload := range []string{``, `,"summary":null,"content":null`, `,"encrypted_content":"OPAQUE_CIPHERTEXT"`} {
		body := []byte(`{"model":"gpt-test","input":[{"type":"reasoning","id":"r1"` + payload + `}]}`)
		info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
		got, err := RepairReasoningRequest(body, info)
		require.NoError(t, err)
		assert.Equal(t, "[]", gjson.GetBytes(got, "input.0.summary").Raw)
		assert.Equal(t, gjson.GetBytes(body, "input.0.encrypted_content").Raw, gjson.GetBytes(got, "input.0.encrypted_content").Raw)
		assert.NotContains(t, string(got), "历史思考内容缺失", "non-DeepSeek requests do not receive plaintext placeholders")
	}
}

func TestThinkingDisabledOnlyNormalizesExistingResponsesItem(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4.1-flash","reasoning":{"effort":"none"},"tools":[{"type":"function","name":"read"}],"input":[{"type":"reasoning","id":"old"},{"type":"function_call","call_id":"a","name":"read","arguments":"{}"}]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ReasoningContentRepairActive: true, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
	got, err := RepairReasoningRequest(body, info)
	require.NoError(t, err)
	assert.Equal(t, int64(2), gjson.GetBytes(got, "input.#").Int())
	assert.JSONEq(t, `{"type":"reasoning","id":"old","summary":[]}`, gjson.GetBytes(got, "input.0").Raw)
	assert.Equal(t, gjson.GetBytes(body, "input.1").Raw, gjson.GetBytes(got, "input.1").Raw)
	assert.False(t, info.ReasoningContentRepairActive, "a previous thinking-enabled attempt must not keep stream repair active")
}

func TestDeepSeekChatFallbackKeepsAnswersToolsAndExistingThoughts(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4.1-flash","tools":[{"type":"function","function":{"name":"read"}}],"messages":[
		{"role":"assistant","content":"original answer","reasoning_content":"original thought"},
		{"role":"user","content":"continue"},
		{"role":"assistant","content":null,"tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"a","content":"real tool result"}
	]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
	got, err := RepairReasoningRequest(body, info)
	require.NoError(t, err)
	for _, index := range []string{"0", "1", "3"} {
		assert.Equal(t, gjson.GetBytes(body, "messages."+index).Raw, gjson.GetBytes(got, "messages."+index).Raw)
	}
	assert.Contains(t, gjson.GetBytes(got, "messages.2.reasoning_content").String(), "历史思考内容缺失")
	assert.Equal(t, gjson.GetBytes(body, "messages.2.tool_calls").Raw, gjson.GetBytes(got, "messages.2.tool_calls").Raw)
}

func TestDeepSeekFallbackChecksWholeTurnAndPreservesOpaqueHistory(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		wantNotice  bool
	}{
		{"late genuine reasoning", `{"type":"function_call","call_id":"a","name":"read","arguments":"{}"},{"type":"reasoning","content":[{"type":"reasoning_text","text":"REAL"}]}`, false},
		{"foreign encrypted history", `{"type":"reasoning","summary":[],"encrypted_content":"OPAQUE","signature":"SIGNED"},{"role":"assistant","content":"original answer"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"deepseek-v4.1-flash","reasoning":{"effort":"high"},"tools":[{"type":"function","name":"read"}],"input":[` + tc.input + `]}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
			got, err := RepairReasoningRequest(body, info)
			require.NoError(t, err)
			if tc.wantNotice {
				assert.Contains(t, gjson.GetBytes(got, "input.0.content.0.text").String(), "历史思考内容缺失")
				assert.Equal(t, gjson.GetBytes(body, "input.0").Raw, gjson.GetBytes(got, "input.1").Raw)
				assert.Equal(t, gjson.GetBytes(body, "input.1").Raw, gjson.GetBytes(got, "input.2").Raw)
			} else {
				assert.Equal(t, body, got)
			}
		})
	}
}

func TestDeepSeekFallbackSkipsOversizedHistoryWithoutPartialPlaceholders(t *testing.T) {
	// 257 separate assistant turns exceed the bounded repair budget. Returning
	// a partially filled history would still fail upstream and grow on replay.
	input := strings.Repeat(`{"role":"assistant","content":"old"},{"role":"user","content":"next"},`, 256) + `{"role":"assistant","content":"last"}`
	body := []byte(`{"model":"deepseek-v4.1-flash","tools":[{"type":"function","name":"read"}],"input":[` + input + `]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
	got, err := RepairReasoningRequest(body, info)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}
