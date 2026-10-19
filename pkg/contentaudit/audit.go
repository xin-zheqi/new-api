// Package contentaudit inspects text values, never serialized request structure.
// It is deliberately independent of routing, billing, databases and remote services.
package contentaudit

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const (
	MaxRules             = 20
	MaxBodyBytes         = 8 << 20
	MaxTextBytes         = 1 << 20
	MaxOutputBytes       = 2 << 20
	MaxMatches           = 10000
	MaxDetails           = 48
	MaxJSONNodes         = 65536
	MaxRegexInstructions = 1024
)

var ErrLimit = errors.New("content audit limit exceeded; shorten the request or simplify your audit rules")

type Rule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Pattern     string `json:"pattern"`
	Flags       string `json:"flags"`
	Mode        string `json:"mode"`
	Action      string `json:"action"`
	Replacement string `json:"replacement"`
}

type Policy struct {
	Enabled bool   `json:"enabled"`
	Rules   []Rule `json:"rules"`
}

type CompiledRule struct {
	Rule
	expression *regexp.Regexp
}

type Match struct {
	Number      int    `json:"number"`
	RuleID      string `json:"rule_id"`
	RuleName    string `json:"rule_name"`
	Path        string `json:"path"`
	Action      string `json:"action"`
	Original    string `json:"original"`
	Replacement string `json:"replacement,omitempty"`
	Applied     bool   `json:"applied"`
	Truncated   bool   `json:"truncated"`
	Overlap     bool   `json:"overlap,omitempty"`
}

type Result struct {
	Action      string  `json:"action"`
	Count       int     `json:"count"`
	Matches     []Match `json:"matches"`
	Truncated   bool    `json:"truncated"`
	textBytes   int
	outputBytes int
}

func Compile(policy Policy) ([]CompiledRule, error) {
	if len(policy.Rules) > MaxRules {
		return nil, fmt.Errorf("at most %d audit rules are allowed", MaxRules)
	}
	seen := map[string]bool{}
	rules := make([]CompiledRule, 0, len(policy.Rules))
	for _, rule := range policy.Rules {
		if rule.ID == "" || len(rule.ID) > 64 || seen[rule.ID] {
			return nil, errors.New("audit rule IDs must be unique and 1–64 bytes")
		}
		seen[rule.ID] = true
		if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > 128 {
			return nil, errors.New("audit rule name must be 1–128 bytes")
		}
		if rule.Pattern == "" || len(rule.Pattern) > 512 || len(rule.Replacement) > 512 {
			return nil, errors.New("audit pattern and replacement must be at most 512 bytes")
		}
		if rule.Action != "warn" && rule.Action != "replace" && rule.Action != "block" {
			return nil, errors.New("invalid audit action")
		}
		if rule.Mode != "all" && rule.Mode != "first" {
			return nil, errors.New("invalid audit matching mode")
		}
		flags := ""
		for _, flag := range rule.Flags {
			if !strings.ContainsRune("ims", flag) || strings.ContainsRune(flags, flag) {
				return nil, errors.New("only unique i, m and s regex flags are supported")
			}
			flags += string(flag)
		}
		pattern := rule.Pattern
		if flags != "" {
			pattern = "(?" + flags + ")" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid RE2 expression", rule.Name)
		}
		if re.NumSubexp() > 32 {
			return nil, errors.New("at most 32 capture groups are allowed per rule")
		}
		tree, err := syntax.Parse(pattern, syntax.Perl)
		if err != nil || canMatchEmpty(tree) {
			return nil, fmt.Errorf("rule %q must not match empty text", rule.Name)
		}
		program, err := syntax.Compile(tree.Simplify())
		if err != nil || len(program.Inst) > MaxRegexInstructions {
			return nil, fmt.Errorf("rule %q is too complex; simplify repetitions and alternatives", rule.Name)
		}
		if rule.Action == "replace" {
			// Validate capture references so a typo cannot silently delete text.
			for i := 0; i < len(rule.Replacement); i++ {
				if rule.Replacement[i] != '$' {
					continue
				}
				i++
				if i < len(rule.Replacement) && rule.Replacement[i] == '$' {
					continue
				}
				if i >= len(rule.Replacement) || rule.Replacement[i] != '{' {
					return nil, errors.New("use ${1} or ${name} for captures and $$ for a literal dollar")
				}
				end := strings.IndexByte(rule.Replacement[i:], '}')
				if end < 0 {
					return nil, errors.New("unclosed replacement capture")
				}
				name := rule.Replacement[i+1 : i+end]
				valid := name == "0"
				for n := 1; n <= re.NumSubexp(); n++ {
					if name == fmt.Sprint(n) || (re.SubexpNames()[n] != "" && name == re.SubexpNames()[n]) {
						valid = true
					}
				}
				if !valid {
					return nil, errors.New("replacement references an unknown capture")
				}
				i += end
			}
		}
		if rule.Enabled {
			rules = append(rules, CompiledRule{rule, re})
		}
	}
	return rules, nil
}

