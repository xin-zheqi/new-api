package model_setting

import (
	"net/http"
	"testing"
)

func TestClaudeSettingsWriteHeadersMergesConfiguredValuesIntoSingleHeader(t *testing.T) {
	settings := &ClaudeSettings{
		HeadersSettings: map[string]map[string][]string{
			"claude-3-7-sonnet-20250219-thinking": {
				"anthropic-beta": {
					"token-efficient-tools-2025-02-19",
				},
			},
		},
	}

	headers := http.Header{}
	headers.Set("anthropic-beta", "output-128k-2025-02-19")

	settings.WriteHeaders("claude-3-7-sonnet-20250219-thinking", &headers)

	got := headers.Values("anthropic-beta")
	if len(got) != 1 {
		t.Fatalf("expected a single merged header value, got %v", got)
	}
	expected := "output-128k-2025-02-19,token-efficient-tools-2025-02-19"
	if got[0] != expected {
		t.Fatalf("expected merged header %q, got %q", expected, got[0])
	}
}

func TestClaudeSettingsWriteHeadersDeduplicatesAcrossCommaSeparatedAndRepeatedValues(t *testing.T) {
	settings := &ClaudeSettings{
		HeadersSettings: map[string]map[string][]string{
			"claude-3-7-sonnet-20250219-thinking": {
				"anthropic-beta": {
					"token-efficient-tools-2025-02-19",
					"computer-use-2025-01-24",
				},
			},
		},
	}

	headers := http.Header{}
	headers.Add("anthropic-beta", "output-128k-2025-02-19, token-efficient-tools-2025-02-19")
	headers.Add("anthropic-beta", "token-efficient-tools-2025-02-19")

	settings.WriteHeaders("claude-3-7-sonnet-20250219-thinking", &headers)

	got := headers.Values("anthropic-beta")
	if len(got) != 1 {
		t.Fatalf("expected duplicate values to collapse into one header, got %v", got)
	}
	expected := "output-128k-2025-02-19,token-efficient-tools-2025-02-19,computer-use-2025-01-24"
	if got[0] != expected {
		t.Fatalf("expected deduplicated merged header %q, got %q", expected, got[0])
	}
}

func TestClaudeSettingsShouldApplyThinkingSignatureCompatibilityByChannel(t *testing.T) {
	settings := &ClaudeSettings{
		ThinkingSignatureCompatibilityPolicy: ChatCompletionsToResponsesPolicy{
			Enabled:     true,
			AllChannels: false,
			ChannelIDs:  []int{12},
		},
	}

	if !settings.ShouldApplyThinkingSignatureCompatibility(12, 0, "claude-sonnet-4-20250514") {
		t.Fatal("expected configured channel id to enable compatibility")
	}
	if settings.ShouldApplyThinkingSignatureCompatibility(13, 0, "claude-sonnet-4-20250514") {
		t.Fatal("expected unconfigured channel id to be disabled")
	}
}

func TestClaudeSettingsShouldApplyThinkingSignatureCompatibilityRespectsModelPatterns(t *testing.T) {
	settings := &ClaudeSettings{
		ThinkingSignatureCompatibilityPolicy: ChatCompletionsToResponsesPolicy{
			Enabled:       true,
			AllChannels:   true,
			ModelPatterns: []string{"^claude-.*$"},
		},
	}

	if !settings.ShouldApplyThinkingSignatureCompatibility(0, 0, "claude-sonnet-4-20250514") {
		t.Fatal("expected matching model pattern to enable compatibility")
	}
	if settings.ShouldApplyThinkingSignatureCompatibility(0, 0, "gpt-4o") {
		t.Fatal("expected non-matching model pattern to disable compatibility")
	}
}
