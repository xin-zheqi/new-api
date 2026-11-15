package middleware

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Observe only script commands; these tests issue requests sequentially. A
// rejected EVALSHA models an error before any Redis state can be changed.
type rateLimitScriptTrace struct {
	commands     []string
	evalSHAError error
}

func (trace *rateLimitScriptTrace) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == "eval" || cmd.Name() == "evalsha" {
		trace.commands = append(trace.commands, cmd.Name())
	}
	if cmd.Name() == "evalsha" {
		return ctx, trace.evalSHAError
	}
	return ctx, nil
}

func (*rateLimitScriptTrace) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (*rateLimitScriptTrace) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (*rateLimitScriptTrace) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

// TEST_REDIS_ADDR must point to an isolated server: recovery tests deliberately
// flush its script cache. The same contracts also run locally with miniredis.
func useRateLimitScriptRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		_, client := useRateLimitMiniRedis(t)
		return client
	}
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	require.NoError(t, client.Ping(context.Background()).Err())
	previousEnabled, previousClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		_ = client.Close() // Recovery cases deliberately close the original pool.
		common.RedisEnabled, common.RDB = previousEnabled, previousClient
	})
	return client
}

func rateLimitScriptRouter(t *testing.T, client *redis.Client, ticket bool) (http.Handler, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	mark := "SCRIPT-" + common.GetUUID()
	limit := rateLimitFactory(2, 37, mark)
	key := redisIPRateLimitKey(mark, "192.0.2.80")
	if ticket {
		limit = ticketRateLimitFactory(2, 37, mark)
		key = "rateLimit:" + mark + ":user:42"
	}
	// The client under test is deliberately replaced/closed in recovery cases.
	// Use an independent cleanup connection rather than retaining that client.
	options := *client.Options()
	t.Cleanup(func() {
		cleanup := redis.NewClient(&options)
		require.NoError(t, cleanup.Del(context.Background(), key).Err())
		require.NoError(t, cleanup.Close())
	})
	router.GET("/limited", func(c *gin.Context) { c.Set("id", 42) }, limit, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router, key
}

func TestRedisRateLimitScriptCacheRecoveryAndClientReplacement(t *testing.T) {
	for _, ticket := range []bool{false, true} {
		name := "fixed window"
		if ticket {
			name = "ticket"
		}
		t.Run(name, func(t *testing.T) {
			client := useRateLimitScriptRedis(t)
			ctx := context.Background()
			require.NoError(t, client.ScriptFlush(ctx).Err())
			trace := &rateLimitScriptTrace{}
			client.AddHook(trace)
			router, key := rateLimitScriptRouter(t, client, ticket)

			assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "192.0.2.80:12345").Code)
			assert.Equal(t, []string{"evalsha", "eval"}, trace.commands, "cold scripts must load transparently")
			count, err := client.Get(ctx, key).Result()
			require.NoError(t, err)
			assert.Equal(t, "1", count)
			ttl, err := client.TTL(ctx, key).Result()
			require.NoError(t, err)
			assert.InDelta(t, 37, ttl.Seconds(), 2)

			// Set an already-running window deterministically instead of waiting
			// for wall time to elapse on the real Redis fixture.
			require.NoError(t, client.Expire(ctx, key, 32*time.Second).Err())
			trace.commands = nil
			assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "192.0.2.80:12345").Code)
			assert.Equal(t, []string{"evalsha"}, trace.commands, "warm requests should not send the script body")
			ttl, err = client.TTL(ctx, key).Result()
			require.NoError(t, err)
			assert.InDelta(t, 32, ttl.Seconds(), 2, "a warm request must not extend the window")

			require.NoError(t, client.ScriptFlush(context.Background()).Err())
			trace.commands = nil
			assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/limited", "192.0.2.80:12345").Code)
			assert.Equal(t, []string{"evalsha", "eval"}, trace.commands)
			count, err = client.Get(ctx, key).Result()
			require.NoError(t, err)
			assert.Equal(t, "3", count, "NOSCRIPT must not increment before the EVAL fallback")
			ttl, err = client.TTL(ctx, key).Result()
			require.NoError(t, err)
			assert.InDelta(t, 32, ttl.Seconds(), 2, "script reload must preserve the existing window")

			// A client replacement must use its live pool while preserving the
			// server's count and window, rather than retaining the closed client.
			require.NoError(t, client.Close())
			options := *client.Options()
			replacement := redis.NewClient(&options)
			t.Cleanup(func() { require.NoError(t, replacement.Close()) })
			common.RDB = replacement
			replacementTrace := &rateLimitScriptTrace{}
			replacement.AddHook(replacementTrace)
			assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/limited", "192.0.2.80:12345").Code)
			assert.Equal(t, []string{"evalsha"}, replacementTrace.commands)
			count, err = replacement.Get(ctx, key).Result()
			require.NoError(t, err)
			assert.Equal(t, "4", count)
			ttl, err = replacement.TTL(ctx, key).Result()
			require.NoError(t, err)
			assert.InDelta(t, 32, ttl.Seconds(), 2)
			if !ticket {
				require.NoError(t, replacement.Persist(ctx, key).Err())
				assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/limited", "192.0.2.80:12345").Code)
				count, err = replacement.Get(ctx, key).Result()
				require.NoError(t, err)
				assert.Equal(t, "5", count)
				ttl, err = replacement.TTL(ctx, key).Result()
				require.NoError(t, err)
				assert.InDelta(t, 37, ttl.Seconds(), 2, "fixed windows must still repair a lost TTL")
			}
		})
	}
}

func TestRedisRateLimitDoesNotFallbackOnOtherScriptErrors(t *testing.T) {
	for _, ticket := range []bool{false, true} {
		name := "fixed window"
		if ticket {
			name = "ticket"
		}
		t.Run(name, func(t *testing.T) {
			client := useRateLimitScriptRedis(t)
			ctx := context.Background()
			trace := &rateLimitScriptTrace{evalSHAError: errors.New("NOPERM EVALSHA is not allowed")}
			router, key := rateLimitScriptRouter(t, client, ticket)
			if os.Getenv("TEST_REDIS_ADDR") != "" {
				username, password := "rate-limit-script-"+common.GetUUID(), common.GetUUID()
				require.NoError(t, client.Do(ctx, "ACL", "SETUSER", username, "reset", "on", ">"+password, "allkeys", "allcommands", "-evalsha").Err())
				t.Cleanup(func() { require.NoError(t, client.Do(ctx, "ACL", "DELUSER", username).Err()) })
				options := *client.Options()
				options.Username, options.Password = username, password
				restricted := redis.NewClient(&options)
				t.Cleanup(func() { require.NoError(t, restricted.Close()) })
				require.NoError(t, restricted.Ping(ctx).Err())
				common.RDB = restricted
				trace.evalSHAError = nil // Exercise the real server's ACL error.
				restricted.AddHook(trace)
				t.Cleanup(func() { common.RDB = client })
			} else {
				client.AddHook(trace)
			}

			response := performRateLimitRequest(router, "/limited", "192.0.2.80:12345")
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, []string{"evalsha"}, trace.commands, "only NOSCRIPT permits the script fallback")
			exists, err := client.Exists(ctx, key).Result()
			require.NoError(t, err)
			assert.Zero(t, exists, "another error must not cause a hidden EVAL mutation")

			common.RDB = client
			trace.evalSHAError = nil
			trace.commands = nil
			assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "192.0.2.80:12345").Code)
			count, err := client.Get(ctx, key).Result()
			require.NoError(t, err)
			assert.Equal(t, "1", count, "the failed request must not consume an allowance")
		})
	}
}
