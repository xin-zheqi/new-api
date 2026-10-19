package contentaudit

import (
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileRuleCountLimit(t *testing.T) {
	rules := make([]Rule, MaxRules)
	for i := range rules {
		rules[i] = auditRule("warn", "secret", "")
		rules[i].ID = "rule-" + strconv.Itoa(i)
	}
	_, err := Compile(Policy{Rules: rules})
	require.NoError(t, err)

	rules = append(rules, Rule{ID: "rule-extra", Name: "Extra", Enabled: true, Pattern: "secret", Action: "warn", Mode: "first"})
	_, err = Compile(Policy{Rules: rules})
	require.ErrorContains(t, err, "at most 20 audit rules are allowed")
}

func auditRule(action, pattern, replacement string) Rule {
	return Rule{ID: "rule-1", Name: "Privacy", Enabled: true, Pattern: pattern, Action: action, Mode: "all", Replacement: replacement}
}

func TestAuditProtocolTextPreservesStructure(t *testing.T) {
	cases := []struct {
		name, body string
		count      int
	}{
		{"chat", `{"model":"secret","messages":[{"role":"user","content":"secret {} \"hi\" secret"},{"role":"assistant","content":[{"type":"text","text":"secret"},{"type":"image_url","image_url":{"url":"secret"}}]}],"metadata":{"secret":"secret"},"seed":18446744073709551615}`, 3},
		{"responses", `{"model":"secret","instructions":"secret","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"secret"},{"type":"input_image","image_url":"secret"}]},{"type":"function_call_output","call_id":"secret","output":"secret"}]}`, 3},
		{"messages", `{"model":"secret","system":[{"type":"text","text":"secret"}],"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"secret","content":"secret"}]}],"tools":[{"name":"secret","input_schema":{"description":"secret"}}]}`, 2},
		{"gemini", `{"systemInstruction":{"parts":[{"text":"secret"}]},"contents":[{"role":"user","parts":[{"text":"secret"},{"inlineData":{"data":"secret","mimeType":"image/png"}}]}]}`, 2},
	}
	rules, err := Compile(Policy{Rules: []Rule{auditRule("replace", "secret", "[safe]")}})
	require.NoError(t, err)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated, result, err := InspectJSON([]byte(tc.body), rules)
			require.NoError(t, err)
			assert.Equal(t, tc.count, result.Count)
			assert.Equal(t, "replace", result.Action)
			var root map[string]common.RawMessage
			require.NoError(t, common.Unmarshal(updated, &root))
			if model, ok := root["model"]; ok {
				assert.JSONEq(t, `"secret"`, string(model))
			}
			if seed, ok := root["seed"]; ok {
				assert.Equal(t, "18446744073709551615", string(seed))
			}
			if metadata, ok := root["metadata"]; ok {
				assert.JSONEq(t, `{"secret":"secret"}`, string(metadata))
			}
			if tools, ok := root["tools"]; ok {
				assert.Contains(t, string(tools), "secret")
			}
			assert.Contains(t, string(updated), "[safe]")
		})
	}
}

func TestBlockUsesOriginalTextAndDominatesReplacement(t *testing.T) {
	replacement := auditRule("replace", "secret", "safe")
	block := auditRule("block", "secret", "")
	block.ID = "block"
	rules, err := Compile(Policy{Rules: []Rule{replacement, block}})
	require.NoError(t, err)
	body := []byte(`{"input":"secret"}`)
	updated, result, err := InspectJSON(body, rules)
	require.NoError(t, err)
	assert.Equal(t, body, updated)
	assert.Equal(t, "block", result.Action)
	assert.Equal(t, 2, result.Count)
	assert.False(t, result.Matches[0].Applied)
	assert.True(t, result.Matches[1].Applied)
}

func TestWarnAndBlockUseFirstMatchPerTextField(t *testing.T) {
	for _, action := range []string{"warn", "block"} {
		t.Run(action, func(t *testing.T) {
			rules, err := Compile(Policy{Rules: []Rule{auditRule(action, "secret", "")}})
			require.NoError(t, err)
			_, result, err := InspectJSON([]byte(`{"input":"secret secret"}`), rules)
			require.NoError(t, err)
			assert.Equal(t, 1, result.Count)
			assert.Equal(t, action, result.Action)
		})
	}
}

