package common

import (
	"io"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasoningEffortMappingPreservesProtocolAndHistory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		format  types.RelayFormat
		input   string
		want    string
		effort  string
		mapping map[string]string
	}{
		{
			name:   "chat primary and alias use the original value once",
			format: types.RelayFormatOpenAI,
			input:  " { \"reasoning\": { \"effort\": \"max\", \"enabled\": true }, \"reasoning_effort\":\"xhigh\", \"messages\":[] } ",
			want:   " { \"reasoning\": { \"effort\": \"low\", \"enabled\": true }, \"reasoning_effort\":\"max\", \"messages\":[] } ",
			effort: "max",
		},
		{
			name:   "responses changes strength without hydrating encrypted reasoning",
			format: types.RelayFormatOpenAIResponses,
			input:  `{"reasoning":{"effort":"xhigh","summary":"auto"},"input":[{"type":"reasoning","encrypted_content":"A+/=opaque","summary":[{"type":"summary_text","text":"xhigh"}]},{"role":"user","content":"reasoning_effort: xhigh"}],"store":false,"tools":[]}`,
			want:   `{"reasoning":{"effort":"max","summary":"auto"},"input":[{"type":"reasoning","encrypted_content":"A+/=opaque","summary":[{"type":"summary_text","text":"xhigh"}]},{"role":"user","content":"reasoning_effort: xhigh"}],"store":false,"tools":[]}`,
			effort: "max",
		},
		{
			name:    "claude output effort keeps budget and signed history",
			format:  types.RelayFormatClaude,
			input:   `{"output_config":{"effort":"high","format":{"type":"json_schema","schema":{"type":"object"}}},"thinking":{"type":"adaptive"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"xhigh","signature":"opaque"}]}]}`,
			want:    `{"output_config":{"effort":"max","format":{"type":"json_schema","schema":{"type":"object"}}},"thinking":{"type":"adaptive"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"xhigh","signature":"opaque"}]}]}`,
			effort:  "max",
			mapping: map[string]string{"high": "max"},
		},
		{
			name:    "gemini camel-case effort leaves numeric budget and thought signature intact",
			format:  types.RelayFormatGemini,
			input:   `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"high","thinkingBudget":8192,"includeThoughts":false}},"contents":[{"parts":[{"text":"xhigh","thoughtSignature":"opaque"}]}]}`,
			want:    `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"low","thinkingBudget":8192,"includeThoughts":false}},"contents":[{"parts":[{"text":"xhigh","thoughtSignature":"opaque"}]}]}`,
			effort:  "low",
			mapping: map[string]string{"high": "low"},
		},
		{
			name:    "gemini both naming styles preserve their original member order",
			format:  types.RelayFormatGemini,
			input:   `{"generation_config":{"thinking_config":{"thinking_level":"high"}},"generationConfig":{"thinkingConfig":{"thinkingLevel":"medium"}}}`,
			want:    `{"generation_config":{"thinking_config":{"thinking_level":"low"}},"generationConfig":{"thinkingConfig":{"thinkingLevel":"high"}}}`,
			effort:  "high",
			mapping: map[string]string{"high": "low", "medium": "high"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)
			mapping := tc.mapping
			if mapping == nil {
				mapping = map[string]string{"xhigh": "max", "max": "low"}
			}
			info := &RelayInfo{
				RelayFormat: tc.format,
				ChannelMeta: &ChannelMeta{ChannelSetting: dto.ChannelSettings{
					ReasoningEffortMapping: mapping,
				}},
			}
			got, err := ApplyReasoningEffortMapping(input, info)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
			assert.Equal(t, tc.input, string(input), "retry input must remain unchanged")
			assert.Equal(t, tc.effort, info.ReasoningEffort)
		})
	}
}

func TestReasoningEffortMappingNoOpCases(t *testing.T) {
	input := []byte(` { "reasoning_effort":"xhigh", "reasoning":{"effort":123,"enabled":true,"max_tokens":8192}, "thinking":{"type":"xhigh","budget_tokens":8192} } `)
	for _, tc := range []struct {
		name     string
		format   types.RelayFormat
		settings dto.ChannelSettings
	}{
		{"empty mappings", types.RelayFormatOpenAI, dto.ChannelSettings{}},
		{"unmatched exact value", types.RelayFormatOpenAI, dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"HIGH": "max"}}},
		{"same value", types.RelayFormatOpenAI, dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"xhigh": "xhigh"}}},
		{"empty legacy target ignored", types.RelayFormatOpenAI, dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"xhigh": ""}}},
		{"passthrough priority", types.RelayFormatOpenAI, dto.ChannelSettings{PassThroughBodyEnabled: true, ReasoningEffortMapping: map[string]string{"xhigh": "max"}}},
		{"responses never modifies numeric effort or chat fields", types.RelayFormatOpenAIResponses, dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"123": "max", "xhigh": "max", "8192": "max", "true": "max"}}},
		{"claude never treats thinking type or budget as effort", types.RelayFormatClaude, dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"xhigh": "max", "8192": "max"}}},
		{"unsupported request format", types.RelayFormatOpenAIImage, dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"xhigh": "max"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &RelayInfo{RelayFormat: tc.format, ReasoningEffort: "original", ChannelMeta: &ChannelMeta{ChannelSetting: tc.settings}}
			got, err := ApplyReasoningEffortMapping(input, info)
			require.NoError(t, err)
			assert.Equal(t, input, got)
			assert.Equal(t, "original", info.ReasoningEffort)
		})
	}
	for _, info := range []*RelayInfo{nil, {RelayFormat: types.RelayFormatOpenAI}} {
		got, err := ApplyReasoningEffortMapping(input, info)
		require.NoError(t, err)
		assert.Equal(t, input, got)
	}
}