type edit struct {
	start, end  int
	replacement string
}

// A word boundary can match zero characters even though it does not match
// the empty string. Reject all nullable patterns, including anchored branches,
// so ignored zero-width matches cannot hide a following sensitive match.
func canMatchEmpty(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpStar, syntax.OpQuest:
		return true
	case syntax.OpCapture, syntax.OpPlus:
		return canMatchEmpty(re.Sub[0])
	case syntax.OpRepeat:
		return re.Min == 0 || canMatchEmpty(re.Sub[0])
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !canMatchEmpty(sub) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			if canMatchEmpty(sub) {
				return true
			}
		}
	}
	return false
}

// Expand only validated capture references, checking the output bound before
// allocating. regexp.ExpandString alone can allocate a capture-amplified payload.
func expandReplacement(rule CompiledRule, text string, indices []int, budget int) (string, error) {
	var out strings.Builder
	template := rule.Replacement
	for len(template) > 0 {
		value := ""
		pos := strings.IndexByte(template, '$')
		if pos < 0 {
			value = template
			template = ""
		} else if pos > 0 {
			value = template[:pos]
			template = template[pos:]
		} else if strings.HasPrefix(template, "$$") {
			value = "$"
			template = template[2:]
		} else {
			end := strings.IndexByte(template, '}')
			name := template[2:end]
			n, err := strconv.Atoi(name)
			if err != nil {
				n = rule.expression.SubexpIndex(name)
			}
			if n >= 0 && 2*n+1 < len(indices) && indices[2*n] >= 0 {
				value = text[indices[2*n]:indices[2*n+1]]
			}
			template = template[end+1:]
		}
		if len(value) > budget-out.Len() {
			return "", ErrLimit
		}
		out.WriteString(value)
	}
	return out.String(), nil
}

func (result *Result) inspect(text, path string, rules []CompiledRule) (string, error) {
	result.textBytes += len(text)
	if result.textBytes > MaxTextBytes {
		return "", ErrLimit
	}
	edits := make([]edit, 0)
	occupied := make([]byte, len(text))
	replacementBytes := 0
	for _, rule := range rules {
		limit := MaxMatches - result.Count + 1
		// Warning and blocking only need the first hit per text field. This is
		// enforced server-side even if an older client submits mode=all.
		if rule.Mode == "first" || rule.Action != "replace" {
			limit = 1
		}
		for _, indices := range rule.expression.FindAllStringSubmatchIndex(text, limit) {
			start, end := indices[0], indices[1]
			if start == end {
				continue
			}
			result.Count++
			if result.Count > MaxMatches {
				return "", ErrLimit
			}
			matched := Match{Number: result.Count, RuleID: rule.ID, RuleName: rule.Name, Path: path, Action: rule.Action}
			matched.Original, matched.Truncated = snippet(text[start:end])
			if result.Action == "" {
				result.Action = "warn"
			}
			if rule.Action == "block" {
				result.Action = "block"
				matched.Applied = true
			}
			if rule.Action == "replace" {
				replacement, err := expandReplacement(rule, text, indices, MaxOutputBytes-result.outputBytes-replacementBytes)
				if err != nil {
					return "", err
				}
				// First configured replacement wins overlapping ranges. Every rule
				// still sees original text, so replacement cannot evade a blocker.
				overlap := bytes.IndexByte(occupied[start:end], 1) >= 0
				matched.Overlap = overlap
				if !overlap {
					if len(replacement) > MaxOutputBytes {
						return "", ErrLimit
					}
					replacementBytes += len(replacement)
					for i := start; i < end; i++ {
						occupied[i] = 1
					}
					edits = append(edits, edit{start, end, replacement})
					matched.Applied = true
					if result.Action != "block" {
						result.Action = "replace"
					}
				}
				var clipped bool
				matched.Replacement, clipped = snippet(replacement)
				matched.Truncated = matched.Truncated || clipped
			}
			if len(result.Matches) < MaxDetails {
				result.Matches = append(result.Matches, matched)
			} else {
				result.Truncated = true
			}
		}
	}
	if len(edits) == 0 {
		result.outputBytes += len(text)
		if result.outputBytes > MaxOutputBytes {
			return "", ErrLimit
		}
		return text, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	length := len(text)
	for _, change := range edits {
		length += len(change.replacement) - (change.end - change.start)
	}
	result.outputBytes += length
	if result.outputBytes > MaxOutputBytes {
		return "", ErrLimit
	}
	var out strings.Builder
	out.Grow(length)
	pos := 0
	for _, change := range edits {
		out.WriteString(text[pos:change.start])
		out.WriteString(change.replacement)
		pos = change.end
	}
	out.WriteString(text[pos:])
	return out.String(), nil
}

func snippet(text string) (string, bool) {
	if len(text) <= 160 {
		return text, false
	}
	end := 160
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end], true
}

