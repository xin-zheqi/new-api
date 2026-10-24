package common

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekSchemaUnionCompatibility(t *testing.T) {
	for _, tc := range []struct{ name, schema, want string }{
		{"nullable array", `{"type":["array","null"],"items":{"type":"string"},"minItems":1}`, `{"anyOf":[{"type":"array","items":{"type":"string"},"minItems":1},{"type":"null","items":{"type":"string"},"minItems":1}]}`},
		{"nullable object", `{"type":["object","null"],"properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`, `{"anyOf":[{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false},{"type":"null","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}]}`},
		{"enum still excludes null", `{"type":["array","null"],"enum":[[1],[2]],"items":{"type":"integer"}}`, `{"anyOf":[{"type":"array","enum":[[1],[2]],"items":{"type":"integer"}},{"type":"null","enum":[[1],[2]],"items":{"type":"integer"}}]}`},
		{"existing composition survives", `{"type":["array","null"],"anyOf":[{"const":null},{"const":[1]}]}`, `{"anyOf":[{"type":"array","anyOf":[{"const":null},{"const":[1]}]},{"type":"null","anyOf":[{"const":null},{"const":[1]}]}]}`},
		{"ordinary array", `{"type":"array","items":{"type":"string"}}`, ""},
		{"primitive union", `{"type":["string","null"],"description":"optional"}`, ""},
		{"invalid union left alone", `{"type":["array",42]}`, ""},
		{"reference paths must not move", `{"type":"object","properties":{"a":{"type":["array","null"],"items":{"type":"string"}},"b":{"$ref":"#/properties/a/items"}}}`, ""},
		{"standard dialect stays at root", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":["object","null"]}`, `{"$schema":"https://json-schema.org/draft/2020-12/schema","anyOf":[{"type":"object"},{"type":"null"}]}`},
		{"unknown dialect", `{"$schema":"https://example.com/custom-dialect","type":["object","null"]}`, ""},
		{"invalid duplicate union", `{"type":["array","array"]}`, ""},
		{"literal values are not schemas", `{"type":"object","default":{"type":["array","null"]},"enum":[{"type":["object","null"]}],"examples":[{"type":["array","null"]}]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"text":{"format":{"type":"json_schema","name":"answer","strict":true,"schema":` + tc.schema + `}},"input": [{"role":"user","content":"keep exactly"}]}`)
			original := string(body)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek}}
			got := NormalizeDeepSeekSchemas(body, info)
			require.True(t, gjson.ValidBytes(got))
			assert.Equal(t, original, string(body), "retry input must not be mutated")
			assert.Equal(t, gjson.Get(original, "input").Raw, gjson.GetBytes(got, "input").Raw)
			assert.True(t, gjson.GetBytes(got, "text.format.strict").Bool())
			if tc.want == "" {
				assert.Equal(t, original, string(got))
			} else {
				assert.JSONEq(t, tc.want, gjson.GetBytes(got, "text.format.schema").Raw)
				assert.Equal(t, string(got), string(NormalizeDeepSeekSchemas(got, info)), "normalization is idempotent")
			}
		})
	}
}

func TestDeepSeekSchemaRequestScope(t *testing.T) {
	schema := `{"type":"object","properties":{"args":{"type":["array","null"],"items":{"type":"string"}}},"required":["args"]}`
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/thinking=%t", format, enabled), func(t *testing.T) {
				body := `{"messages":[],"tools":[{"type":"function","function":{"name":"run","strict":false,"parameters":` + schema + `}}],"response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":` + schema + `}}}`
				toolPath, outputPath := "tools.0.function.parameters", "response_format.json_schema.schema"
				if format == types.RelayFormatOpenAIResponses {
					body = `{"text":{"format":{"type":"json_schema","name":"answer","schema":` + schema + `}},"tools":[{"type":"function","name":"run","strict":false,"parameters":` + schema + `},{"type":"custom","name":"patch","parameters":` + schema + `}],"input":[]}`
					toolPath, outputPath = "tools.0.parameters", "text.format.schema"
				}
				info := &RelayInfo{RelayFormat: format, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek, ChannelSetting: dto.ChannelSettings{ReasoningContentRepair: enabled, ToolOutputRepair: enabled}}}
				got := NormalizeDeepSeekSchemas([]byte(body), info)
				require.True(t, gjson.ValidBytes(got))
				for _, path := range []string{toolPath, outputPath} {
					assert.Equal(t, "array", gjson.GetBytes(got, path+".properties.args.anyOf.0.type").String())
					assert.Equal(t, "null", gjson.GetBytes(got, path+".properties.args.anyOf.1.type").String())
					assert.Equal(t, `["args"]`, gjson.GetBytes(got, path+".required").Raw)
				}
				assert.Equal(t, gjson.Get(body, "tools.1").Raw, gjson.GetBytes(got, "tools.1").Raw)
				info.ChannelType = constant.ChannelTypeOpenAI
				assert.Equal(t, body, string(NormalizeDeepSeekSchemas([]byte(body), info)))
				info.ChannelType = constant.ChannelTypeDeepSeek
				info.ChannelSetting.PassThroughBodyEnabled = true
				assert.Equal(t, body, string(NormalizeDeepSeekSchemas([]byte(body), info)))
				info.ChannelSetting.PassThroughBodyEnabled = false
				info.RelayFormat = types.RelayFormatClaude
				assert.Equal(t, body, string(NormalizeDeepSeekSchemas([]byte(body), info)))
			})
		}
	}
}

func TestDeepSeekSchemaBoundsLeaveWholeSchemaIntact(t *testing.T) {
	for _, tc := range []struct{ name, schema string }{
		{"deep", strings.Repeat(`{"type":["array","null"],"items":`, 66) + `{"type":"string"}` + strings.Repeat(`}`, 66)},
		{"expansion", `{"type":["array","null"],"description":"` + strings.Repeat("x", 1<<20) + `","items":{"type":"string"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"tools":[{"type":"function","name":"run","parameters":` + tc.schema + `}]}`)
			info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek}}
			assert.Equal(t, body, NormalizeDeepSeekSchemas(body, info))
		})
	}
}

func TestDeepSeekSchemaNamespacedFunction(t *testing.T) {
	body := []byte(`{"tools":[{"type":"namespace","name":"agent","tools":[{"type":"function","name":"run","parameters":{"type":"object","properties":{"args":{"type":["array","null"],"items":{"type":"string"}}}}}]}]}`)
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeDeepSeek}}
	got := NormalizeDeepSeekSchemas(body, info)
	require.True(t, gjson.ValidBytes(got))
	assert.Equal(t, "array", gjson.GetBytes(got, "tools.0.tools.0.parameters.properties.args.anyOf.0.type").String())
	assert.Equal(t, "agent", gjson.GetBytes(got, "tools.0.name").String())
	assert.Equal(t, "run", gjson.GetBytes(got, "tools.0.tools.0.name").String())
}
