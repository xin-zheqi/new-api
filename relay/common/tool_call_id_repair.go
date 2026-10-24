package common

import (
	"sort"
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type toolCallIDPair struct {
	call, output gjson.Result
	renameItem   bool
	duplicate    bool
}

// repairDuplicateToolCallIDs handles reused IDs in fully paired, local history.
// It is reached only after the normal scanner encounters a duplicate, so valid
// requests pay no additional scan/allocation cost. Each completed batch must be
// closed before another begins; overlapping calls or uncertain result links are
// not safe to rename. Stateful history may reference IDs outside this request.
func repairDuplicateToolCallIDs(data []byte, info *RelayInfo) ([]byte, error) {
	root := gjson.ParseBytes(data)
	for _, key := range []string{"previous_response_id", "conversation"} {
		if value := root.Get(key); value.Type != gjson.Null && value.String() != "" {
			return data, nil
		}
	}
	var pairs []*toolCallIDPair
	pending := make(map[string]*toolCallIDPair)
	seenCalls := make(map[string]bool)
	itemOwners := make(map[string]string)
	usedIDs := make(map[string]bool)
	count, outputs, duplicates := 0, 0, 0
	valid, sawOutput := true, false
	root.Get("input").ForEach(func(_, item gjson.Result) bool {
		count++
		if count > 4096 || !item.IsObject() {
			valid = false
			return false
		}
		kind := item.Get("type").String()
		isCall := kind == "function_call" || kind == "custom_tool_call"
		id := item.Get("call_id").String()
		itemID := item.Get("id")
		renameItem := false
		if itemID.Exists() {
			if itemID.Type != gjson.String || itemID.String() == "" || len(itemID.String()) > 256 {
				valid = false
				return false
			}
			owner, exists := itemOwners[itemID.String()]
			if exists && (!isCall || owner == "" || owner != id) {
				valid = false // An item ID is shared by unrelated history entries.
				return false
			}
			renameItem = exists
			owner = ""
			if isCall {
				owner = id
			}
			itemOwners[itemID.String()] = owner
			usedIDs[itemID.String()] = true
		}
		switch kind {
		case "function_call", "custom_tool_call":
			status := item.Get("status").String()
			arguments := item.Get("arguments")
			if len(pairs) >= 256 || item.Get("call_id").Type != gjson.String || id == "" || len(id) > 256 ||
				pending[id] != nil || (sawOutput && len(pending) > 0) || (status != "" && status != "completed") ||
				item.Get("name").Type != gjson.String || item.Get("name").String() == "" ||
				(kind == "function_call" && (arguments.Type != gjson.String || !gjson.Valid(arguments.String()))) ||
				(kind == "custom_tool_call" && item.Get("input").Type != gjson.String) {
				valid = false
				return false
			}
			pair := &toolCallIDPair{call: item, duplicate: seenCalls[id], renameItem: renameItem}
			if pair.duplicate {
				duplicates++
			}
			pairs = append(pairs, pair)
			pending[id] = pair
			seenCalls[id], usedIDs[id], sawOutput = true, true, false
		case "function_call_output", "custom_tool_call_output":
			outputs++
			pair := pending[id]
			result := item.Get("output")
			if outputs > 256 || pair == nil || item.Get("call_id").Type != gjson.String ||
				kind != pair.call.Get("type").String()+"_output" || (result.Type != gjson.String && !result.IsArray()) {
				valid = false
				return false
			}
			pair.output = item
			delete(pending, id)
			sawOutput = true
		case "reasoning":
			// Opaque reasoning may bind tool identities we cannot inspect.
			if protectedReasoning(item) {
				valid = false
				return false
			}
		case "message", "":
			role := item.Get("role").String()
			if len(pending) != 0 || (role != "user" && role != "assistant" && role != "system" && role != "developer") {
				valid = false
				return false
			}
		default:
			valid = false // Includes remote item references and hosted-tool history.
			return false
		}
		return true
	})
	if !valid || len(pending) != 0 || duplicates == 0 {
		return data, nil
	}
	for itemID, owner := range itemOwners {
		if seenCalls[itemID] && itemID != owner {
			return data, nil // Never guess between a call ID and another item's ID.
		}
	}
	patches := make([]jsonTextPatch, 0, duplicates*2)
	for index, pair := range pairs {
		if !pair.duplicate {
			continue
		}
		if protectedReasoning(pair.call) || protectedReasoning(pair.output) {
			return data, nil // Signed/encrypted identities must remain intact.
		}
		// Deterministic IDs keep retries stable; reserve all existing IDs first,
		// including later history, so the repair cannot introduce a new collision.
		suffix := strconv.Itoa(index)
		for attempt := 0; usedIDs["call_newapi_"+suffix] || usedIDs["fc_newapi_"+suffix]; attempt++ {
			suffix = strconv.Itoa(index) + "_" + strconv.Itoa(attempt)
		}
		callID := "call_newapi_" + suffix
		usedIDs[callID], usedIDs["fc_newapi_"+suffix] = true, true
		call, err := sjson.Set(pair.call.Raw, "call_id", callID)
		if err != nil {
			return data, err
		}
		if pair.renameItem {
			call, err = sjson.Set(call, "id", "fc_newapi_"+suffix)
			if err != nil {
				return data, err
			}
		}
		output, err := sjson.Set(pair.output.Raw, "call_id", callID)
		if err != nil {
			return data, err
		}
		patches = append(patches,
			jsonTextPatch{start: pair.call.Index, end: pair.call.Index + len(pair.call.Raw), value: call},
			jsonTextPatch{start: pair.output.Index, end: pair.output.Index + len(pair.output.Raw), value: output})
	}
	// Parallel batches may return results in a different order to their calls.
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	info.ToolOutputRepairStats = &ToolOutputRepairStats{RenamedCalls: duplicates}
	return []byte(applyJSONTextPatches(string(data), patches)), nil
}
