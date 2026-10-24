package common

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestReasoningRepairPreservesWhitespaceAndMultipleHistoryItems(t *testing.T) {
	body := []byte(" \n" + `{"reasoning_effort":"high", "messages":[ {"role":"assistant","reasoning":"one"}, {"role":"assistant","reasoning":"two"}], "unknown": 0 }` + "\n")
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
	got, err := RepairReasoningRequest(body, info)
	require.NoError(t, err)
	require.True(t, gjson.ValidBytes(got))
	assert.Equal(t, "one", gjson.GetBytes(got, "messages.0.reasoning_content").String())
	assert.Equal(t, "two", gjson.GetBytes(got, "messages.1.reasoning_content").String())
	assert.True(t, strings.HasPrefix(string(got), " \n"))
	assert.True(t, strings.HasSuffix(string(got), "\n"))
}

func TestResponsesReasoningRepairDiscardsIncompleteOrOversizedStreams(t *testing.T) {
	for _, tc := range []struct{ name, first, next string }{
		{"byte limit", `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"small"}`, `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"` + strings.Repeat("x", 2<<20) + `"}`},
		{"sequence gap", `{"sequence_number":0,"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"partial"}`, `{"sequence_number":2,"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"tail"}`},
		{"part bound", `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"partial"}`, `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":64,"delta":"tail"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewResponsesReasoningRepair(true)
			for _, data := range []string{tc.first, tc.next, `{"sequence_number":3,"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"r1"}}`} {
				got, err := r.Repair(data)
				require.NoError(t, err)
				assert.Equal(t, data, got)
			}
		})
	}
}

func BenchmarkReasoningRepairRequest(b *testing.B) {
	body := []byte(`{"reasoning_effort":"high","messages":[{"role":"user","content":"` + strings.Repeat("context ", 16384) + `"}]}`)
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled_128KiB_no_repair"
		}
		b.Run(name, func(b *testing.B) {
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: enabled}}}
			b.ReportAllocs()
			for b.Loop() {
				_, err := RepairReasoningRequest(body, info)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestRepairReasoningRequest(t *testing.T) {
	for _, tc := range []struct {
		name, body, path, want string
		format                 types.RelayFormat
		enabled, active        bool
		channel                int
	}{
		{"chat alias", `{"reasoning_effort":"high","messages":[{"role":"assistant","content":"answer","reasoning":"actual thought"}]}`, "messages.0.reasoning_content", "actual thought", types.RelayFormatOpenAI, true, true, 1},
		{"responses alias", `{"reasoning":{"effort":"high"},"input":[{"type":"reasoning","id":"r1","reasoning_content":"actual thought"}]}`, "input.0.content.0.text", "actual thought", types.RelayFormatOpenAIResponses, true, true, 1},
		{"default off", `{"reasoning_effort":"high","messages":[{"role":"assistant","reasoning":"actual thought"}]}`, "messages.0.reasoning_content", "", types.RelayFormatOpenAI, false, false, 1},
		{"explicit off wins", `{"reasoning_effort":"high","thinking":{"type":"disabled"},"messages":[{"role":"assistant","reasoning":"actual thought"}]}`, "messages.0.reasoning_content", "", types.RelayFormatOpenAI, true, false, 43},
		{"unknown default untouched", `{"messages":[{"role":"assistant","reasoning":"actual thought"}]}`, "messages.0.reasoning_content", "", types.RelayFormatOpenAI, true, false, 1},
		{"deepseek v4 default", `{"model":"deepseek-v4.1-flash","input":[{"type":"reasoning","reasoning_text":"actual thought"}]}`, "input.0.content.0.text", "actual thought", types.RelayFormatOpenAIResponses, true, true, constant.ChannelTypeDeepSeek},
		{"existing content kept", `{"reasoning_effort":"high","messages":[{"role":"assistant","reasoning_content":"original","reasoning":"alias"}]}`, "messages.0.reasoning_content", "original", types.RelayFormatOpenAI, true, true, 1},
		{"ordinary answer is not reasoning", `{"reasoning_effort":"high","messages":[{"role":"assistant","content":"answer"}]}`, "messages.0.reasoning_content", "", types.RelayFormatOpenAI, true, true, 1},
		{"summary is not full reasoning", `{"reasoning":{"effort":"high"},"input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"summary"}]}]}`, "input.0.content.0.text", "", types.RelayFormatOpenAIResponses, true, true, 1},
		{"encrypted item untouched", `{"reasoning":{"effort":"high"},"input":[{"type":"reasoning","encrypted_content":"opaque","summary":[],"reasoning_text":"alias"}]}`, "input.0.content.0.text", "", types.RelayFormatOpenAIResponses, true, true, 1},
		{"user text untouched", `{"reasoning_effort":"high","messages":[{"role":"user","reasoning":"not assistant"}]}`, "messages.0.reasoning_content", "", types.RelayFormatOpenAI, true, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &RelayInfo{RelayFormat: tc.format, ChannelMeta: &ChannelMeta{ChannelType: tc.channel, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: tc.enabled}}}
			original := []byte(tc.body)
			got, err := RepairReasoningRequest(original, info)
			require.NoError(t, err)
			assert.Equal(t, tc.want, gjson.GetBytes(got, tc.path).String())
			assert.Equal(t, tc.active, info.ReasoningContentRepairActive)
			assert.Equal(t, tc.body, string(original), "a channel attempt must not mutate the replay body")
			if tc.want == "" || tc.want == "original" {
				assert.Equal(t, tc.body, string(got), "no-op requests retain original bytes")
			}
		})
	}
}

