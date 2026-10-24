package common

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// These limits bound retained plaintext and bookkeeping per request. On
// overflow we discard the cache and forward original events, never partial
// reasoning. Nothing is stored across requests or in Redis.
const (
	maxReasoningRepairBytes = 2 << 20
	maxReasoningRepairItems = 256
	maxReasoningRepairParts = 64
)

type reasoningTextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type reasoningRepairItem struct {
	id      string
	parts   map[int]*strings.Builder
	content string
	blocked bool
}

type ResponsesReasoningRepair struct {
	items        map[int]*reasoningRepairItem
	used         int
	disabled     bool
	lastSequence int64
	hasSequence  bool
}

// A nil collector is a zero-allocation fast path for the default-off setting.
func NewResponsesReasoningRepair(enabled bool) *ResponsesReasoningRepair {
	if !enabled {
		return nil
	}
	return &ResponsesReasoningRepair{}
}

// Repair patches only terminal reasoning content. Deltas, answers, errors,
// usage, unknown extensions, and events requiring no repair retain their bytes.
// An item is identified by BOTH output index and item ID to prevent reasoning
// from being attached to a different tool turn or output item.
func (r *ResponsesReasoningRepair) Repair(data string) (string, error) {
	if r == nil || r.disabled {
		return data, nil
	}
	root := gjson.Parse(data)
	// A detected gap/duplicate makes accumulated deltas untrustworthy. Providers
	// without sequence numbers still work; the terminal event closes each item.
	if seq := root.Get("sequence_number"); seq.Exists() {
		if seq.Type != gjson.Number || seq.Float() != float64(seq.Int()) || seq.Int() < 0 || (r.hasSequence && seq.Int() != r.lastSequence+1) {
			r.disabled, r.items = true, nil
			return data, nil
		}
		r.lastSequence, r.hasSequence = seq.Int(), true
	} else if r.hasSequence {
		r.disabled, r.items = true, nil
		return data, nil
	}
	switch root.Get("type").String() {
	case "response.output_item.added":
		// Protection may be present only on the opening item. Remember it even
		// if a nonconforming provider omits it from the terminal item.
		index, raw := root.Get("output_index"), root.Get("item")
		if validReasoningIndex(index, maxReasoningRepairItems) && raw.Get("type").String() == "reasoning" && protectedReasoning(raw) {
			if item := r.item(int(index.Int()), raw.Get("id").String()); item != nil {
				item.blocked, item.parts, item.content = true, nil, ""
			}
		}
	case "response.reasoning_text.delta", "response.reasoning_text.done", "response.content_part.done":
		index, partIndex := root.Get("output_index"), root.Get("content_index")
		if !validReasoningIndex(index, maxReasoningRepairItems) || !validReasoningIndex(partIndex, maxReasoningRepairParts) {
			r.disabled, r.items = true, nil
			return data, nil
		}
		kind := root.Get("type").String()
		text := root.Get("delta")
		if kind == "response.reasoning_text.done" {
			text = root.Get("text")
		} else if kind == "response.content_part.done" {
			if root.Get("part.type").String() != "reasoning_text" {
				return data, nil
			}
			text = root.Get("part.text")
		}
		if text.Type != gjson.String {
			return data, nil
		}
		id := root.Get("item_id").String()
		if id == "" || len(id) > 256 {
			r.disabled, r.items = true, nil
			return data, nil
		}
		item := r.item(int(index.Int()), id)
		if item == nil || item.blocked || item.content != "" {
			return data, nil
		}
		if kind == "response.content_part.done" && protectedReasoning(root.Get("part")) {
			item.blocked, item.parts, item.content = true, nil, ""
			return data, nil
		}
		part := item.parts[int(partIndex.Int())]
		if part == nil {
			part = &strings.Builder{}
			item.parts[int(partIndex.Int())] = part
		}
		if kind != "response.reasoning_text.delta" {
			if part.String() == text.String() {
				return data, nil // done often repeats all deltas verbatim.
			}
			part.Reset() // A full done text is authoritative, not another delta.
		}
		if r.reserve(len(text.String())) {
			part.WriteString(text.String())
		}
	case "response.output_item.done":
		index := root.Get("output_index")
		if !validReasoningIndex(index, maxReasoningRepairItems) {
			return data, nil
		}
		content, err := r.finalContent(int(index.Int()), root.Get("item"))
		if err != nil || content == "" || !missingReasoningText(root.Get("item.content")) {
			return data, err
		}
		return sjson.SetRaw(data, "item.content", content)
	case "response.completed", "response.done":
		if status := root.Get("response.status").String(); status != "" && status != "completed" {
			return data, nil
		}
		outputs := root.Get("response.output")
		if !outputs.IsArray() {
			return data, nil
		}
		var patches []jsonTextPatch
		var repairErr error
		outputs.ForEach(func(index, item gjson.Result) bool {
			raw := item.Raw
			content, err := r.finalContent(int(index.Int()), item)
			if err != nil {
				repairErr = err
				return false
			}
			if content != "" && missingReasoningText(item.Get("content")) {
				raw, repairErr = sjson.SetRaw(raw, "content", content)
				if repairErr != nil {
					return false
				}
				patches = append(patches, jsonTextPatch{start: item.Index, end: item.Index + len(item.Raw), value: raw})
			}
			return true
		})
		if repairErr != nil || len(patches) == 0 {
			return data, repairErr
		}
		return applyJSONTextPatches(data, patches), nil
	}
	return data, nil
}