// Check before allocating RawMessage arrays, including JSON encoded inside
// tool argument strings. Numeric/empty-object payloads consume memory too.
func checkJSONStructure(body []byte) error {
	inString, escaped, nodes := false, false, 0
	for _, char := range body {
		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		if char == '"' {
			inString = true
			continue
		}
		if char == '{' || char == '[' || char == ',' || char == ':' {
			nodes++
			if nodes > MaxJSONNodes {
				return ErrLimit
			}
		}
	}
	return nil
}

// InspectJSON uses RawMessage for untouched values. This preserves large
// integers and unknown provider extensions without turning numbers into floats.
func InspectJSON(body []byte, rules []CompiledRule) ([]byte, Result, error) {
	result := Result{Matches: []Match{}}
	if len(body) > MaxBodyBytes {
		return nil, result, ErrLimit
	}
	if err := checkJSONStructure(body); err != nil {
		return nil, result, err
	}
	var root map[string]common.RawMessage
	if err := common.Unmarshal(body, &root); err != nil || root == nil {
		return nil, result, errors.New("invalid audit request JSON")
	}
	for _, key := range []string{"messages", "input", "instructions", "system", "prompt", "contents", "systemInstruction", "system_instruction", "query", "documents"} {
		value, ok := root[key]
		if !ok {
			continue
		}
		updated, err := result.walk(value, key, rules, 0)
		if err != nil {
			return nil, result, err
		}
		root[key] = updated
	}
	if result.Action == "block" {
		for i := range result.Matches {
			result.Matches[i].Applied = result.Matches[i].Action == "block"
		}
		return body, result, nil
	}
	if result.Action != "replace" {
		return body, result, nil
	}
	updated, err := common.Marshal(root)
	if len(updated) > MaxBodyBytes {
		return nil, result, ErrLimit
	}
	return updated, result, err
}

