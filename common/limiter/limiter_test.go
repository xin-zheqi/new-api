package limiter

import (
	"context"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisLimiterUsesProvidedClient(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"first server", "replacement server"} {
		t.Run(name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			limiter := New(ctx, client)
			allowed, err := limiter.Allow(ctx, "independent-bucket", WithCapacity(1), WithRate(0), WithRequested(1))
			require.NoError(t, err)
			assert.True(t, allowed, "each supplied Redis client owns its own bucket")
			allowed, err = limiter.Allow(ctx, "independent-bucket", WithCapacity(1), WithRate(0), WithRequested(1))
			require.NoError(t, err)
			assert.False(t, allowed)
		})
	}
}

func TestRedisLimiterRealScriptCacheRecovery(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not configured; this test flushes scripts in an isolated test Redis")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr})
	require.NoError(t, client.Ping(ctx).Err())
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	key := "test:limiter-script-recovery:" + common.GetUUID()
	t.Cleanup(func() { require.NoError(t, client.Del(ctx, key).Err()) })
	limiter := New(ctx, client)
	allowed, err := limiter.Allow(ctx, key, WithCapacity(2), WithRate(0), WithRequested(1))
	require.NoError(t, err)
	assert.True(t, allowed)
	// Redis loses the script cache after restart/failover. Flush only this
	// dedicated test server's script cache while retaining the bucket state.
	require.NoError(t, client.ScriptFlush(ctx).Err())
	allowed, err = limiter.Allow(ctx, key, WithCapacity(2), WithRate(0), WithRequested(1))
	require.NoError(t, err, "a cache miss must transparently reload the limiter script")
	assert.True(t, allowed)
	allowed, err = limiter.Allow(ctx, key, WithCapacity(2), WithRate(0), WithRequested(1))
	require.NoError(t, err)
	assert.False(t, allowed, "script reloading must not reset the existing bucket")
}