func validReasoningIndex(value gjson.Result, limit int) bool {
	return value.Type == gjson.Number && value.Float() == float64(value.Int()) && value.Int() >= 0 && value.Int() < int64(limit)
}

func (r *ResponsesReasoningRepair) reserve(size int) bool {
	if size > maxReasoningRepairBytes-r.used {
		r.disabled, r.items = true, nil
		return false
	}
	r.used += size
	return true
}

func (r *ResponsesReasoningRepair) item(index int, id string) *reasoningRepairItem {
	if r.disabled || index < 0 || index >= maxReasoningRepairItems || id == "" || len(id) > 256 {
		return nil
	}
	if r.items == nil {
		r.items = make(map[int]*reasoningRepairItem)
	}
	item := r.items[index]
	if item == nil {
		// gjson strings can point into the entire SSE frame; retain only the ID.
		item = &reasoningRepairItem{id: strings.Clone(id), parts: make(map[int]*strings.Builder)}
		r.items[index] = item
	} else if item.id != id {
		item.blocked, item.parts, item.content = true, nil, ""
	}
	return item
}

// A done item/response is the completion boundary for collected text. Missing
// part indexes or protected content cannot be reconstructed safely.
func (r *ResponsesReasoningRepair) finalContent(index int, raw gjson.Result) (string, error) {
	if raw.Get("type").String() != "reasoning" {
		return "", nil
	}
	item := r.item(index, raw.Get("id").String())
	if item == nil || item.blocked {
		return "", nil
	}
	if protectedReasoning(raw) {
		item.blocked, item.parts, item.content = true, nil, ""
		return "", nil
	}
	if status := raw.Get("status").String(); status != "" && status != "completed" {
		item.blocked, item.parts, item.content = true, nil, ""
		return "", nil
	}
	if content := raw.Get("content"); !missingReasoningText(content) {
		plain := content.IsArray()
		content.ForEach(func(_, part gjson.Result) bool {
			plain = plain && part.Get("type").String() == "reasoning_text" && part.Get("text").Type == gjson.String && !protectedReasoning(part)
			return plain
		})
		if !plain {
			item.blocked, item.parts, item.content = true, nil, ""
			return "", nil
		}
		if item.content != content.Raw && r.reserve(len(content.Raw)) {
			item.content, item.parts = strings.Clone(content.Raw), nil
		}
		return "", nil
	}
	if item.content != "" {
		return item.content, nil
	}
	if len(item.parts) == 0 {
		return "", nil
	}
	parts := make([]reasoningTextPart, len(item.parts))
	for i := range parts {
		part := item.parts[i]
		if part == nil || strings.TrimSpace(part.String()) == "" {
			return "", nil
		}
		parts[i] = reasoningTextPart{Type: "reasoning_text", Text: part.String()}
	}
	encoded, err := common.Marshal(parts)
	if err != nil || !r.reserve(len(encoded)) {
		return "", err
	}
	item.content, item.parts = string(encoded), nil
	return item.content, nil
}
