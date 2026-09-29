package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/stretchr/testify/require"
)

func TestApplyThinkingConfigPreservesNativeCaseAndRecordsCanonicalEffort(t *testing.T) {
	request := &dto.GeminiChatRequest{
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ThinkingConfig: &dto.GeminiThinkingConfig{ThinkingLevel: "HIGH"},
		},
	}
	info := &convmeta.Values{UpstreamModelName: "gemini-3-pro", ChannelMetaAttached: true}

	require.NoError(t, ApplyThinkingConfig(request, info))
	require.NotNil(t, request.GenerationConfig.ThinkingConfig)
	require.Equal(t, "HIGH", request.GenerationConfig.ThinkingConfig.ThinkingLevel)
	require.Equal(t, "high", info.GetReasoningEffort())
}
