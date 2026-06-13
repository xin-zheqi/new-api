package common

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstResponseTimeoutGuardCancelsContext(t *testing.T) {
	t.Parallel()

	guard := &FirstResponseTimeoutGuard{
		timeout: 20 * time.Millisecond,
		started: time.Now(),
	}
	ctx, cancel := guard.WithCancel(context.Background())
	defer cancel()

	select {
	case <-ctx.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first response timeout guard did not cancel context")
	}

	assert.True(t, guard.IsTimedOut())
}

func TestFirstResponseTimeoutGuardMarkContentStopsTimer(t *testing.T) {
	t.Parallel()

	guard := &FirstResponseTimeoutGuard{
		timeout: 20 * time.Millisecond,
		started: time.Now(),
	}
	ctx, cancel := guard.WithCancel(context.Background())
	defer cancel()

	guard.MarkContent()
	time.Sleep(60 * time.Millisecond)

	assert.False(t, guard.IsTimedOut())
	select {
	case <-ctx.Done():
		t.Fatal("context should remain active after first content is marked")
	default:
	}
}

func TestRelayInfoStartFirstResponseTimeoutGuardResetsPerAttempt(t *testing.T) {
	t.Parallel()

	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				FirstResponseTimeoutSeconds: 1,
			},
		},
	}

	first := info.StartFirstResponseTimeoutGuard()
	require.NotNil(t, first)
	first.timeout = 20 * time.Millisecond
	ctx, cancel := first.WithCancel(context.Background())
	defer cancel()

	select {
	case <-ctx.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first attempt guard did not time out")
	}
	require.True(t, first.IsTimedOut())

	second := info.StartFirstResponseTimeoutGuard()
	require.NotNil(t, second)
	require.NotSame(t, first, second)
	assert.False(t, second.IsTimedOut(), "retry attempt must not inherit prior timeout state")
	assert.Less(t, second.Elapsed(), 200*time.Millisecond, "retry attempt timer should start from the new upstream request")

	info.ChannelSetting.FirstResponseTimeoutSeconds = 0
	assert.Nil(t, info.StartFirstResponseTimeoutGuard())
	assert.Nil(t, info.FirstResponseTimeoutGuard)
}

func TestNewFirstResponseTimeoutError(t *testing.T) {
	t.Parallel()

	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				FirstResponseTimeoutSeconds: 30,
			},
		},
	}

	err := NewFirstResponseTimeoutError(info)
	require.NotNil(t, err)

	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.Equal(t, types.ErrorCodeChannelFirstResponseTimeout, err.GetErrorCode())
	assert.Equal(t, types.ErrorCodeChannelFirstResponseTimeout, err.ToOpenAIError().Code)
	assert.Contains(t, err.Error(), "upstream first response timeout after 30s")
}
