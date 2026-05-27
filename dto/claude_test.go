package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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

	removed := request.RemoveThinkingBlocksFromMessages()
	if removed != 2 {
		t.Fatalf("expected 2 thinking blocks removed, got %d", removed)
	}

	content, ok := request.Messages[0].Content.([]any)
	if !ok {
		t.Fatalf("expected []any content, got %T", request.Messages[0].Content)
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks left, got %d", len(content))
	}
	if blockType(content[0]) != "text" {
		t.Fatalf("expected text block to be preserved, got %q", blockType(content[0]))
	}
	if blockType(content[1]) != "tool_use" {
		t.Fatalf("expected tool_use block to be preserved, got %q", blockType(content[1]))
	}
	if request.Messages[1].Content != "next question" {
		t.Fatalf("expected string content to be unchanged, got %#v", request.Messages[1].Content)
	}
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
	if err := common.Unmarshal(body, &request); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	removed := request.RemoveThinkingBlocksFromMessages()
	if removed != 1 {
		t.Fatalf("expected 1 thinking block removed, got %d", removed)
	}

	content, ok := request.Messages[0].Content.([]any)
	if !ok {
		t.Fatalf("expected []any content, got %T", request.Messages[0].Content)
	}
	if len(content) != 1 || blockType(content[0]) != "text" {
		t.Fatalf("expected only text block to remain, got %#v", content)
	}
}

func blockType(item any) string {
	block, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	value, _ := block["type"].(string)
	return value
}
