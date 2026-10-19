package relayconvert

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiConversionPreservesJSONSchemaOrReportsLegacyDowngrade(t *testing.T) {
	const schema = `{"type":"object","properties":{"flag":{"type":"boolean","enum":[true]},"kind":{"const":"fixed"},"choice":{"oneOf":[{"type":"string"},{"type":"number"}]},"ref":{"$ref":"#/$defs/text"}},"$defs":{"text":{"type":"string"}},"required":["flag"]}`
	for _, source := range []string{"chat", "responses", "claude"} {
		for _, modern := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/jsonSchema=%t", source, modern), func(t *testing.T) {
				var request any
				switch source {
				case "chat":
					request = &dto.GeneralOpenAIRequest{}
					require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"gemini-fixture","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"check","parameters":`+schema+`}}],"response_format":{"type":"json_schema","json_schema":{"name":"result","schema":`+schema+`}}}`), request))
				case "responses":
					request = &dto.OpenAIResponsesRequest{}
					require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"gemini-fixture","input":"hello","tools":[{"type":"function","name":"check","parameters":`+schema+`}],"text":{"format":{"type":"json_schema","name":"result","schema":`+schema+`}}}`), request))
				case "claude":
					request = &dto.ClaudeRequest{}
					require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"gemini-fixture","messages":[{"role":"user","content":"hello"}],"tools":[{"name":"check","input_schema":`+schema+`}]}`), request))
				}
				before, err := kitutil.Marshal(request)
				require.NoError(t, err)
				meta := &convmeta.Values{UpstreamModelName: "gemini-fixture", ChannelMetaAttached: true, Options: &convmeta.Options{
					Gemini: convmeta.GeminiOptions{SupportsJSONSchema: func(model string) bool {
						assert.Equal(t, "gemini-fixture", model)
						return modern
					}},
				}}
				result, err := ConvertRequest(context.Background(), meta, types.RelayFormatGemini, request)
				require.NoError(t, err)
				body, err := kitutil.Marshal(result.Value)
				require.NoError(t, err)
				function := gjson.GetBytes(body, "tools.0.functionDeclarations.0")
				if modern {
					assert.JSONEq(t, schema, function.Get("parametersJsonSchema").Raw)
					assert.False(t, function.Get("parameters").Exists())
					if source != "claude" {
						assert.JSONEq(t, schema, gjson.GetBytes(body, "generationConfig.responseJsonSchema").Raw)
						assert.False(t, gjson.GetBytes(body, "generationConfig.responseSchema").Exists())
					}
					assert.Empty(t, result.Diagnostics)
				} else {
					assert.False(t, function.Get("parametersJsonSchema").Exists())
					assert.Equal(t, "BOOLEAN", function.Get("parameters.properties.flag.type").String())
					assert.False(t, function.Get("parameters.properties.flag.enum").Exists())
					require.NotEmpty(t, result.Diagnostics)
					assert.Equal(t, "gemini_schema_keyword_dropped", result.Diagnostics[0].Code)
					if source != "claude" {
						assert.False(t, gjson.GetBytes(body, "generationConfig.responseSchema.properties.flag.enum").Exists())
						assert.False(t, gjson.GetBytes(body, "generationConfig.responseJsonSchema").Exists())
					}
				}
				after, err := kitutil.Marshal(request)
				require.NoError(t, err)
				assert.JSONEq(t, string(before), string(after), "conversion must not mutate the request used for a retry")
			})
		}
	}
}

func TestResponsesRetryHistoryOmitsOnlyCompletedTrailingAssistantTurns(t *testing.T) {
	for _, target := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatGemini} {
		for _, tc := range []struct {
			name    string
			input   string
			turns   int
			omitted bool
		}{
			{"retry after answer", `[{"role":"user","content":"question"},{"role":"assistant","content":"partial answer"}]`, 1, true},
			{"only assistant", `[{"role":"assistant","content":"prefill"}]`, 1, false},
			{"pending tool call", `[{"role":"user","content":"question"},{"type":"function_call","call_id":"call_1","name":"check","arguments":"{}"}]`, 2, false},
			{"answer after tool result", `[{"role":"user","content":"question"},{"type":"function_call","call_id":"call_1","name":"check","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"result"},{"role":"assistant","content":"partial answer"}]`, 3, true},
		} {
			t.Run(fmt.Sprintf("%s/%s", target, tc.name), func(t *testing.T) {
				result, err := ConvertRequest(context.Background(), nil, target, &dto.OpenAIResponsesRequest{Model: "fixture", MaxOutputTokens: kitutil.GetPointer(uint(100)), Input: []byte(tc.input)})
				require.NoError(t, err)
				body, err := kitutil.Marshal(result.Value)
				require.NoError(t, err)
				path := "messages"
				if target == types.RelayFormatGemini {
					path = "contents"
				}
				wantTurns := tc.turns
				if target == types.RelayFormatClaude && tc.name == "only assistant" {
					wantTurns++ // Existing Claude conversion supplies an initial user turn.
				}
				assert.Len(t, gjson.GetBytes(body, path).Array(), wantTurns)
				if tc.omitted {
					require.Len(t, result.Diagnostics, 1)
					assert.Equal(t, "trailing_assistant_omitted", result.Diagnostics[0].Code)
				} else {
					assert.Empty(t, result.Diagnostics)
				}
			})
		}
		// Chat requests intentionally using a prefill keep their final turn.
		result, err := ConvertRequest(context.Background(), nil, target, &dto.GeneralOpenAIRequest{Model: "fixture", MaxTokens: kitutil.GetPointer(uint(100)), Messages: []dto.Message{{Role: "user", Content: "question"}, {Role: "assistant", Content: "prefill"}}})
		require.NoError(t, err)
		assert.Empty(t, result.Diagnostics)
		body, err := kitutil.Marshal(result.Value)
		require.NoError(t, err)
		assert.Contains(t, string(body), "prefill")
	}
}
