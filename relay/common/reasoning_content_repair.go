package common

import (
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NewReasoningAwareJSONBody runs after conversion and parameter overrides.
// Passthrough paths deliberately keep using the original replayable body.
func NewReasoningAwareJSONBody(data []byte, info *RelayInfo) (common.ReplayableBody, io.Closer, error) {
	data, err := RepairReasoningRequest(data, info)
	if err != nil {
		return nil, nil, err
	}
	return NewOutboundJSONBody(data)
}

// RepairReasoningRequest normalizes genuine reasoning aliases on the same
// history item. It never borrows user/tool text, answers, summaries, or another
// turn's reasoning. Raw JSON patches preserve unknown provider fields, and
// never modify the incoming byte slice reused by retries.
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
	if !reasoningRepairEnabled(root, info.ChannelType) {
		return data, nil
	}
	info.ReasoningContentRepairActive = true
	path := "messages"
	if format == types.RelayFormatOpenAIResponses {
		path = "input"
	}
	items := root.Get(path)
	if !items.IsArray() {
		return data, nil
	}
	var patches []reasoningJSONPatch
	used := 0
	var repairErr error
	items.ForEach(func(_, item gjson.Result) bool {
		if len(patches) >= maxReasoningRepairItems {
			return false
		}
		if protectedReasoning(item) {
			return true
		}
		field := "reasoning_content"
		eligible := item.Get("role").String() == "assistant"
		if format == types.RelayFormatOpenAIResponses {
			field, eligible = "content", item.Get("type").String() == "reasoning"
		}
		if !eligible || !missingReasoning(item.Get(field)) {
			return true
		}
		alias := item.Get("reasoning")
		if format == types.RelayFormatOpenAIResponses {
			alias = item.Get("reasoning_content")
			if alias.Type != gjson.String || alias.String() == "" {
				alias = item.Get("reasoning_text")
			}
		}
		if alias.Type != gjson.String || strings.TrimSpace(alias.String()) == "" || len(alias.String()) > maxReasoningRepairBytes-used {
			return true
		}
		used += len(alias.String())
		var value any = alias.String()
		if format == types.RelayFormatOpenAIResponses {
			value = []reasoningTextPart{{Type: "reasoning_text", Text: alias.String()}}
		}
		encoded, err := common.Marshal(value)
		if err != nil {
			repairErr = err
			return false
		}
		raw, err := sjson.SetRaw(item.Raw, field, string(encoded))
		if err != nil {
			repairErr = err
			return false
		}
		patches = append(patches, reasoningJSONPatch{start: item.Index, end: item.Index + len(item.Raw), value: raw})
		return true
	})
	if repairErr != nil || len(patches) == 0 {
		return data, repairErr
	}
	return []byte(applyReasoningPatches(string(data), patches)), nil
}

// Explicit disabling always wins, including overrides applied by a channel.
// Unknown model defaults are not guessed. DeepSeek V4 and reasoner defaults
// are only inferred for the actual DeepSeek channel, not arbitrary gateways.
func reasoningRepairEnabled(root gjson.Result, channelType int) bool {
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
	model := root.Get("model").String()
	return enabled || channelType == constant.ChannelTypeDeepSeek &&
		(model == "deepseek-reasoner" || strings.HasPrefix(model, "deepseek-v4"))
}

func missingReasoning(value gjson.Result) bool {
	return !value.Exists() || value.Type == gjson.Null || value.Type == gjson.String && value.String() == "" || value.IsArray() && value.Get("#").Int() == 0
}

func protectedReasoning(item gjson.Result) bool {
	return !missingReasoning(item.Get("encrypted_content")) || !missingReasoning(item.Get("signature")) || !missingReasoning(item.Get("reasoning_details"))
}
