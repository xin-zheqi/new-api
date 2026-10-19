package middleware

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Interrupt recording either before EXEC is sent or after every server reply
// has arrived. The latter models a lost acknowledgement after Redis executed.
type redisRecordingFaultConn struct {
	net.Conn
	target     []byte
	dropped    *atomic.Bool
	beforeExec bool
	replies    int
}

func (c *redisRecordingFaultConn) Write(p []byte) (int, error) {
	if !c.dropped.Load() && bytes.Contains(p, c.target) && bytes.Contains(p, []byte("\r\nlpush\r\n")) {
		if c.beforeExec {
			execOffset := bytes.Index(p, []byte("*1\r\n$4\r\nexec\r\n"))
			if execOffset >= 0 && c.dropped.CompareAndSwap(false, true) {
				if _, err := c.Conn.Write(p[:execOffset]); err != nil {
					return 0, err
				}
				_ = c.Conn.Close()
				return len(p), nil // Connection loss is observed when preparing to read.
			}
			return c.Conn.Write(p)
		}
		c.replies = 3
		if bytes.Contains(p, []byte("\r\nmulti\r\n")) {
			c.replies = 5 // MULTI, three QUEUED replies, then the EXEC array.
		}
	}
	return c.Conn.Write(p)
}

func (c *redisRecordingFaultConn) Read(p []byte) (int, error) {
	if c.replies > 0 && c.dropped.CompareAndSwap(false, true) {
		reader := bufio.NewReader(c.Conn)
		for range c.replies {
			if err := discardRedisFaultReply(reader); err != nil {
				return 0, fmt.Errorf("fault fixture could not drain server acknowledgement: %w", err)
			}
		}
		_ = c.Conn.Close()
		return 0, io.EOF
	}
	return c.Conn.Read(p)
}

// Decode only the RESP2 reply framing needed to await actual server execution.
func discardRedisFaultReply(reader *bufio.Reader) error {
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	switch line[0] {
	case '+', '-', ':':
		return nil
	case '*':
		count, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return err
		}
		for range count {
			if err := discardRedisFaultReply(reader); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unexpected Redis reply: %q", line)
	}
}

func TestModelRateLimitRealRedis(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not configured; use an isolated test Redis")
	}
	ctx := context.Background()
	control := redis.NewClient(&redis.Options{Addr: addr})
	require.NoError(t, control.Ping(ctx).Err())
	t.Cleanup(func() { require.NoError(t, control.Close()) })
	info, err := control.Info(ctx, "server").Result()
	require.NoError(t, err)
	for line := range strings.SplitSeq(info, "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			t.Log(strings.TrimSpace(line))
		}
	}
	key := "test:model-rate-limit:" + common.GetUUID()
	t.Cleanup(func() { require.NoError(t, control.Del(ctx, key, key+":disabled", key+":wrongtype").Err()) })

	t.Run("successful recording and window", func(t *testing.T) {
		require.NoError(t, control.RPush(ctx, key, "2020-01-01T00:00:01.000Z", "2020-01-01T00:00:00.000Z").Err())
		require.NoError(t, recordRedisRequest(ctx, control, key, 2, 60))
		values, err := control.LRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		require.Len(t, values, 2)
		assert.Equal(t, "2020-01-01T00:00:01.000Z", values[1])
		ttl, err := control.TTL(ctx, key).Result()
		require.NoError(t, err)
		assert.Positive(t, ttl)
		assert.LessOrEqual(t, ttl, time.Minute)
		allowed, err := checkRedisRateLimit(ctx, control, key, 2, 60)
		require.NoError(t, err)
		assert.True(t, allowed)
		require.NoError(t, recordRedisRequest(ctx, control, key, 2, 60))
		allowed, err = checkRedisRateLimit(ctx, control, key, 2, 60)
		require.NoError(t, err)
		assert.False(t, allowed)
		require.NoError(t, recordRedisRequest(ctx, control, key+":disabled", 0, 60))
		exists, err := control.Exists(ctx, key+":disabled").Result()
		require.NoError(t, err)
		assert.Zero(t, exists)
	})

	for _, tc := range []struct {
		name       string
		beforeExec bool
		wantCount  int64
		wantErr    error
	}{
		{name: "lost acknowledgement does not double count", wantCount: 1, wantErr: io.EOF},
		{name: "disconnect before EXEC leaves no partial record", beforeExec: true, wantErr: net.ErrClosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, control.Del(ctx, key).Err())
			var dropped atomic.Bool
			client := redis.NewClient(&redis.Options{
				Addr: addr,
				Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
					conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
					if err != nil {
						return nil, err
					}
					return &redisRecordingFaultConn{Conn: conn, target: []byte(key), dropped: &dropped, beforeExec: tc.beforeExec}, nil
				},
			})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			require.NoError(t, client.Ping(ctx).Err())
			err := recordRedisRequest(ctx, client, key, 10, 60)
			assert.True(t, dropped.Load(), "the fault must actually interrupt recording")
			assert.ErrorIs(t, err, tc.wantErr, "an uncertain write must be reported, not replayed blindly")
			count, err := control.LLen(ctx, key).Result()
			require.NoError(t, err)
			assert.Equal(t, tc.wantCount, count, "recording must not be duplicated or partially executed")
			if !tc.beforeExec {
				ttl, err := control.TTL(ctx, key).Result()
				require.NoError(t, err)
				assert.Positive(t, ttl, "the executed transaction includes expiry even if its reply is lost")
			} else {
				exists, err := control.Exists(ctx, key).Result()
				require.NoError(t, err)
				assert.Zero(t, exists)
			}
			// A new request can reconnect; the failed read must not poison the pool.
			require.NoError(t, recordRedisRequest(ctx, client, key, 10, 60))
			count, err = control.LLen(ctx, key).Result()
			require.NoError(t, err)
			assert.Equal(t, tc.wantCount+1, count)
		})
	}

	t.Run("server errors are returned", func(t *testing.T) {
		require.NoError(t, control.Set(ctx, key+":wrongtype", "unrelated", time.Minute).Err())
		err := recordRedisRequest(ctx, control, key+":wrongtype", 2, 60)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "WRONGTYPE")
		value, err := control.Get(ctx, key+":wrongtype").Result()
		require.NoError(t, err)
		assert.Equal(t, "unrelated", value)
		allowed, err := checkRedisRateLimit(ctx, control, key+":wrongtype", 2, 60)
		assert.Error(t, err)
		assert.False(t, allowed)
	})
}

func TestModelRedisFailureDoesNotCorruptAPIResponse(t *testing.T) {
	for _, duringResponse := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis_closes_during_response=%t", duringResponse), func(t *testing.T) {
			_, client := useRateLimitMiniRedis(t)
			if !duringResponse {
				require.NoError(t, client.Close())
			}
			engine := gin.New()
			called := false
			engine.GET("/relay", redisRateLimitHandler("redis-failure-test", 60, 0, 2), func(c *gin.Context) {
				called = true
				require.NoError(t, client.Close())
				c.Data(http.StatusOK, "text/event-stream", []byte("data: hello\n\ndata: [DONE]\n\n"))
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/relay", nil))
			assert.Equal(t, duringResponse, called)
			if duringResponse {
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Equal(t, "data: hello\n\ndata: [DONE]\n\n", response.Body.String())
			} else {
				assert.Equal(t, http.StatusInternalServerError, response.Code)
				assert.Contains(t, response.Body.String(), "rate_limit_check_failed")
			}
			assert.ErrorIs(t, recordRedisRequest(context.Background(), client, "closed", 2, 60), redis.ErrClosed)
		})
	}
}
