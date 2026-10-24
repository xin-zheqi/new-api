package common

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NormalizeDeepSeekSchemas handles DeepSeek's primitive-only `type` arrays.
// A scalar type:"array" is valid; only unions containing array/object need an
// equivalent anyOf. This is independent of thinking and history-repair toggles.
// Only schema subtrees are patched; conversation bytes and retry input survive.
func NormalizeDeepSeekSchemas(data []byte, info *RelayInfo) []byte {
	if info == nil || info.ChannelMeta == nil || info.ChannelType != constant.ChannelTypeDeepSeek || info.ChannelSetting.PassThroughBodyEnabled {
		return data
	}
	format := info.GetFinalRequestRelayFormat()
	if format != types.RelayFormatOpenAI && format != types.RelayFormatOpenAIResponses {
		return data
	}
	root := gjson.ParseBytes(data)
	var schemas []gjson.Result
	toolLists, visited := []gjson.Result{root.Get("tools")}, 0
	for len(toolLists) > 0 && visited < 256 {
		list := toolLists[0]
		toolLists = toolLists[1:]
		if !list.IsArray() {
			continue
		}
		list.ForEach(func(_, tool gjson.Result) bool {
			visited++
			switch tool.Get("type").String() {
			case "function":
				path := "function.parameters"
				if format == types.RelayFormatOpenAIResponses {
					path = "parameters"
				}
				schemas = append(schemas, tool.Get(path))
			case "namespace":
				if format == types.RelayFormatOpenAIResponses {
					toolLists = append(toolLists, tool.Get("tools"))
				}
			}
			return visited < 256
		})
	}
	output := root.Get("response_format")
	path := "json_schema.schema"
	if format == types.RelayFormatOpenAIResponses {
		output, path = root.Get("text.format"), "schema"
	}
	if output.Get("type").String() == "json_schema" {
		schemas = append(schemas, output.Get(path))
	}

	var patches []jsonTextPatch
	// Bound total traversal and expansion for the request, including all tools.
	budget := deepSeekSchemaBudget{nodes: 4096, bytes: 2 << 20}
	for _, schema := range schemas {
		if !schema.IsObject() || len(schema.Raw) > budget.bytes {
			continue
		}
		budget.bytes -= len(schema.Raw)
		value, safe := normalizeDeepSeekSchema(schema.Raw, 0, &budget)
		if safe && value != schema.Raw {
			patches = append(patches, jsonTextPatch{start: schema.Index, end: schema.Index + len(schema.Raw), value: value})
		}
	}
	if len(patches) == 0 {
		return data
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	return []byte(applyJSONTextPatches(string(data), patches))
}

type deepSeekSchemaBudget struct {
	nodes, bytes int
}

// Visit schema keywords, never literal objects inside enum/default/examples.
// Each anyOf branch retains every original constraint, including enum/const;
// removing null or dropping array constraints would change the tool contract.
// References/identifiers make relocating subschemas unsafe (JSON pointers and
// resource scopes can change), so leave that entire schema untouched instead.
func normalizeDeepSeekSchema(raw string, depth int, budget *deepSeekSchemaBudget) (string, bool) {
	budget.nodes--
	if depth > 64 || budget.nodes < 0 {
		return raw, false
	}
	root := gjson.Parse(raw)
	if !root.IsObject() {
		return raw, true
	}
	var children []gjson.Result
	safe := true
	root.ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "$ref", "$dynamicRef", "$recursiveRef", "$id", "id", "$anchor", "$dynamicAnchor", "$recursiveAnchor":
			safe = false
		case "$schema":
			// Only known JSON Schema dialects have the union semantics used here.
			switch value.String() {
			case "https://json-schema.org/draft/2020-12/schema", "https://json-schema.org/draft/2019-09/schema", "http://json-schema.org/draft-07/schema#", "http://json-schema.org/draft-06/schema#", "http://json-schema.org/draft-04/schema#":
			default:
				safe = false
			}
		case "properties", "patternProperties", "$defs", "$def", "definitions", "dependentSchemas", "dependencies":
			if value.IsObject() {
				value.ForEach(func(_, child gjson.Result) bool {
					if child.IsObject() { // dependencies can also be arrays of names.
						children = append(children, child)
					}
					return true
				})
			}
		case "items", "additionalItems", "additionalProperties", "unevaluatedItems", "unevaluatedProperties", "contains", "propertyNames", "not", "if", "then", "else", "allOf", "anyOf", "oneOf", "prefixItems":
			if value.IsObject() {
				children = append(children, value)
			} else if value.IsArray() {
				value.ForEach(func(_, child gjson.Result) bool {
					children = append(children, child)
					return true
				})
			}
		}
		return safe
	})
	if !safe || len(children) > budget.nodes {
		return raw, false
	}
	var patches []jsonTextPatch
	for _, child := range children {
		value, ok := normalizeDeepSeekSchema(child.Raw, depth+1, budget)
		if !ok {
			return raw, false
		}
		if value != child.Raw {
			patches = append(patches, jsonTextPatch{start: child.Index, end: child.Index + len(child.Raw), value: value})
		}
	}
	if len(patches) > 0 {
		raw = applyJSONTextPatches(raw, patches)
	}
	typesValue := root.Get("type")
	if !typesValue.IsArray() {
		return raw, true
	}
	var names []string
	complex := false
	seen := make(map[string]bool, 7)
	typesValue.ForEach(func(_, value gjson.Result) bool {
		name := value.String()
		switch name {
		case "array", "object":
			complex = true
		case "string", "number", "integer", "boolean", "null":
		default:
			safe = false
		}
		if value.Type != gjson.String || len(names) >= 7 || seen[name] {
			safe = false
		}
		seen[name] = true
		names = append(names, name)
		return safe
	})
	if !safe || !complex {
		return raw, true // Invalid or primitive-only unions remain upstream-owned.
	}
	// Account for branch copies before allocating, to bound nested union growth.
	if (len(raw)+16)*len(names)+16 > budget.bytes {
		return raw, false
	}
	var out strings.Builder
	out.WriteByte('{')
	if dialect := root.Get("$schema"); dialect.Exists() {
		out.WriteString(`"$schema":` + dialect.Raw + `,`)
		var err error
		raw, err = sjson.Delete(raw, "$schema")
		if err != nil {
			return raw, false
		}
	}
	out.WriteString(`"anyOf":[`)
	for i, name := range names {
		branch, err := sjson.Set(raw, "type", name)
		if err != nil {
			return raw, false
		}
		if i > 0 {
			out.WriteByte(',')
		}
		out.WriteString(branch)
	}
	out.WriteString(`]}`)
	budget.bytes -= out.Len()
	return out.String(), true
}