func (result *Result) walk(raw common.RawMessage, path string, rules []CompiledRule, depth int) (common.RawMessage, error) {
	if depth > 32 {
		return nil, ErrLimit
	}
	switch common.GetJsonType(raw) {
	case "string":
		var text string
		if err := common.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		updated, err := result.inspect(text, path, rules)
		if err != nil {
			return nil, err
		}
		if updated == text {
			return raw, nil
		}
		return common.Marshal(updated)
	case "array":
		var items []common.RawMessage
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		for i := range items {
			updated, err := result.walk(items[i], fmt.Sprintf("%s[%d]", path, i), rules, depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = updated
		}
		return common.Marshal(items)
	case "object":
		var item map[string]common.RawMessage
		if err := common.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		var kind string
		_ = common.Unmarshal(item["type"], &kind)
		// Only dialogue containers and known text blocks are traversed. Image,
		// audio, encrypted reasoning and file references remain opaque.
		allowed := []string{"content", "parts"}
		payloads := []string{}
		switch kind {
		case "", "text", "input_text", "output_text":
			allowed = append(allowed, "text")
		case "function_call_output", "custom_tool_call_output":
			payloads = append(payloads, "output")
		case "message", "tool_result":
		case "tool_use":
			payloads = append(payloads, "input")
		case "function_call", "custom_tool_call":
			payloads = append(payloads, "arguments", "input")
		default:
			return raw, nil
		}
		// Tool arguments/results are user payloads, not API structure. Walk
		// string values only; object keys, numbers and call identifiers survive.
		for _, key := range payloads {
			if value, ok := item[key]; ok {
				updated, err := result.payload(value, path+"."+key, rules, depth+1, key == "arguments" || key == "output")
				if err != nil {
					return nil, err
				}
				item[key] = updated
			}
		}
		for _, key := range []string{"functionCall", "functionResponse", "function_call"} {
			if value, ok := item[key]; ok {
				var call map[string]common.RawMessage
				if common.Unmarshal(value, &call) == nil && call != nil {
					for _, field := range []string{"args", "response", "arguments"} {
						if payload, ok := call[field]; ok {
							updated, err := result.payload(payload, path+"."+key+"."+field, rules, depth+1, field == "arguments")
							if err != nil {
								return nil, err
							}
							call[field] = updated
						}
					}
					encoded, err := common.Marshal(call)
					if err != nil {
						return nil, err
					}
					item[key] = encoded
				}
			}
		}
		if rawCalls, ok := item["tool_calls"]; ok {
			var calls []map[string]common.RawMessage
			if common.Unmarshal(rawCalls, &calls) == nil {
				for i, call := range calls {
					var function map[string]common.RawMessage
					if common.Unmarshal(call["function"], &function) != nil || function == nil {
						continue
					}
					if arguments, ok := function["arguments"]; ok {
						updated, err := result.payload(arguments, fmt.Sprintf("%s.tool_calls[%d].function.arguments", path, i), rules, depth+1, true)
						if err != nil {
							return nil, err
						}
						function["arguments"] = updated
						encoded, err := common.Marshal(function)
						if err != nil {
							return nil, err
						}
						call["function"] = encoded
					}
				}
				encoded, err := common.Marshal(calls)
				if err != nil {
					return nil, err
				}
				item["tool_calls"] = encoded
			}
		}
		for _, key := range allowed {
			value, ok := item[key]
			if !ok {
				continue
			}
			updated, err := result.walk(value, path+"."+key, rules, depth+1)
			if err != nil {
				return nil, err
			}
			item[key] = updated
		}
		return common.Marshal(item)
	default:
		return raw, nil
	}
}

func (result *Result) payload(raw common.RawMessage, path string, rules []CompiledRule, depth int, encodedJSON bool) (common.RawMessage, error) {
	if depth > 32 {
		return nil, ErrLimit
	}
	if encodedJSON && common.GetJsonType(raw) == "string" {
		var value string
		if err := common.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		if common.GetJsonType([]byte(value)) == "object" || common.GetJsonType([]byte(value)) == "array" {
			// A leading '[' or '{' is also valid prose (for example a tool's
			// "[notice]" prefix). Only parse structurally valid embedded JSON;
			// RawMessage validates syntax without allocating its child nodes.
			var embedded common.RawMessage
			if common.Unmarshal([]byte(value), &embedded) == nil {
				if err := checkJSONStructure(embedded); err != nil {
					return nil, err
				}
				updated, err := result.payload(embedded, path, rules, depth+1, false)
				if err != nil {
					return nil, err
				}
				return common.Marshal(string(updated))
			}
		}
	}
	switch common.GetJsonType(raw) {
	case "object":
		var values map[string]common.RawMessage
		if err := common.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			// Do not persist arbitrary user-supplied JSON keys in detail paths.
			updated, err := result.payload(values[key], path+".value", rules, depth+1, false)
			if err != nil {
				return nil, err
			}
			values[key] = updated
		}
		return common.Marshal(values)
	case "array":
		var values []common.RawMessage
		if err := common.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		for i, value := range values {
			updated, err := result.payload(value, fmt.Sprintf("%s[%d]", path, i), rules, depth+1, false)
			if err != nil {
				return nil, err
			}
			values[i] = updated
		}
		return common.Marshal(values)
	default:
		return result.walk(raw, path, rules, depth)
	}
}
