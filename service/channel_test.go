package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestShouldDisableChannelTreatsFirstResponseTimeoutAsChannelError(t *testing.T) {
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	firstResponseTimeout := types.NewOpenAIError(
		errors.New("upstream first response timeout after 30s"),
		types.ErrorCodeChannelFirstResponseTimeout,
		http.StatusBadGateway,
	)
	require.True(t, ShouldDisableChannel(firstResponseTimeout))

	invalidKey := types.NewOpenAIError(
		errors.New("invalid key"),
		types.ErrorCodeChannelInvalidKey,
		http.StatusUnauthorized,
	)
	require.True(t, ShouldDisableChannel(invalidKey))
}
