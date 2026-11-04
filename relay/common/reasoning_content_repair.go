package common

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// RepairReasoningRequest preserves genuine reasoning and repairs replayed
// history on opt-in channels. DeepSeek's missing-history fallback is an explicit
// diagnostic, never text borrowed from answers, tools, or another turn.
// Raw patches preserve unknown fields and the original body used by retries.
func RepairReasoningRequest(data []byte, info *RelayInfo) ([]byte, error) {
	if info == nil {
		return data, nil
	}
	info.ReasoningContentRepairActive = false
	if info.ChannelMeta == nil || !info.ChannelSetting.ReasoningContentRepair || info.ChannelSetting.PassThroughBodyEnabled {
		return data, nil
	}
	format := info.GetFinalRequestRelayFormat()
	if format != types.RelayFormatOpenAI && format != types.RelayFormatOpenAIResponses {
		return data, nil // Native Claude/Gemini signatures must remain untouched.
	}
	root := gjson.ParseBytes(data)
	thinking := reasoningRepairEnabled(root)
	if !thinking && format != types.RelayFormatOpenAIResponses {
		return data, nil
	}
	info.ReasoningContentRepairActive = thinking
	path := "messages"
	if format == types.RelayFormatOpenAIResponses {
		path = "input"
	}
	items := root.Get(path)
	if !items.IsArray() {
		return data, nil
	}
	var patches []jsonTextPatch
	used, count := 0, 0
	var repairErr error
	items.ForEach(func(_, item gjson.Result) bool {
		count++
		if count > 4096 || len(patches) >= maxReasoningRepairItems {
			return false
		}
		field := "reasoning_content"
		eligible := item.Get("role").String() == "assistant"
		if format == types.RelayFormatOpenAIResponses {
			field, eligible = "content", item.Get("type").String() == "reasoning"
		}
		if !eligible {
			return true
		}
		raw := item.Raw
		alias := item.Get("reasoning")
		if format == types.RelayFormatOpenAIResponses {
			alias = item.Get("reasoning_content")
			if alias.Type != gjson.String || strings.TrimSpace(alias.String()) == "" {
				alias = item.Get("reasoning_text")
			}
		}
		if thinking && !protectedReasoning(item) && missingReasoningText(item.Get(field)) && alias.Type == gjson.String && strings.TrimSpace(alias.String()) != "" && len(alias.String()) <= maxReasoningRepairBytes-used {
			used += len(alias.String())
			raw, repairErr = setReasoningPlaintext(item, field, alias.String())
		} else if format == types.RelayFormatOpenAIResponses && !item.Get("summary").IsArray() && !item.Get("content").IsArray() && missingReasoning(item.Get("summary")) && missingReasoning(item.Get("content")) {
			// Replayed Responses items require summary or content even when the
			// next turn disables thinking. An empty summary changes no thought
			// or ciphertext, and avoids sending an invalid bare reasoning item.
			raw, repairErr = sjson.SetRaw(raw, "summary", "[]")
		}
		if repairErr != nil {
			return false
		}
		if raw != item.Raw {
			patches = append(patches, jsonTextPatch{start: item.Index, end: item.Index + len(item.Raw), value: raw})
		}
		return true
	})
	if repairErr != nil {
		return data, repairErr
	}
	if len(patches) > 0 {
		data = []byte(applyJSONTextPatches(string(data), patches))
		root = gjson.ParseBytes(data)
	}
	model := strings.ToLower(root.Get("model").String())
	deepSeek := info.ChannelType == constant.ChannelTypeDeepSeek || strings.HasPrefix(model, "deepseek-") || strings.HasPrefix(model, "deepseek/") || strings.HasPrefix(model, "deepseek-ai/deepseek-")
	if thinking && deepSeek {
		// A Responses client may omit the current tools declaration while still
		// replaying assistant/tool history (for example after a ZCode/Sub2API
		// conversion). DeepSeek validates the replayed reasoning independently of
		// that declaration, so let the history scanner decide whether a local
		// batch actually needs a placeholder. The scanner is a no-op for a new
		// user-only request and never invents a turn on its own.
		// DeepSeek's native Responses endpoint ignores stateful parameters.
		// Repair explicit local history even if a client also sent those fields;
		// compatible gateways may implement them, so retain their original guard.
		stateful := root.Get("previous_response_id").String() != "" || root.Get("conversation").Type != gjson.Null
		if !stateful || info.ChannelType == constant.ChannelTypeDeepSeek {
			return repairDeepSeekReasoningHistory(data, root, format)
		}
	}
	return data, nil
}

// Explicit disabling always wins, including overrides applied by a channel.
// Thinking is enabled only by an embedded request option. Model names are
// deliberately excluded here so an OpenAI-compatible gateway cannot be
// treated as a thinking model merely because of its model label.
func reasoningRepairEnabled(root gjson.Result) bool {
	enabled := false
	for _, path := range []string{"reasoning_effort", "reasoning.effort", "thinking.type", "reasoning.enabled", "enable_thinking"} {
		value := root.Get(path)
		if value.Type == gjson.False {
			return false
		}
		if value.Type == gjson.True {
			enabled = true
		}
		if value.Type != gjson.String {
			continue
		}
		switch value.String() {
		case "none", "disabled":
			return false
		case "enabled", "adaptive", "minimal", "low", "medium", "high", "xhigh", "max":
			enabled = true
		}
	}
	return enabled
}

func missingReasoning(value gjson.Result) bool {
	return !value.Exists() || value.Type == gjson.Null || value.Type == gjson.String && value.String() == "" || value.IsArray() && value.Get("#").Int() == 0
}

// A populated content array can still contain no plaintext. Only recognize
// empty, unsigned reasoning_text parts: unknown/signed content must survive.
// Keep this separate from missingReasoning, which also protects opaque fields.
func missingReasoningText(value gjson.Result) bool {
	if missingReasoning(value) {
		return true
	}
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String()) == ""
	}
	if !value.IsArray() {
		return false
	}
	empty, count := true, 0
	value.ForEach(func(_, part gjson.Result) bool {
		count++
		text := part.Get("text")
		empty = count <= maxReasoningRepairParts && part.Get("type").String() == "reasoning_text" &&
			!protectedReasoning(part) && (text.Type == gjson.Null || text.Type == gjson.String && strings.TrimSpace(text.String()) == "")
		return empty
	})
	return empty
}

// Preserve metadata and empty companion parts when filling an existing block.
// The caller has already checked that no genuine/signed plaintext is replaced.
func setReasoningPlaintext(item gjson.Result, field, text string) (string, error) {
	var value any = text
	path := field
	if field == "content" {
		if item.Get("content").IsArray() && item.Get("content.0").Exists() {
			path = "content.0.text"
		} else {
			value = []reasoningTextPart{{Type: "reasoning_text", Text: text}}
		}
	}
	encoded, err := common.Marshal(value)
	if err != nil {
		return item.Raw, err
	}
	return sjson.SetRaw(item.Raw, path, string(encoded))
}

func protectedReasoning(item gjson.Result) bool {
	return !missingReasoning(item.Get("encrypted_content")) || !missingReasoning(item.Get("signature")) || !missingReasoning(item.Get("reasoning_details"))
}
