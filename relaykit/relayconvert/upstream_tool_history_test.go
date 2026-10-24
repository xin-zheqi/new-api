package relayconvert

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeMessagesToOpenAIChatPreservesAssistantTextWithToolCalls(t *testing.T) {
	text := "I will look that up."
	request := dto.ClaudeRequest{
		Model: "claude-test",
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{Type: "text", Text: &text},
					{
						Type:  "tool_use",
						Id:    "call_1",
						Name:  "lookup",
						Input: map[string]any{"query": "weather"},
					},
				},
			},
		},
	}

	result, err := ConvertRequest(
		context.Background(),
		&convmeta.Values{},
		types.RelayFormatOpenAI,
		&request,
	)
	require.NoError(t, err)

	converted, ok := result.Value.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, converted.Messages, 1)

	assistant := converted.Messages[0]
	assert.Equal(t, "assistant", assistant.Role)
	toolCalls := assistant.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_1", toolCalls[0].ID)
	require.Len(t, assistant.ParseContent(), 1)
	assert.Equal(t, "text", assistant.ParseContent()[0].Type)
	assert.Equal(t, text, assistant.ParseContent()[0].Text)
}

func TestClaudeMessagesToOpenAIChatKeepsToolOnlyContentEmpty(t *testing.T) {
	request := dto.ClaudeRequest{
		Model: "claude-test",
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{
						Type:  "tool_use",
						Id:    "call_1",
						Name:  "lookup",
						Input: map[string]any{"query": "weather"},
					},
				},
			},
		},
	}

	result, err := ConvertRequest(
		context.Background(),
		&convmeta.Values{},
		types.RelayFormatOpenAI,
		&request,
	)
	require.NoError(t, err)

	converted, ok := result.Value.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, converted.Messages, 1)

	assistant := converted.Messages[0]
	assert.Empty(t, assistant.ParseContent())
	toolCalls := assistant.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_1", toolCalls[0].ID)
}
