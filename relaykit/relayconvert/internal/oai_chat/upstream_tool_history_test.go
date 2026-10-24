package oaichat

import (
	"context"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOpenAIChatRequestToClaudeMessagesMergesAdjacentAssistantToolCalls(t *testing.T) {
	toolCall := func(id string) []dto.ToolCallRequest {
		return []dto.ToolCallRequest{{
			ID:       id,
			Type:     "function",
			Function: dto.FunctionRequest{Name: "lookup_" + id, Arguments: `{"q":"` + id + `"}`},
		}}
	}
	assistant := func(content any, toolCalls []dto.ToolCallRequest) dto.Message {
		message := dto.Message{Role: "assistant", Content: content}
		if toolCalls != nil {
			message.SetToolCalls(toolCalls)
		}
		return message
	}

	tests := []struct {
		name         string
		messages     []dto.Message
		wantTexts    []string
		wantToolUses []string
	}{
		{
			name: "both assistants have tool calls",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a and b."},
				assistant("", toolCall("call_a")),
				assistant("", toolCall("call_b")),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
				{Role: "tool", ToolCallId: "call_b", Content: "result b"},
			},
			wantTexts:    []string{"..."},
			wantToolUses: []string{"call_a", "call_b"},
		},
		{
			name: "tool calls followed by plain text",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a."},
				assistant(nil, toolCall("call_a")),
				assistant("Checking a.", nil),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
			},
			wantTexts:    []string{"Checking a."},
			wantToolUses: []string{"call_a"},
		},
		{
			name: "literal ellipsis with tool calls is preserved",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a and b."},
				assistant("...", toolCall("call_a")),
				assistant("Checking b.", toolCall("call_b")),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
				{Role: "tool", ToolCallId: "call_b", Content: "result b"},
			},
			wantTexts:    []string{"... Checking b."},
			wantToolUses: []string{"call_a", "call_b"},
		},
		{
			name: "literal ellipsis replacing a placeholder survives the next merge",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a."},
				assistant(nil, toolCall("call_a")),
				assistant("...", nil),
				assistant("Checking a.", nil),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
			},
			wantTexts:    []string{"... Checking a."},
			wantToolUses: []string{"call_a"},
		},
		{
			name: "literal ellipsis before tool calls survives repeated merges",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a."},
				assistant("...", nil),
				assistant("", toolCall("call_a")),
				assistant("Checking a.", nil),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
			},
			wantTexts:    []string{"... Checking a."},
			wantToolUses: []string{"call_a"},
		},
		{
			name: "three tool-only assistants keep one placeholder",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a, b and c."},
				assistant(nil, toolCall("call_a")),
				assistant("", toolCall("call_b")),
				assistant("", toolCall("call_c")),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
				{Role: "tool", ToolCallId: "call_b", Content: "result b"},
				{Role: "tool", ToolCallId: "call_c", Content: "result c"},
			},
			wantTexts:    []string{"..."},
			wantToolUses: []string{"call_a", "call_b", "call_c"},
		},
		{
			name: "both assistants are plain text",
			messages: []dto.Message{
				{Role: "user", Content: "Hello."},
				assistant("First.", nil),
				assistant("Second.", nil),
			},
			wantTexts: []string{"First. Second."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxTokens := uint(1024)
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:     "claude-test",
				MaxTokens: &maxTokens,
				Messages:  tt.messages,
			})
			require.NoError(t, err)

			var wantToolResults []dto.ClaudeMediaMessage
			for _, message := range tt.messages {
				if message.Role == "tool" {
					wantToolResults = append(wantToolResults, dto.ClaudeMediaMessage{
						Type:      "tool_result",
						ToolUseId: message.ToolCallId,
						Content:   message.Content,
					})
				}
			}
			if len(wantToolResults) > 0 {
				require.Len(t, got.Messages, 3)
				assert.Equal(t, "user", got.Messages[2].Role)
				assert.Equal(t, wantToolResults, got.Messages[2].Content)
			} else {
				require.Len(t, got.Messages, 2)
			}
			assert.Equal(t, "assistant", got.Messages[1].Role)

			var texts, toolUses []string
			switch content := got.Messages[1].Content.(type) {
			case string:
				texts = append(texts, content)
			case []dto.ClaudeMediaMessage:
				for _, block := range content {
					switch block.Type {
					case "text":
						texts = append(texts, block.GetText())
					case "tool_use":
						toolUses = append(toolUses, block.Id)
						assert.Equal(t, "lookup_"+block.Id, block.Name)
						assert.Equal(t, map[string]any{"q": block.Id}, block.Input)
					}
				}
			default:
				require.Failf(t, "unexpected assistant content type", "%T", content)
			}
			assert.Equal(t, tt.wantTexts, texts)
			assert.Equal(t, tt.wantToolUses, toolUses)
		})
	}
}