func TestReasoningEffortMappingUsesFinalProtocolAfterOverride(t *testing.T) {
	input := []byte(`{"reasoning_effort":"xhigh","reasoning":{"effort":"medium","summary":"auto"},"input":[]}`)
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"xhigh": "max", "high": "low"}},
			ParamOverride:  map[string]any{"reasoning": map[string]any{"effort": "high", "summary": "auto"}},
		},
	}
	overridden, err := ApplyParamOverrideWithRelayInfo(input, info)
	require.NoError(t, err)
	body, closer, err := NewChannelJSONBody(overridden, info)
	require.NoError(t, err)
	defer closer.Close()
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"reasoning_effort":"xhigh","reasoning":{"effort":"low","summary":"auto"},"input":[]}`, string(got))
	assert.Equal(t, "low", info.ReasoningEffort)
	assert.Equal(t, `{"reasoning_effort":"xhigh","reasoning":{"effort":"medium","summary":"auto"},"input":[]}`, string(input))
}

func TestReasoningEffortMappingEncodesTargetsAndIsolatesRetryChannels(t *testing.T) {
	input := []byte(`{"reasoning":{"effort":"xhigh"},"input":[{"type":"reasoning","encrypted_content":"UNCHANGED"}]}`)
	originalRequest := &dto.OpenAIResponsesRequest{Reasoning: &dto.Reasoning{Effort: "xhigh"}}
	info := &RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, Request: originalRequest, ChannelMeta: &ChannelMeta{
		ChannelSetting: dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"xhigh": "quoted\"/值"}},
	}}
	first, err := ApplyReasoningEffortMapping(input, info)
	require.NoError(t, err)
	assert.JSONEq(t, `{"reasoning":{"effort":"quoted\"/值"},"input":[{"type":"reasoning","encrypted_content":"UNCHANGED"}]}`, string(first))
	info.ChannelSetting.ReasoningEffortMapping = map[string]string{"xhigh": "high"}
	second, err := ApplyReasoningEffortMapping(input, info)
	require.NoError(t, err)
	assert.Equal(t, `{"reasoning":{"effort":"high"},"input":[{"type":"reasoning","encrypted_content":"UNCHANGED"}]}`, string(second))
	assert.Equal(t, "xhigh", originalRequest.Reasoning.Effort)
	assert.Equal(t, `{"reasoning":{"effort":"xhigh"},"input":[{"type":"reasoning","encrypted_content":"UNCHANGED"}]}`, string(input))
}

func TestReasoningEffortMappingOllamaNativeThink(t *testing.T) {
	for _, tc := range []struct {
		name    string
		format  types.RelayFormat
		channel int
		input   string
		want    string
		effort  string
	}{
		{
			name: "native string strength", format: types.RelayFormatOpenAI, channel: constant.ChannelTypeOllama,
			input:  `{"think":"high","messages":[{"role":"assistant","thinking":"high"}],"reasoning_effort":"high"}`,
			want:   `{"think":"max","messages":[{"role":"assistant","thinking":"high"}],"reasoning_effort":"high"}`,
			effort: "max",
		},
		{
			name: "boolean thinking flag", format: types.RelayFormatOpenAI, channel: constant.ChannelTypeOllama,
			input: `{"think":true,"reasoning_effort":"high"}`, want: `{"think":true,"reasoning_effort":"high"}`,
			effort: "original",
		},
		{
			name: "boolean disabled flag", format: types.RelayFormatOpenAI, channel: constant.ChannelTypeOllama,
			input: `{"think":false}`, want: `{"think":false}`, effort: "original",
		},
		{
			name: "OpenAI channel keeps provider extension", format: types.RelayFormatOpenAI, channel: constant.ChannelTypeOpenAI,
			input: `{"think":"high","reasoning_effort":"high"}`, want: `{"think":"high","reasoning_effort":"max"}`, effort: "max",
		},
		{
			name: "Ollama Responses uses Responses protocol", format: types.RelayFormatOpenAIResponses, channel: constant.ChannelTypeOllama,
			input: `{"think":"high","reasoning":{"effort":"high"}}`, want: `{"think":"high","reasoning":{"effort":"max"}}`, effort: "max",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &RelayInfo{RelayFormat: tc.format, ReasoningEffort: "original", ChannelMeta: &ChannelMeta{
				ChannelType:    tc.channel,
				ChannelSetting: dto.ChannelSettings{ReasoningEffortMapping: map[string]string{"high": "max", "true": "max", "false": "max"}},
			}}
			input := []byte(tc.input)
			got, err := ApplyReasoningEffortMapping(input, info)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
			assert.Equal(t, tc.input, string(input))
			assert.Equal(t, tc.effort, info.ReasoningEffort)
		})
	}
}
