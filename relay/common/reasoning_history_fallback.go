package common

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/tidwall/gjson"
)

// This is a compatibility notice, not reconstructed model reasoning. Never
// derive a substitute from user messages, answers, summaries, or tool results.
const unavailableReasoningText = "历史思考内容缺失，Agent或请求链路中可能丢弃历史思考内容。"

type reasoningHistoryTurn struct {
	start        int
	emptyItem    gjson.Result
	hasReasoning bool
	hasAssistant bool
	blocked      bool
}

// DeepSeek requires prior plaintext reasoning when thinking and tools are both
// enabled. The caller has checked those conditions on the final upstream body.
// Only explicit local history is repairable; no cache or extra request is used.
func repairDeepSeekReasoningHistory(data []byte, root gjson.Result, format types.RelayFormat) ([]byte, error) {
	path := "input"
	if format == types.RelayFormatOpenAI {
		path = "messages"
	}
	var turns []reasoningHistoryTurn
	current, count, valid := -1, 0, true
	root.Get(path).ForEach(func(_, item gjson.Result) bool {
		count++
		if count > 4096 || !item.IsObject() || item.Get("type").String() == "item_reference" {
			valid = false
			return false
		}
		kind, role := item.Get("type").String(), item.Get("role").String()
		assistant := role == "assistant" && (kind == "" || kind == "message")
		call := kind == "function_call" || kind == "custom_tool_call"
		reasoning := kind == "reasoning" && format == types.RelayFormatOpenAIResponses
		if !assistant && !call && !reasoning {
			current = -1 // User/system messages and tool results end this turn.
			return true
		}
		// Chat attaches reasoning to each assistant message; Responses uses a
		// separate item shared by the contiguous assistant/parallel-call batch.
		if current < 0 || format == types.RelayFormatOpenAI {
			if len(turns) >= maxReasoningRepairItems {
				valid = false
				return false
			}
			turns = append(turns, reasoningHistoryTurn{start: item.Index})
			current = len(turns) - 1
		}
		turn := &turns[current]
		// Interrupted answers/reasoning can legitimately remain in history.
		// A missing-content notice does not change their status or pretend to
		// restore truncated text. Running/invalid calls stay untouched, without
		// preventing repairs in other, independent history groups.
		status := item.Get("status").String()
		if status != "" && status != "completed" && (call || status != "incomplete") {
			turn.blocked = true
		}
		if call {
			arguments := item.Get("arguments")
			if item.Get("call_id").String() == "" || item.Get("name").String() == "" ||
				(kind == "function_call" && (arguments.Type != gjson.String || !gjson.Valid(arguments.String()))) ||
				(kind == "custom_tool_call" && item.Get("input").Type != gjson.String) {
				turn.blocked = true
			}
		}
		turn.hasAssistant = turn.hasAssistant || assistant || call
		field := "reasoning_content"
		if reasoning {
			field = "content"
		}
		if reasoning || format == types.RelayFormatOpenAI && assistant {
			// Even an oversized or signed genuine alias must not be replaced by
			// a notice just because the earlier normalization skipped it.
			if !missingReasoningText(item.Get(field)) || !missingReasoningText(item.Get("reasoning_content")) || !missingReasoningText(item.Get("reasoning_text")) || !missingReasoningText(item.Get("reasoning")) {
				turn.hasReasoning = true
			} else if !turn.emptyItem.Exists() && !protectedReasoning(item) {
				turn.emptyItem = item
			} else if format == types.RelayFormatOpenAI && protectedReasoning(item) {
				turn.hasReasoning = true // Chat has no separate unsigned item.
			}
		}
		return true
	})
	if !valid {
		return data, nil
	}
	var patches []jsonTextPatch
	var encoded []byte
	for _, turn := range turns {
		if !turn.hasAssistant || turn.hasReasoning || turn.blocked {
			continue
		}
		if item := turn.emptyItem; item.Exists() {
			field := "content"
			if format == types.RelayFormatOpenAI {
				field = "reasoning_content"
			}
			raw, err := setReasoningPlaintext(item, field, unavailableReasoningText)
			if err != nil {
				return data, err
			}
			patches = append(patches, jsonTextPatch{start: item.Index, end: item.Index + len(item.Raw), value: raw})
			continue
		}
		if encoded == nil {
			var err error
			encoded, err = common.Marshal([]reasoningTextPart{{Type: "reasoning_text", Text: unavailableReasoningText}})
			if err != nil {
				return data, err
			}
		}
		// No reasoning item survived (e.g. old non-thinking history). Insert
		// one before this entire batch. No fabricated ID or ciphertext needed.
		raw := `{"type":"reasoning","summary":[],"content":` + string(encoded) + `},`
		patches = append(patches, jsonTextPatch{start: turn.start, end: turn.start, value: raw})
	}
	if len(patches) == 0 {
		return data, nil
	}
	return []byte(applyJSONTextPatches(string(data), patches)), nil
}
