package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountTokenInputArrayMatchesConcatenatedText(t *testing.T) {
	model := "claude-3"

	require.Equal(t,
		CountTextToken("alphabetagamma", model),
		CountTokenInput([]string{"alpha", "beta", "gamma"}, model),
	)

	require.Equal(t,
		CountTextToken("alpha42true", model),
		CountTokenInput([]interface{}{"alpha", 42, true}, model),
	)
}