func TestMultipleMatchesAreEnforcedBeyondDetailLimit(t *testing.T) {
	rules, err := Compile(Policy{Rules: []Rule{auditRule("replace", "secret", "safe")}})
	require.NoError(t, err)
	body, err := common.Marshal(map[string]string{"input": strings.Repeat("secret ", MaxDetails+2)})
	require.NoError(t, err)
	updated, result, err := InspectJSON(body, rules)
	require.NoError(t, err)
	assert.Equal(t, MaxDetails+2, result.Count)
	assert.Len(t, result.Matches, MaxDetails)
	assert.True(t, result.Truncated)
	assert.NotContains(t, string(updated), "secret")
}
func TestOverlappingReplacementsAndFirstMode(t *testing.T) {
	first := auditRule("replace", "secret", "SAFE")
	first.Mode = "first"
	second := auditRule("replace", "secret", "SECOND")
	second.ID = "rule-2"
	rules, err := Compile(Policy{Rules: []Rule{first, second}})
	require.NoError(t, err)
	updated, result, err := InspectJSON([]byte(`{"input":["secret secret","secret"]}`), rules)
	require.NoError(t, err)
	assert.JSONEq(t, `{"input":["SAFE SECOND","SAFE"]}`, string(updated))
	assert.True(t, result.Matches[1].Overlap)
}
func TestCaptureExpansionIsBoundedAndUnicodeSafe(t *testing.T) {
	for _, tc := range []struct {
		name, pattern, replacement, input string
		limit                             bool
	}{
		{"captures", "(?P<name>秘密)", "[\u0024{name}] $$", "秘密", false},
		{"amplification", "(.+) ", strings.Repeat("\u0024{1}", 100), strings.Repeat("a", 40000) + " ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules, err := Compile(Policy{Rules: []Rule{auditRule("replace", tc.pattern, tc.replacement)}})
			require.NoError(t, err)
			body, err := common.Marshal(map[string]string{"input": tc.input})
			require.NoError(t, err)
			updated, _, err := InspectJSON(body, rules)
			if tc.limit {
				require.ErrorIs(t, err, ErrLimit)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, `{"input":"[秘密] $"}`, string(updated))
		})
	}
}
func TestToolArgumentsAreReplacedAsValuesNotJSONSyntax(t *testing.T) {
	rules, err := Compile(Policy{Rules: []Rule{auditRule("replace", "secret|[{}]", "SAFE")}})
	require.NoError(t, err)
	body := []byte(`{"input":[{"type":"function_call","name":"secret","call_id":"secret","arguments":"{\"secret\":\"secret\",\"n\":18446744073709551615}"}]}`)
	updated, result, err := InspectJSON(body, rules)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Count)
	var root struct {
		Input []struct {
			Arguments string
			Name      string
		}
	}
	require.NoError(t, common.Unmarshal(updated, &root))
	assert.JSONEq(t, `{"secret":"SAFE","n":18446744073709551615}`, root.Input[0].Arguments)
	assert.Equal(t, "secret", root.Input[0].Name)
}

func TestToolOutputJSONLikeTextRemainsValidDialogue(t *testing.T) {
	rules, err := Compile(Policy{Rules: []Rule{auditRule("replace", "secret", "safe")}})
	require.NoError(t, err)
	for _, output := range []string{"[notice] secret", "{secret}: tool output"} {
		body, err := common.Marshal(map[string]any{"input": []any{map[string]string{"type": "function_call_output", "output": output}}})
		require.NoError(t, err)
		updated, result, err := InspectJSON(body, rules)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Count)
		assert.Contains(t, string(updated), strings.ReplaceAll(output, "secret", "safe"))
	}
}

func TestEncodedToolJSONCannotBypassStructuralLimit(t *testing.T) {
	rules, err := Compile(Policy{Rules: []Rule{auditRule("warn", "secret", "")}})
	require.NoError(t, err)
	arguments := "[" + strings.Repeat("0,", MaxJSONNodes) + "0]"
	body, err := common.Marshal(map[string]any{"input": []any{map[string]string{"type": "function_call", "arguments": arguments}}})
	require.NoError(t, err)
	_, _, err = InspectJSON(body, rules)
	require.ErrorIs(t, err, ErrLimit)
}
func TestInvalidRegexAndReplacementsAreRejected(t *testing.T) {
	for _, rule := range []Rule{auditRule("warn", "(?<=secret)x", ""), auditRule("warn", "a*", ""), auditRule("replace", "secret", "$typo"), auditRule("replace", "(secret)", "\u0024{2}")} {
		_, err := Compile(Policy{Rules: []Rule{rule}})
		require.Error(t, err)
	}
}

func TestAuditRejectsExpensiveRegexButAcceptsPrivacyPatterns(t *testing.T) {
	_, err := Compile(Policy{Rules: []Rule{auditRule("warn", "a{600}b{600}", "")}})
	require.ErrorContains(t, err, "too complex")
	rules, err := Compile(Policy{Rules: []Rule{auditRule("replace", "token-[A-Za-z0-9]{32,64}", "[redacted]")}})
	require.NoError(t, err)
	body := []byte(`{"input":"token-0123456789abcdef0123456789abcdef"}`)
	updated, result, err := InspectJSON(body, rules)
	require.NoError(t, err)
	assert.JSONEq(t, `{"input":"[redacted]"}`, string(updated))
	assert.Equal(t, 1, result.Count)
}

func TestAuditRejectsZeroWidthAlternativesAndOversizedStructure(t *testing.T) {
	rule := auditRule("warn", `\b|secret`, "")
	_, err := Compile(Policy{Rules: []Rule{rule}})
	require.Error(t, err)
	rules, err := Compile(Policy{Rules: []Rule{auditRule("warn", "secret", "")}})
	require.NoError(t, err)
	body := []byte(`{"input":[` + strings.Repeat("0,", MaxJSONNodes) + `0]}`)
	_, _, err = InspectJSON(body, rules)
	require.ErrorIs(t, err, ErrLimit)
}
