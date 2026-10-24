package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekHistoryRepairsEmptyPlaintextWithoutReplacingRealContent(t *testing.T) {
	for _, tc := range []struct {
		name, item, want string
		unchanged        bool
	}{
		{"empty block", `{"type":"reasoning","content":[{"type":"reasoning_text","text":"","vendor":42}]}`, unavailableReasoningText, false},
		{"blank block", `{"type":"reasoning","content":[{"type":"reasoning_text","text":" \n\t"}]}`, unavailableReasoningText, false},
		{"blank alias", `{"type":"reasoning","summary":[],"reasoning_content":"  "}`, unavailableReasoningText, false},
		{"second real alias", `{"type":"reasoning","summary":[],"reasoning_content":"  ","reasoning_text":" ORIGINAL "}`, " ORIGINAL ", false},
		{"real alias fills block", `{"type":"reasoning","content":[{"type":"reasoning_text","text":"","vendor":42}],"reasoning_content":"ORIGINAL"}`, "ORIGINAL", false},
		{"genuine text", `{"type":"reasoning","content":[{"type":"reasoning_text","text":" ORIGINAL "}]}`, " ORIGINAL ", true},
		{"later genuine part", `{"type":"reasoning","content":[{"type":"reasoning_text","text":""},{"type":"reasoning_text","text":"ORIGINAL"}]}`, "", true},
		{"signed empty part", `{"type":"reasoning","content":[{"type":"reasoning_text","text":"","signature":"SIGNED"}]}`, "", true},
		{"unknown part", `{"type":"reasoning","content":[{"type":"vendor_reasoning","text":""}]}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"deepseek-flash","tools":[{"type":"function","name":"read"}],"input":[` + tc.item + `,{"role":"assistant","content":"REAL ANSWER"}]}`)
			before := string(body)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
			got, err := RepairReasoningRequest(body, info)
			require.NoError(t, err)
			require.True(t, gjson.ValidBytes(got))
			assert.Equal(t, tc.want, gjson.GetBytes(got, "input.0.content.0.text").String())
			if tc.unchanged {
				assert.Equal(t, before, string(got))
			}
			if gjson.Get(tc.item, "content.0.vendor").Exists() {
				assert.Equal(t, int64(42), gjson.GetBytes(got, "input.0.content.0.vendor").Int())
			}
			assert.Equal(t, gjson.Get(before, "input.1").Raw, gjson.GetBytes(got, "input.1").Raw)
			assert.Equal(t, before, string(body), "the retry source must remain unchanged")
			again, err := RepairReasoningRequest(got, info)
			require.NoError(t, err)
			assert.Equal(t, got, again)
		})
	}
}

func TestDeepSeekInterruptedHistoryDoesNotDisableOtherTurns(t *testing.T) {
	for _, tc := range []struct {
		name, history string
		wantFirstFill bool
	}{
		{"interrupted answer", `{"role":"assistant","status":"incomplete","content":"PARTIAL ANSWER"}`, true},
		{"interrupted reasoning", `{"type":"reasoning","status":"incomplete","content":[]},{"role":"assistant","content":"ANSWER"}`, true},
		{"running call", `{"type":"function_call","status":"in_progress","call_id":"a","name":"read","arguments":"{}"}`, false},
		{"truncated call", `{"type":"function_call","status":"incomplete","call_id":"a","name":"read","arguments":"{"}`, false},
		{"malformed call blocks its own group", `{"type":"reasoning","content":[]},{"type":"function_call","call_id":"a","name":"read","arguments":"{"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"deepseek-flash","tools":[{"type":"function","name":"read"}],"input":[` + tc.history + `,{"role":"user","content":"NEW TURN"},{"type":"function_call","call_id":"b","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"b","output":"REAL RESULT"}]}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
			got, err := RepairReasoningRequest(body, info)
			require.NoError(t, err)
			items := gjson.GetBytes(got, "input").Array()
			require.GreaterOrEqual(t, len(items), 5)
			assert.Equal(t, unavailableReasoningText, items[len(items)-3].Get("content.0.text").String())
			assert.Equal(t, tc.wantFirstFill, items[0].Get("content.0.text").String() == unavailableReasoningText)
			// Calls, answers (including status), user messages, and results stay
			// verbatim; only the explicitly missing reasoning may change.
			var before, after []string
			gjson.GetBytes(body, "input").ForEach(func(_, item gjson.Result) bool {
				if item.Get("type").String() != "reasoning" {
					before = append(before, item.Raw)
				}
				return true
			})
			for _, item := range items {
				if item.Get("type").String() != "reasoning" {
					after = append(after, item.Raw)
				}
			}
			assert.Equal(t, before, after)
		})
	}
}

func TestDeepSeekLocalHistoryWithIgnoredStatefulParameters(t *testing.T) {
	for _, channel := range []int{constant.ChannelTypeDeepSeek, constant.ChannelTypeOpenAI} {
		for _, state := range []string{`"previous_response_id":"resp_old",`, `"conversation":{},`} {
			body := []byte(`{"model":"deepseek-flash","reasoning":{"effort":"high"},` + state + `"tools":[{"type":"function","name":"read"}],"input":[{"role":"assistant","content":"OLD ANSWER"}]}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: channel, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: true}}}
			got, err := RepairReasoningRequest(body, info)
			require.NoError(t, err)
			if channel == constant.ChannelTypeDeepSeek {
				assert.Equal(t, unavailableReasoningText, gjson.GetBytes(got, "input.0.content.0.text").String())
				assert.Equal(t, gjson.GetBytes(body, "previous_response_id").Raw, gjson.GetBytes(got, "previous_response_id").Raw)
				assert.Equal(t, gjson.GetBytes(body, "conversation").Raw, gjson.GetBytes(got, "conversation").Raw)
			} else {
				assert.Equal(t, body, got, "compatible gateways may resolve remote history")
			}
		}
	}
}

func TestReasoningStreamRestoresRealTextOverEmptyBlocks(t *testing.T) {
	r := NewResponsesReasoningRepair(true)
	delta := `{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"ORIGINAL STREAM TEXT"}`
	got, err := r.Repair(delta)
	require.NoError(t, err)
	assert.Equal(t, delta, got)
	for _, event := range []string{
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"r1","type":"reasoning","content":[{"type":"reasoning_text","text":""}]}}`,
		`{"type":"response.completed","response":{"status":"completed","output":[{"id":"r1","type":"reasoning","content":[{"type":"reasoning_text","text":"   "}]}],"usage":{"input_tokens":100,"output_tokens":20}}}`,
	} {
		got, err := r.Repair(event)
		require.NoError(t, err)
		assert.Contains(t, got, `"text":"ORIGINAL STREAM TEXT"`)
		assert.NotContains(t, got, unavailableReasoningText)
		assert.Equal(t, gjson.Get(event, "response.usage").Raw, gjson.Get(got, "response.usage").Raw)
	}
}
