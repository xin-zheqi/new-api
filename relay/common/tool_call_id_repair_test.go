package common

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestToolOutputRepairDisambiguatesCompletedRepeatedCalls(t *testing.T) {
	for _, kind := range []string{"function_call", "custom_tool_call"} {
		t.Run(kind, func(t *testing.T) {
			argument := `"arguments":"{\"path\":\"fc_0\"}"`
			if kind == "custom_tool_call" {
				argument = `"input":"fc_0"`
			}
			call := `{"type":"` + kind + `","id":"fc_0","call_id":"fc_0","name":"read",` + argument + `}`
			output := `{"type":"` + kind + `_output","call_id":"fc_0","output":[{"type":"input_text","text":"fc_0"}]}`
			body := []byte(`{"reasoning":{"effort":"high"},"input":[` + call + `,` + output + `,{"type":"reasoning","summary":[{"type":"summary_text","text":"fc_0"}]},` + call + `,` + output + `,` + call + `,` + output + `],"extension":9007199254740993}`)
			original := string(body)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
			got, err := RepairToolOutputRequest(body, info)
			require.NoError(t, err)
			require.True(t, gjson.ValidBytes(got))
			items := gjson.GetBytes(got, "input").Array()
			require.Len(t, items, 7, "real results must not be replaced with missing-output placeholders")
			assert.Equal(t, call, items[0].Raw)
			assert.Equal(t, output, items[1].Raw)
			assert.Equal(t, gjson.GetBytes(body, "input.2").Raw, items[2].Raw, "reasoning must stay verbatim")
			seen := map[string]bool{"fc_0": true}
			for _, index := range []int{3, 5} {
				id := items[index].Get("call_id").String()
				assert.False(t, seen[id], "each completed call needs a distinct identity")
				seen[id] = true
				assert.Equal(t, id, items[index+1].Get("call_id").String())
				assert.NotEqual(t, "fc_0", items[index].Get("id").String())
				assert.Equal(t, items[0].Get("arguments").Raw, items[index].Get("arguments").Raw)
				assert.Equal(t, items[0].Get("input").Raw, items[index].Get("input").Raw)
				assert.Equal(t, items[1].Get("output").Raw, items[index+1].Get("output").Raw)
			}
			assert.Equal(t, "9007199254740993", gjson.GetBytes(got, "extension").Raw)
			assert.Equal(t, original, string(body), "channel retries must retain the original request")
			assert.Equal(t, &ToolOutputRepairStats{RenamedCalls: 2}, info.ToolOutputRepairStats)
			again, err := RepairToolOutputRequest(got, info)
			require.NoError(t, err)
			assert.Equal(t, got, again, "repair must be idempotent")
			assert.Nil(t, info.ToolOutputRepairStats)
		})
	}
}

func TestToolOutputRepairRepeatedParallelBatchesKeepResultAssociations(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"function_call","call_id":"fc_0","id":"item_first","name":"read","arguments":"{}"},
		{"type":"function_call","call_id":"fc_1","name":"read","arguments":"{}"},
		{"type":"function_call_output","call_id":"fc_1","output":"first B"},
		{"type":"function_call_output","call_id":"fc_0","output":"first A"},
		{"role":"user","content":"continue"},
		{"type":"function_call","call_id":"fc_0","id":"item_second","name":"read","arguments":"{}"},
		{"type":"function_call","call_id":"fc_1","name":"read","arguments":"{}"},
		{"type":"function_call_output","call_id":"fc_1","output":"second B"},
		{"type":"function_call_output","call_id":"fc_0","output":"second A"},
		{"type":"function_call","call_id":"call_newapi_2","name":"read","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_newapi_2","output":"reserved later ID"}
	]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ToolOutputRepair: true}}}
	got, err := RepairToolOutputRequest(body, info)
	require.NoError(t, err)
	items := gjson.GetBytes(got, "input").Array()
	require.Len(t, items, 11)
	assert.Equal(t, items[5].Get("call_id").String(), items[8].Get("call_id").String())
	assert.Equal(t, items[6].Get("call_id").String(), items[7].Get("call_id").String())
	assert.NotEqual(t, "call_newapi_2", items[5].Get("call_id").String(), "reserve IDs from later history")
	assert.Equal(t, "item_second", items[5].Get("id").String(), "unique item IDs stay intact")
	assert.Equal(t, "second A", items[8].Get("output").String())
	assert.Equal(t, "second B", items[7].Get("output").String())
	assert.Equal(t, gjson.GetBytes(body, "input.9").Raw, items[9].Raw)
	assert.Equal(t, gjson.GetBytes(body, "input.10").Raw, items[10].Raw)
}

