package common

import (
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/tidwall/gjson"
)

// ApplyReasoningEffortMapping changes only explicit string-valued strength
// controls in the final upstream protocol. It neither invents absent settings
// nor turns token budgets, enable flags, history, or encrypted items into effort.
// Matching is exact and one-step; original bytes stay available for a retry on
// another channel, and mappings cannot cascade within the same request.
func ApplyReasoningEffortMapping(data []byte, info *RelayInfo) ([]byte, error) {
	if info == nil || info.ChannelMeta == nil || len(info.ChannelSetting.ReasoningEffortMapping) == 0 || info.ChannelSetting.PassThroughBodyEnabled {
		return data, nil
	}
	var paths []string
	nativeOllama := false
	switch info.GetFinalRequestRelayFormat() {
	case types.RelayFormatOpenAI:
		paths = []string{"reasoning_effort", "reasoning.effort"}
		if info.ChannelType == constant.ChannelTypeOllama {
			// Ollama's chat/generate adapter has already rendered the control
			// as think. Its boolean form is an enable flag, never a strength.
			paths = []string{"think"}
			nativeOllama = true
		}
	case types.RelayFormatOpenAIResponses:
		paths = []string{"reasoning.effort"}
	case types.RelayFormatClaude:
		paths = []string{"output_config.effort"}
	case types.RelayFormatGemini:
		paths = []string{"generationConfig.thinkingConfig.thinkingLevel", "generation_config.thinking_config.thinking_level"}
	default:
		return data, nil
	}

	var patches []jsonTextPatch
	var ollamaEffort string
	size := len(data)
	for _, path := range paths {
		value := gjson.GetBytes(data, path)
		if value.Type != gjson.String {
			continue
		}
		target, matched := info.ChannelSetting.ReasoningEffortMapping[value.String()]
		if !matched || target == "" || target == value.String() {
			continue
		}
		encoded, err := common.Marshal(target)
		if err != nil {
			return nil, err
		}
		patches = append(patches, jsonTextPatch{start: value.Index, end: value.Index + len(value.Raw), value: string(encoded)})
		if nativeOllama {
			ollamaEffort = target
		}
		size += len(encoded) - len(value.Raw)
	}
	if len(patches) == 0 {
		return data, nil
	}
	// JSON member order is unconstrained. Copy the potentially large context
	// exactly once, regardless of how many effort aliases need replacement.
	slices.SortFunc(patches, func(a, b jsonTextPatch) int { return a.start - b.start })
	result := make([]byte, 0, size)
	last := 0
	for _, patch := range patches {
		result = append(result, data[last:patch.start]...)
		result = append(result, patch.value...)
		last = patch.end
	}
	result = append(result, data[last:]...)
	if nativeOllama {
		info.SetReasoningEffort(ollamaEffort)
	} else {
		syncReasoningEffortAfterParamOverride(info, data, result)
	}
	return result, nil
}
