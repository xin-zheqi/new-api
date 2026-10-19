package oairesponses

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesToClaudeMessagesPreservesNativeToolDefinitionsAndHistory(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Model: "claude-compatible",
		Input: []byte(`[
			{"type":"message","role":"user","content":"inspect the workspace"},
			{"type":"computer_call","id":"computer_1","call_id":"computer_1","action":{"type":"screenshot"}},
			{"type":"computer_call_output","call_id":"computer_1","output":{"type":"computer_screenshot","data":"..."}}
		]`),
		Tools: []byte(`[
			{"type":"computer_use_preview","display_width":1280,"display_height":720},
			{"type":"shell"},
			{"type":"apply_patch"},
			{"type":"web_search_preview"},
			{"type":"function","name":"lookup","parameters":{"type":"object","properties":{}}}
		]`),
		MaxOutputTokens: kitutil.GetPointer(uint(256)),
	}

	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), &convmeta.Values{}, request)
	require.NoError(t, err)
	require.Len(t, got.Tools, 5)
	body, err := kitutil.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"type":"computer_20250124"`)
	assert.Contains(t, string(body), `"type":"bash_20250124"`)
	assert.Contains(t, string(body), `"type":"text_editor_20250124"`)
	assert.Contains(t, string(body), `"type":"web_search_20250305"`)
	assert.Contains(t, string(body), `"name":"lookup"`)

	require.Len(t, got.Messages, 3)
	first, ok := got.Messages[0].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	assert.Equal(t, "user", got.Messages[0].Role)
	assert.Equal(t, "inspect the workspace", first[0].GetText())
	second, ok := got.Messages[1].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	assert.Equal(t, "assistant", got.Messages[1].Role)
	assert.Equal(t, "tool_use", second[0].Type)
	assert.Equal(t, "computer", second[0].Name)
	third, ok := got.Messages[2].Content.([]dto.ClaudeMediaMessage)
	require.True(t, ok)
	assert.Equal(t, "user", got.Messages[2].Role)
	assert.Equal(t, "tool_result", third[0].Type)
	assert.Equal(t, "computer_1", third[0].ToolUseId)
}
