package dto

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelSettingsValidateReasoningEffortMapping(t *testing.T) {
	tooMany := make(map[string]string, MaxReasoningEffortMappings+1)
	for i := 0; i <= MaxReasoningEffortMappings; i++ {
		tooMany[fmt.Sprintf("source-%d", i)] = "max"
	}
	for _, tc := range []struct {
		name    string
		mapping map[string]string
		valid   bool
	}{
		{"unset remains valid", nil, true},
		{"empty remains valid", map[string]string{}, true},
		{"private provider names allowed", map[string]string{"xhight": "max", "CUSTOM": "custom/value"}, true},
		{"64 unicode characters allowed", map[string]string{strings.Repeat("值", 64): "max"}, true},
		{"empty source rejected", map[string]string{"": "max"}, false},
		{"empty target rejected", map[string]string{"xhigh": ""}, false},
		{"source whitespace rejected", map[string]string{" xhigh": "max"}, false},
		{"target whitespace rejected", map[string]string{"xhigh": "max "}, false},
		{"control characters rejected", map[string]string{"xhigh": "ma\x00x"}, false},
		{"newline rejected", map[string]string{"xhigh\n": "max"}, false},
		{"invalid utf8 rejected", map[string]string{"xhigh": string([]byte{0xff})}, false},
		{"oversize target rejected", map[string]string{"xhigh": strings.Repeat("m", 65)}, false},
		{"too many entries rejected", tooMany, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := (&ChannelSettings{ReasoningEffortMapping: tc.mapping}).ValidateReasoningEffortMapping()
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "reasoning_effort_mapping")
		})
	}
	var absent *ChannelSettings
	require.NoError(t, absent.ValidateReasoningEffortMapping())
}
