package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromptSensitiveCheckFollowsEffectiveWordsAndSwitches(t *testing.T) {
	oldEnabled, oldPrompt, oldWords := CheckSensitiveEnabled, CheckSensitiveOnPromptEnabled, SensitiveWords
	t.Cleanup(func() {
		CheckSensitiveEnabled, CheckSensitiveOnPromptEnabled, SensitiveWords = oldEnabled, oldPrompt, oldWords
	})
	CheckSensitiveEnabled, CheckSensitiveOnPromptEnabled = true, true
	SensitiveWordsFromString(" \n\t\n")
	assert.False(t, ShouldCheckPromptSensitive(), "an empty dictionary must not request prompt extraction")
	SensitiveWordsFromString("secret\n")
	assert.True(t, ShouldCheckPromptSensitive(), "newly configured words must take effect immediately")
	CheckSensitiveOnPromptEnabled = false
	assert.False(t, ShouldCheckPromptSensitive())
	CheckSensitiveOnPromptEnabled, CheckSensitiveEnabled = true, false
	assert.False(t, ShouldCheckPromptSensitive())
	CheckSensitiveEnabled = true
	SensitiveWordsFromString("")
	assert.False(t, ShouldCheckPromptSensitive(), "clearing the dictionary must remove prompt scanning")
}