func TestResponsesReasoningRepair(t *testing.T) {
	r := NewResponsesReasoningRepair(true)
	// Interleaved items and multiple content parts must remain separate.
	for _, event := range []string{
		`{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"first "}`,
		`{"type":"response.reasoning_text.delta","item_id":"r2","output_index":1,"content_index":0,"delta":"second"}`,
		`{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"thought"}`,
		`{"type":"response.reasoning_text.done","item_id":"r1","output_index":0,"content_index":0,"text":"first thought"}`,
		`{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":1,"delta":"more"}`,
	} {
		got, err := r.Repair(event)
		require.NoError(t, err)
		assert.Equal(t, event, got)
	}
	got, err := r.Repair(`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"r1","content":null,"summary":[],"vendor":"keep"}}`)
	require.NoError(t, err)
	assert.Equal(t, "first thought", gjson.Get(got, "item.content.0.text").String())
	assert.Equal(t, "more", gjson.Get(got, "item.content.1.text").String())
	assert.Equal(t, "keep", gjson.Get(got, "item.vendor").String())
	got, err = r.Repair(`{"type":"response.completed","response":{"status":"completed","output":[{"type":"reasoning","id":"r1"},{"type":"reasoning","id":"r2"}],"usage":{"input_tokens":11,"output_tokens":22,"output_tokens_details":{"reasoning_tokens":18}}}}`)
	require.NoError(t, err)
	assert.Equal(t, "first thought", gjson.Get(got, "response.output.0.content.0.text").String())
	assert.Equal(t, "second", gjson.Get(got, "response.output.1.content.0.text").String())
	assert.JSONEq(t, `{"input_tokens":11,"output_tokens":22,"output_tokens_details":{"reasoning_tokens":18}}`, gjson.Get(got, "response.usage").Raw)
}

func TestResponsesReasoningRepairLeavesUnsafeItemsUntouched(t *testing.T) {
	for _, tc := range []struct {
		name, delta, final string
		enabled            bool
	}{
		{"disabled", `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"thought"}`, `{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"r1"}}`, false},
		{"summary only", `{"type":"response.reasoning_summary_text.delta","item_id":"r1","output_index":0,"summary_index":0,"delta":"summary"}`, `{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"r1"}}`, true},
		{"wrong item", `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"thought"}`, `{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"r2"}}`, true},
		{"missing first part", `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":1,"delta":"partial"}`, `{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"r1"}}`, true},
		{"encrypted", `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"thought"}`, `{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"r1","encrypted_content":"opaque"}}`, true},
		{"existing content", `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"thought"}`, `{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"r1","content":[{"type":"reasoning_text","text":"original"}]}}`, true},
		{"failed response", `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"partial"}`, `{"type":"response.failed","response":{"output":[{"type":"reasoning","id":"r1"}]}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewResponsesReasoningRepair(tc.enabled)
			_, err := r.Repair(tc.delta)
			require.NoError(t, err)
			got, err := r.Repair(tc.final)
			require.NoError(t, err)
			assert.Equal(t, tc.final, got)
		})
	}
}
