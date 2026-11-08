package common

import (
	"regexp"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var missingToolOutputErrorPattern = regexp.MustCompile(`(?i)no\s+tool\s+output\s+found\s+for\s+tool\s+call\s+["']?([A-Za-z0-9][A-Za-z0-9._:-]{0,255})`)

// This is a gateway diagnostic, not a tool execution result. The execution may
// already have had side effects; do not encourage the model to repeat it.
const unavailableToolOutput = `{"error":{"code":"tool_output_unavailable","message":"New API could not find this tool's result in the supplied history. Execution status is unknown. Do not assume success or automatically repeat side-effecting actions; ask the user if needed."}}`

type ToolOutputRepairStats struct {
	MissingOutputs  int `json:"missing_outputs"`
	RelinkedOutputs int `json:"relinked_outputs"`
	RenamedCalls    int `json:"renamed_calls,omitempty"`
}

type toolHistoryCall struct {
	id, outputType string
	group          int
	hasOutput      bool
}

type toolHistoryGroup struct {
	calls     []*toolHistoryCall
	end       int
	sawOutput bool
}

type toolHistoryOutput struct {
	item  gjson.Result
	group int
}

// RepairToolOutputRequest is an opt-in repair for explicit Chat Completions
// or Responses history. It runs after overrides, independently of thinking.
// Ambiguous or incomplete history is left untouched, and nothing is retained
// across calls. Stateful Responses identifiers do not disable repair for call
// items supplied locally; remote-only history is handled only when the provider
// names the missing ID.
func RepairToolOutputRequest(data []byte, info *RelayInfo) ([]byte, error) {
	if info == nil {
		return data, nil
	}
	info.ToolOutputRepairStats = nil
	if info.ChannelMeta == nil || !info.ChannelSetting.ToolOutputRepair || info.ChannelSetting.PassThroughBodyEnabled {
		return data, nil
	}
	switch info.GetFinalRequestRelayFormat() {
	case types.RelayFormatOpenAIResponses:
		return repairResponsesToolOutputRequest(data, info)
	case types.RelayFormatOpenAI:
		return repairChatToolOutputRequest(data, info)
	default:
		return data, nil
	}
}

func repairResponsesToolOutputRequest(data []byte, info *RelayInfo) ([]byte, error) {
	root := gjson.ParseBytes(data)
	// A stateful identifier may point to remote history, but explicit call items
	// in this request are still locally inspectable. The scanner below only
	// repairs those items and remains a no-op when the request contains no local
	// call, so compatible gateways never receive invented remote history.
	items := root.Get("input")
	if !items.IsArray() {
		return data, nil
	}
	var groups []toolHistoryGroup
	var outputs []toolHistoryOutput
	var calls, aliases map[string]*toolHistoryCall
	current, count, valid := -1, 0, true
	duplicateCall := false
	items.ForEach(func(_, item gjson.Result) bool {
		count++
		if count > 4096 || !item.IsObject() {
			valid = false
			return false
		}
		switch kind := item.Get("type").String(); kind {
		case "item_reference":
			valid = false
			return false
		case "function_call", "custom_tool_call":
			id, status := item.Get("call_id"), item.Get("status").String()
			duplicateCall = id.Type == gjson.String && calls[id.String()] != nil
			if id.Type != gjson.String || id.String() == "" || len(id.String()) > 256 || len(calls) >= 256 || calls[id.String()] != nil || (status != "" && status != "completed") {
				valid = false
				return false
			}
			// A truncated call is not evidence of a tool awaiting its result.
			name, arguments := item.Get("name"), item.Get("arguments")
			invalidArguments := kind == "function_call" && (arguments.Type != gjson.String || !gjson.Valid(arguments.String()))
			invalidCustomInput := kind == "custom_tool_call" && item.Get("input").Type != gjson.String
			if name.Type != gjson.String || name.String() == "" || invalidArguments || invalidCustomInput {
				valid = false
				return false
			}
			if current < 0 || groups[current].sawOutput {
				groups = append(groups, toolHistoryGroup{})
				current = len(groups) - 1
			}
			call := &toolHistoryCall{id: id.String(), outputType: kind + "_output", group: current}
			if calls == nil {
				calls, aliases = make(map[string]*toolHistoryCall), make(map[string]*toolHistoryCall)
			}
			calls[call.id] = call
			if alias := item.Get("id"); alias.Exists() {
				if alias.Type != gjson.String || alias.String() == "" || len(alias.String()) > 256 || aliases[alias.String()] != nil {
					valid = false
					return false
				}
				aliases[alias.String()] = call
			}
			groups[current].calls = append(groups[current].calls, call)
			groups[current].end = item.Index + len(item.Raw)
		case "function_call_output", "custom_tool_call_output":
			if current < 0 || len(outputs) >= 256 {
				valid = false
				return false
			}
			outputs = append(outputs, toolHistoryOutput{item: item, group: current})
			groups[current].sawOutput = true
			groups[current].end = item.Index + len(item.Raw)
		case "reasoning":
			// Keep reasoning byte-for-byte; it may accompany a parallel call batch.
		default:
			current = -1
		}
		return true
	})
	if !valid || len(calls) == 0 {
		if duplicateCall {
			return repairDuplicateToolCallIDs(data, info)
		}
		return data, nil
	}
	for alias, call := range aliases {
		if other := calls[alias]; other != nil && other != call {
			return data, nil // An item ID also names another call: never guess.
		}
	}
	var patches []jsonTextPatch
	stats := ToolOutputRepairStats{}
	for _, output := range outputs {
		item := output.item
		id := item.Get("call_id")
		if id.Exists() && id.Type != gjson.String {
			return data, nil // Do not coerce malformed IDs into another call's ID.
		}
		if id.String() == "" {
			id = item.Get("tool_call_id")
		}
		if id.Type != gjson.String || id.String() == "" {
			return data, nil
		}
		call := calls[id.String()]
		if call == nil {
			call = aliases[id.String()]
		}
		result := item.Get("output")
		if call == nil || call.hasOutput || call.group != output.group || call.outputType != item.Get("type").String() || (result.Type != gjson.String && !result.IsArray()) {
			return data, nil // Orphaned/duplicate/malformed results remain visible errors.
		}
		call.hasOutput = true
		if item.Get("call_id").String() != call.id {
			if protectedReasoning(item) {
				return data, nil // Do not rewrite a signed/encrypted item identity.
			}
			raw, err := sjson.Set(item.Raw, "call_id", call.id)
			if err != nil {
				return data, err
			}
			patches = append(patches, jsonTextPatch{start: item.Index, end: item.Index + len(item.Raw), value: raw})
			stats.RelinkedOutputs++
		}
	}
	for _, group := range groups {
		var missing strings.Builder
		for _, call := range group.calls {
			if call.hasOutput {
				continue
			}
			encoded, err := common.Marshal(struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Output string `json:"output"`
			}{call.outputType, call.id, unavailableToolOutput})
			if err != nil {
				return data, err
			}
			missing.WriteByte(',')
			missing.Write(encoded)
			stats.MissingOutputs++
		}
		if missing.Len() > 0 {
			// Append after this batch's calls and real results, before the next
			// conversation turn. Original items never move or get overwritten.
			patches = append(patches, jsonTextPatch{start: group.end, end: group.end, value: missing.String()})
		}
	}
	if len(patches) == 0 {
		return data, nil
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	info.ToolOutputRepairStats = &stats
	return []byte(applyJSONTextPatches(string(data), patches)), nil
}

type chatToolCall struct {
	id     string
	output bool
}

type chatToolGroup struct {
	calls []*chatToolCall
	byID  map[string]*chatToolCall
	end   int
}

type chatToolMessage struct {
	Role       string `json:"role"`
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// repairChatToolOutputRequest closes missing Chat Completions tool-result
// blocks. OpenAI-compatible servers require every assistant tool_calls entry to
// be answered before the next non-tool message. Existing results and unrelated
// fields are preserved; unsafe or ambiguous history is left untouched.
func repairChatToolOutputRequest(data []byte, info *RelayInfo) ([]byte, error) {
	root := gjson.ParseBytes(data)
	messages := root.Get("messages")
	if !messages.IsArray() {
		return data, nil
	}
	var groups []*chatToolGroup
	var current *chatToolGroup
	var patches []jsonTextPatch
	stats := ToolOutputRepairStats{}
	count, callCount, outputs := 0, 0, 0
	valid := true
	var repairErr error
	messages.ForEach(func(_, message gjson.Result) bool {
		count++
		if count > 4096 || !message.IsObject() {
			valid = false
			return false
		}
		role := message.Get("role").String()
		switch role {
		case "assistant":
			calls := message.Get("tool_calls")
			if !calls.Exists() || calls.Type == gjson.Null {
				current = nil
				return true
			}
			if !calls.IsArray() {
				valid = false
				return false
			}
			group := &chatToolGroup{byID: make(map[string]*chatToolCall), end: message.Index + len(message.Raw)}
			calls.ForEach(func(_, rawCall gjson.Result) bool {
				id := rawCall.Get("id")
				if !rawCall.IsObject() || id.Type != gjson.String || id.String() == "" || len(id.String()) > 256 || callCount >= 256 || group.byID[id.String()] != nil {
					valid = false
					return false
				}
				call := &chatToolCall{id: id.String()}
				group.calls = append(group.calls, call)
				group.byID[call.id] = call
				callCount++
				return true
			})
			if !valid {
				return false
			}
			if len(group.calls) == 0 {
				current = nil
				return true
			}
			groups = append(groups, group)
			current = group
		case "tool":
			id := message.Get("tool_call_id")
			if current == nil || id.Type != gjson.String || id.String() == "" {
				valid = false
				return false
			}
			call := current.byID[id.String()]
			outputs++
			if call == nil || call.output || outputs > 256 {
				valid = false
				return false
			}
			content := message.Get("content")
			if content.Type != gjson.String && !content.IsArray() {
				encoded, err := common.Marshal(unavailableToolOutput)
				if err != nil {
					repairErr = err
					valid = false
					return false
				}
				raw, err := sjson.SetRaw(message.Raw, "content", string(encoded))
				if err != nil {
					repairErr = err
					valid = false
					return false
				}
				patches = append(patches, jsonTextPatch{start: message.Index, end: message.Index + len(message.Raw), value: raw})
				stats.MissingOutputs++
			}
			call.output = true
			current.end = message.Index + len(message.Raw)
		default:
			current = nil
		}
		return true
	})
	if repairErr != nil {
		return data, repairErr
	}
	if !valid || callCount == 0 {
		return data, nil
	}
	for _, group := range groups {
		var missing strings.Builder
		for _, call := range group.calls {
			if call.output {
				continue
			}
			encoded, err := common.Marshal(chatToolMessage{Role: "tool", ToolCallID: call.id, Content: unavailableToolOutput})
			if err != nil {
				return data, err
			}
			if missing.Len() > 0 {
				missing.WriteByte(',')
			}
			missing.Write(encoded)
			stats.MissingOutputs++
		}
		if missing.Len() > 0 {
			patches = append(patches, jsonTextPatch{start: group.end, end: group.end, value: "," + missing.String()})
		}
	}
	if len(patches) == 0 {
		return data, nil
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	info.ToolOutputRepairStats = &stats
	return []byte(applyJSONTextPatches(string(data), patches)), nil
}

// RepairToolOutputFromUpstreamError handles the narrow case where a provider
// reports a missing output for a stateful call whose call item is not present
// in the current request. The provider supplies the exact call ID, so adding
// an explicit "execution status unknown" output is safer than retrying the
// same invalid request. It returns the original body and zero when the error
// is unrelated or the input cannot be safely extended.
func RepairToolOutputFromUpstreamError(data, upstreamError []byte, info *RelayInfo) ([]byte, int, error) {
	if info == nil || info.ChannelMeta == nil || !info.ChannelSetting.ToolOutputRepair ||
		info.ChannelSetting.PassThroughBodyEnabled || info.GetFinalRequestRelayFormat() != types.RelayFormatOpenAIResponses {
		return data, 0, nil
	}
	matches := missingToolOutputErrorPattern.FindAllSubmatch(upstreamError, -1)
	if len(matches) == 0 {
		return data, 0, nil
	}
	root := gjson.ParseBytes(data)
	items := root.Get("input")
	if !items.IsArray() || len(items.Array()) > 4096 {
		return data, 0, nil
	}
	knownOutputs := make(map[string]struct{})
	invalidOutputs := make(map[string]gjson.Result)
	callTypes := make(map[string]string)
	items.ForEach(func(_, item gjson.Result) bool {
		switch item.Get("type").String() {
		case "function_call", "custom_tool_call":
			if id := item.Get("call_id").String(); id != "" {
				callTypes[id] = item.Get("type").String() + "_output"
			}
		case "function_call_output", "custom_tool_call_output":
			if id := item.Get("call_id").String(); id != "" {
				result := item.Get("output")
				if result.Type == gjson.String || result.IsArray() {
					knownOutputs[id] = struct{}{}
				} else {
					invalidOutputs[id] = item
				}
			}
		}
		return true
	})

	var appendItems strings.Builder
	var patches []jsonTextPatch
	added := 0
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		id := string(match[1])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := knownOutputs[id]; ok {
			continue
		}
		if invalid := invalidOutputs[id]; invalid.Exists() {
			encodedOutput, err := common.Marshal(unavailableToolOutput)
			if err != nil {
				return data, 0, err
			}
			raw, err := sjson.SetRaw(invalid.Raw, "output", string(encodedOutput))
			if err != nil {
				return data, 0, err
			}
			patches = append(patches, jsonTextPatch{start: invalid.Index, end: invalid.Index + len(invalid.Raw), value: raw})
			added++
			continue
		}
		outputType := callTypes[id]
		if outputType == "" {
			outputType = "function_call_output"
		}
		encoded, err := common.Marshal(struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
			Output string `json:"output"`
		}{outputType, id, unavailableToolOutput})
		if err != nil {
			return data, 0, err
		}
		if appendItems.Len() > 0 {
			appendItems.WriteByte(',')
		}
		appendItems.Write(encoded)
		added++
	}
	if added == 0 {
		return data, 0, nil
	}
	// Insert before the closing bracket so existing request items and unknown
	// provider fields stay byte-for-byte unchanged.
	insertAt := items.Index + len(items.Raw) - 1
	if appendItems.Len() > 0 {
		value := "," + appendItems.String()
		if len(items.Array()) == 0 {
			value = appendItems.String()
		}
		patches = append(patches, jsonTextPatch{
			start: insertAt,
			end:   insertAt,
			value: value,
		})
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	stats := info.ToolOutputRepairStats
	if stats == nil {
		stats = &ToolOutputRepairStats{}
	}
	stats.MissingOutputs += added
	info.ToolOutputRepairStats = stats
	return []byte(applyJSONTextPatches(string(data), patches)), added, nil
}
