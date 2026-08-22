package claude

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestClaudeCodeMimicAddsHeadersAndBodyTraits(t *testing.T) {
	request := &dto.ClaudeRequest{Model: "claude-sonnet", Messages: []dto.ClaudeMessage{{Role: "user", Content: "hi"}}}
	applyClaudeCodeMimicBody(request)
	require.Contains(t, string(request.Metadata), "device_id")
	require.Contains(t, string(request.Metadata), "session_id")
	require.Contains(t, request.System.([]dto.ClaudeMediaMessage)[0].GetText(), "Claude Code")

	header := http.Header{}
	applyClaudeCodeMimicHeaders(&header)
	require.Equal(t, "claude-cli/2.1.220 (external, cli)", header.Get("User-Agent"))
	require.Contains(t, header.Get("anthropic-beta"), "claude-code-20250219")
}

func TestClaudeCodeMimicAlsoAppliesAfterOpenAIConversion(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{ClaudeCodeMimic: true},
		},
	}
	request := &dto.GeneralOpenAIRequest{
		Model:    "claude-sonnet",
		Messages: []dto.Message{{Role: "user", Content: "hello"}},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
	require.NoError(t, err)
	claudeRequest, ok := converted.(*dto.ClaudeRequest)
	require.True(t, ok)
	require.Contains(t, string(claudeRequest.Metadata), "device_id")
	require.Contains(t, claudeRequest.System.([]dto.ClaudeMediaMessage)[0].GetText(), "Claude Code")
}
