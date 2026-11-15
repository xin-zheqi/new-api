package common

import (
	"strings"
	"testing"

	commonroot "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func regexReplaceOverrideFixture(pattern, replacement string) map[string]interface{} {
	return map[string]interface{}{
		"operations": []map[string]interface{}{{
			"path": "model", "mode": "regex_replace", "from": pattern, "to": replacement,
		}},
	}
}

func TestApplyParamOverrideRegexReplaceCachedSemantics(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		pattern     string
		replacement string
		want        string
	}{
		{
			name: "named captures and unicode", input: "模型-42",
			pattern: `^(?P<name>\p{Han}+)-(\d+)$`, replacement: `${name}/$2`, want: "模型/42",
		},
		{
			name: "same pattern with another replacement", input: "模型-42",
			pattern: `^(?P<name>\p{Han}+)-(\d+)$`, replacement: `$2-${name}`, want: "42-模型",
		},
		{
			name: "inline flags are part of pattern", input: "GPT-42",
			pattern: `(?i)^gpt-(\d+)$`, replacement: `model-$1`, want: "model-42",
		},
		{
			name: "edited pattern changes matching immediately", input: "GPT-42",
			pattern: `^gpt-(\d+)$`, replacement: `model-$1`, want: "GPT-42",
		},
		{
			name: "zero width matches", input: "模型",
			pattern: `^|$`, replacement: "_", want: "_模型_",
		},
		{
			name: "leftmost first alternation", input: "aa",
			pattern: `a|aa`, replacement: "x", want: "xx",
		},
		{
			name: "literal dollar", input: "gpt-42",
			pattern: `^gpt-(\d+)$`, replacement: `$$$1`, want: "$42",
		},
		{
			name: "empty replacement", input: "gpt-42",
			pattern: `^gpt-(\d+)$`, replacement: "", want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := commonroot.Marshal(map[string]interface{}{
				"model": tt.input, "stream": false, "max_tokens": 0,
			})
			require.NoError(t, err)
			want, err := commonroot.Marshal(map[string]interface{}{
				"model": tt.want, "stream": false, "max_tokens": 0,
			})
			require.NoError(t, err)

			out, err := ApplyParamOverride(input, regexReplaceOverrideFixture(tt.pattern, tt.replacement), nil)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(out))
		})
	}
}

func TestApplyParamOverrideRegexReplacePreservesValidationOrder(t *testing.T) {
	// The target type is checked before the pattern, even when compilation would
	// fail. Cached expressions must not bypass this existing validation order.
	_, err := ApplyParamOverride([]byte(`{"model":0}`), regexReplaceOverrideFixture("(", "x"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation not supported for type:")
	assert.NotContains(t, err.Error(), "error parsing regexp")
}

func TestApplyParamOverrideRegexReplaceLargeValidPattern(t *testing.T) {
	// Large patterns use normal compilation rather than becoming a new request
	// validation limit just because they are unsuitable for the shared cache.
	pattern := strings.Repeat("a", 16*1024)
	input, err := commonroot.Marshal(map[string]string{"model": pattern})
	require.NoError(t, err)
	out, err := ApplyParamOverride(input, regexReplaceOverrideFixture(pattern, "matched"), nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"matched"}`, string(out))
}

func TestApplyParamOverrideRegexReplaceConcurrentReplacements(t *testing.T) {
	const pattern = `^gpt-(\d+)$`
	out, err := ApplyParamOverride([]byte(`{"model":"gpt-42"}`), regexReplaceOverrideFixture(pattern, "$1"), nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"42"}`, string(out))

	tests := []struct {
		replacement string
		want        string
	}{
		{replacement: "openai/$1", want: "openai/42"},
		{replacement: "model-$1", want: "model-42"},
		{replacement: "$1-suffix", want: "42-suffix"},
		{replacement: "$$$1", want: "$42"},
	}
	for _, tt := range tests {
		t.Run(tt.replacement, func(t *testing.T) {
			t.Parallel()
			out, err := ApplyParamOverride([]byte(`{"model":"gpt-42"}`), regexReplaceOverrideFixture(pattern, tt.replacement), nil)
			require.NoError(t, err)
			want, err := commonroot.Marshal(map[string]string{"model": tt.want})
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(out))
		})
	}
}
