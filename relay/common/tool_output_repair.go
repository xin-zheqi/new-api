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

// RepairToolOutputRequest is an opt-in repair for explicit Responses history.
// It runs after overrides, independently of thinking. Ambiguous or incomplete
// history is left untouched, and nothing is retained across calls. Stateful
// request identifiers do not disable repair for call items supplied locally;
// remote-only history is handled only when the provider names the missing ID.
func RepairToolOutputRequest(data []byte, info *RelayInfo) ([]byte, error) {
	if info == nil {
		return data, nil
	}
	info.ToolOutputRepairStats = nil
	if info.ChannelMeta == nil || !info.ChannelSetting.ToolOutputRepair || info.ChannelSetting.PassThroughBodyEnabled || info.GetFinalRequestRelayFormat() != types.RelayFormatOpenAIResponses {
		return data, nil
	}
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
