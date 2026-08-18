package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldDisableChannelIgnoresFirstResponseTimeoutByDefault(t *testing.T) {
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
	require.False(t, ShouldDisableChannel(firstResponseTimeout))

	invalidKey := types.NewOpenAIError(
		errors.New("invalid key"),
		types.ErrorCodeChannelInvalidKey,
		http.StatusUnauthorized,
	)
	require.True(t, ShouldDisableChannel(invalidKey))
}

func TestShouldDisableChannelIgnoresCyberPolicy(t *testing.T) {
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	cyberPolicy := types.NewOpenAIError(
		errors.New("blocked by network policy"),
		types.ErrorCodeCyberPolicy,
		http.StatusBadRequest,
	)
	require.False(t, ShouldDisableChannel(cyberPolicy))
}

func TestShouldDisableFirstResponseTimeoutChannelRequiresChannelSettingFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.False(t, ShouldDisableFirstResponseTimeoutChannel(ctx))

	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	require.False(t, ShouldDisableFirstResponseTimeoutChannel(ctx))

	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		FirstResponseTimeoutAutoBan: true,
	})
	require.True(t, ShouldDisableFirstResponseTimeoutChannel(ctx))
}

func TestShouldDisableFirstResponseTimeoutChannelRequiresGlobalSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = false
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		FirstResponseTimeoutAutoBan: true,
	})

	require.False(t, ShouldDisableFirstResponseTimeoutChannel(ctx))
}
