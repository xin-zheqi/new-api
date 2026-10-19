package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2, 60)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

func TestModelRedisSuccessRecordingPreservesWindowAndDisabledLimits(t *testing.T) {
	server, client := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := "rateLimit:success-window"
	older := "2020-01-01T00:00:00.000Z"
	newer := "2020-01-01T00:00:01.000Z"
	_, err := server.Push(key, newer, older)
	require.NoError(t, err)
	recordRedisRequest(ctx, client, key, 2, 60)
	values, err := client.LRange(ctx, key, 0, -1).Result()
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, newer, values[1], "trim the oldest entry while preserving newest-first order")
	assert.Equal(t, time.Minute, server.TTL(key))
	allowed, err := checkRedisRateLimit(ctx, client, key, 2, 60)
	require.NoError(t, err)
	assert.True(t, allowed, "an expired oldest entry must not block a request")
	recordRedisRequest(ctx, client, key, 2, 60)
	allowed, err = checkRedisRateLimit(ctx, client, key, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "a full current window must still be limited")
	recordRedisRequest(ctx, client, "disabled", 0, 60)
	assert.False(t, server.Exists("disabled"))
}
