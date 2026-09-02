package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeRequestRemoveThinkingBlocksFromMessagesRemovesOnlyThinkingBlocks(t *testing.T) {
	request := ClaudeRequest{
		Messages: []ClaudeMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type":      "thinking",
						"thinking":  "private chain",
						"signature": "stale",
					},
					map[string]any{
						"type": "text",
						"text": "visible answer",
					},
					map[string]any{
						"type": "redacted_thinking",
						"data": "redacted",
					},
					map[string]any{
						"type":  "tool_use",
						"id":    "toolu_1",
						"name":  "lookup",
						"input": map[string]any{"q": "test"},
					},
				},
			},
			{
				Role:    "user",
				Content: "next question",
			},
		},
	}

	assert.Equal(t, 2, request.RemoveThinkingBlocksFromMessages())
	content, ok := request.Messages[0].Content.([]any)
	require.True(t, ok, "expected []any content, got %T", request.Messages[0].Content)
	require.Len(t, content, 2)
	assert.Equal(t, "text", claudeBlockType(content[0]))
	assert.Equal(t, "tool_use", claudeBlockType(content[1]))
	assert.Equal(t, "next question", request.Messages[1].Content)
}

func TestClaudeRequestRemoveThinkingBlocksFromMessagesHandlesDecodedClaudeJSON(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "private", "signature": "bad"},
					{"type": "text", "text": "hello"}
				]
			}
		]
	}`)
	var request ClaudeRequest
	require.NoError(t, kitutil.Unmarshal(body, &request))

	assert.Equal(t, 1, request.RemoveThinkingBlocksFromMessages())
	content, ok := request.Messages[0].Content.([]any)
	require.True(t, ok, "expected []any content, got %T", request.Messages[0].Content)
	require.Len(t, content, 1)
	assert.Equal(t, "text", claudeBlockType(content[0]))
}

func claudeBlockType(item any) string {
	block, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	value, _ := block["type"].(string)
	return value
}
