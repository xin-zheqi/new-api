package reasoning

import (
	"strconv"
	"strings"
)

type ModelModifier struct {
	Key   string
	Value string
}

type ModelModifierSpec struct {
	Raw       string
	Base      string
	Modifiers []ModelModifier
}

func (s ModelModifierSpec) HasModifiers() bool {
	return len(s.Modifiers) > 0
}

// ParseModelModifiers removes only a contiguous trailing chain of @key:value
// segments. Other @ characters remain part of the opaque model name.
func ParseModelModifiers(modelName string) ModelModifierSpec {
	spec := ModelModifierSpec{Raw: modelName, Base: modelName}
	parts := strings.Split(modelName, "@")
	if len(parts) < 2 {
		return spec
	}

	firstModifier := len(parts)
	for i := len(parts) - 1; i > 0; i-- {
		key, value, ok := parseModelModifierSegment(parts[i])
		if !ok {
			break
		}
		firstModifier = i
		spec.Modifiers = append([]ModelModifier{{Key: key, Value: value}}, spec.Modifiers...)
	}
	if firstModifier == len(parts) {
		return spec
	}

	base := strings.Join(parts[:firstModifier], "@")
	if base == "" {
		return ModelModifierSpec{Raw: modelName, Base: modelName}
	}
	spec.Base = base
	return spec
}

func parseModelModifierSegment(segment string) (string, string, bool) {
	colon := strings.IndexByte(segment, ':')
	if colon <= 0 {
		return "", "", false
	}
	key := segment[:colon]
	for i, r := range key {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if i == 0 && !letter || i > 0 && !letter && (r < '0' || r > '9') && r != '_' && r != '-' {
			return "", "", false
		}
	}
	return strings.ToLower(key), segment[colon+1:], true
}

// ParseThinkingModifier maps an explicit @thinking value onto a portable
// Intent. on/adaptive/off and integer budgets (including -1) are accepted;
// values below -1 are rejected.
func ParseThinkingModifier(raw string) (Intent, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "on":
		return Intent{Mode: ModeEnabled, Source: SourceSuffix}, true
	case "adaptive":
		return Intent{Mode: ModeAdaptive, Source: SourceSuffix}, true
	case "off":
		return Intent{Mode: ModeDisabled, Effort: EffortNone, Source: SourceSuffix}, true
	}
	if budget, err := strconv.Atoi(value); err == nil {
		if budget < -1 {
			return Intent{}, false
		}
		if budget == 0 {
			return Intent{Mode: ModeDisabled, Effort: EffortNone, Source: SourceSuffix}, true
		}
		return Intent{
			Mode:         ModeEnabled,
			BudgetTokens: &budget,
			Source:       SourceSuffix,
			BudgetSource: SourceSuffix,
		}, true
	}
	return Intent{}, false
}