func TestToolOutputRepairDuplicateCallGatesAndBounds(t *testing.T) {
	pair := `{"type":"function_call","call_id":"fc_0","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"fc_0","output":"real"}`
	input := pair + `,` + pair
	for _, tc := range []struct {
		name, input string
		settings    dto.ChannelSettings
		format      types.RelayFormat
	}{
		{"disabled", input, dto.ChannelSettings{}, types.RelayFormatOpenAIResponses},
		{"passthrough", input, dto.ChannelSettings{ToolOutputRepair: true, PassThroughBodyEnabled: true}, types.RelayFormatOpenAIResponses},
		{"converted to chat", input, dto.ChannelSettings{ToolOutputRepair: true}, types.RelayFormatOpenAI},
		{"too many pairs", strings.Repeat(pair+`,`, 256) + pair, dto.ChannelSettings{ToolOutputRepair: true}, types.RelayFormatOpenAIResponses},
		{"too many input items", input + strings.Repeat(`,{"role":"user","content":"padding"}`, 4093), dto.ChannelSettings{ToolOutputRepair: true}, types.RelayFormatOpenAIResponses},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"input":[` + tc.input + `]}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, FinalRequestRelayFormat: tc.format, ChannelMeta: &ChannelMeta{ChannelSetting: tc.settings}, ToolOutputRepairStats: &ToolOutputRepairStats{RenamedCalls: 1}}
			got, err := RepairToolOutputRequest(body, info)
			require.NoError(t, err)
			assert.Equal(t, body, got)
			assert.Nil(t, info.ToolOutputRepairStats)
		})
	}
}

func TestToolOutputRepairKeepsAmbiguousRepeatedCalls(t *testing.T) {
	call := `{"type":"function_call","id":"fc_0","call_id":"fc_0","name":"read","arguments":"{}"}`
	output := `{"type":"function_call_output","call_id":"fc_0","output":"real result"}`
	pair := call + `,` + output
	for _, tc := range []struct{ name, input, extra string }{
		{"parallel duplicate", call + `,` + call + `,` + output + `,` + output, ""},
		{"second output absent", pair + `,` + call, ""},
		{"duplicate output", pair + `,` + output + `,` + pair, ""},
		{"unknown output", pair + `,` + strings.Replace(pair, `"call_id":"fc_0","output"`, `"call_id":"unknown","output"`, 1), ""},
		{"signed call", pair + `,` + strings.Replace(pair, `"name":"read"`, `"name":"read","signature":"signed"`, 1), ""},
		{"encrypted result", pair + `,` + strings.Replace(pair, `"output":"real result"`, `"output":"real result","encrypted_content":"sealed"`, 1), ""},
		{"opaque reasoning", pair + `,{"type":"reasoning","encrypted_content":"sealed"},` + pair, ""},
		{"truncated call", pair + `,` + strings.Replace(pair, `"arguments":"{}"`, `"arguments":"{"`, 1), ""},
		{"unfinished call", pair + `,` + strings.Replace(pair, `"name":"read"`, `"name":"read","status":"in_progress"`, 1), ""},
		{"item reference", pair + `,{"type":"item_reference","id":"item_1"},` + pair, ""},
		{"remote previous history", pair + `,` + pair, `,"previous_response_id":"resp_remote"`},
		{"remote conversation", pair + `,` + pair, `,"conversation":{"id":"conv_remote"}`},
		{"result across user turn", pair + `,` + call + `,{"role":"user","content":"continue"},` + output, ""},
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
