package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func notificationLimitFixture(t *testing.T) {
	t.Helper()
	oldRedis, oldClient := common.RedisEnabled, common.RDB
	oldCount, oldDuration := constant.NotifyLimitCount, constant.NotificationLimitDurationMinute
	oldEnabled, oldThreshold := common.QuotaRemindEnabled, common.QuotaRemindThreshold
	common.RedisEnabled = false
	constant.NotifyLimitCount, constant.NotificationLimitDurationMinute = 3, 90
	common.QuotaRemindEnabled, common.QuotaRemindThreshold = true, 100
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldRedis, oldClient
		constant.NotifyLimitCount, constant.NotificationLimitDurationMinute = oldCount, oldDuration
		common.QuotaRemindEnabled, common.QuotaRemindThreshold = oldEnabled, oldThreshold
	})
}

func TestNotificationLimitFixedWindow(t *testing.T) {
	notificationLimitFixture(t)
	server := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { assert.NoError(t, common.RDB.Close()) })
	common.RedisEnabled = true
	for i := 0; i < 3; i++ {
		allowed, err := CheckNotificationLimit(81001, "quota")
		require.NoError(t, err)
		require.True(t, allowed)
	}
	server.FastForward(61 * time.Minute)
	allowed, err := CheckNotificationLimit(81001, "quota")
	require.NoError(t, err)
	assert.False(t, allowed, "clock-hour changes must not reset a 90 minute window")
	assert.Equal(t, 29*time.Minute, server.TTL("notify_limit:81001:quota"))
	for _, tc := range []struct {
		id       int
		category string
	}{{81002, "quota"}, {81001, "channel"}} {
		allowed, err = CheckNotificationLimit(tc.id, tc.category)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
	server.FastForward(29 * time.Minute)
	allowed, err = CheckNotificationLimit(81001, "quota")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 90*time.Minute, server.TTL("notify_limit:81001:quota"))
	for _, duration := range []int{0, -1, int(^uint(0) >> 1)} {
		constant.NotificationLimitDurationMinute = duration
		allowed, err = CheckNotificationLimit(duration, "invalid-duration")
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 10*time.Minute, server.TTL(fmt.Sprintf("notify_limit:%d:invalid-duration", duration)))
	}
	require.NoError(t, server.Set("notify_limit:81003:broken", "invalid-counter"))
	allowed, err = CheckNotificationLimit(81003, "broken")
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestNotificationLimitConcurrentReservations(t *testing.T) {
	notificationLimitFixture(t)
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			common.RedisEnabled = backend == "redis"
			if common.RedisEnabled {
				addr := os.Getenv("TEST_REDIS_ADDR")
				if addr == "" {
					t.Skip("TEST_REDIS_ADDR not configured")
				}
				client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
				common.RDB = client
				ctx := context.Background()
				require.NoError(t, client.Ping(ctx).Err())
				key := "notify_limit:81004:concurrent"
				require.NoError(t, client.Del(ctx, key).Err())
				t.Cleanup(func() { assert.NoError(t, client.Del(ctx, key).Err()); assert.NoError(t, client.Close()) })
			} else {
				notifyLimitStore.Delete("81004:concurrent")
				t.Cleanup(func() { notifyLimitStore.Delete("81004:concurrent") })
			}
			var accepted atomic.Int32
			var wg sync.WaitGroup
			results := make(chan error, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					ok, err := CheckNotificationLimit(81004, "concurrent")
					results <- err
					if ok {
						accepted.Add(1)
					}
				}()
			}
			wg.Wait()
			close(results)
			for err := range results {
				require.NoError(t, err)
			}
			assert.EqualValues(t, 3, accepted.Load())
			if common.RedisEnabled {
				ttl, err := common.RDB.TTL(context.Background(), "notify_limit:81004:concurrent").Result()
				require.NoError(t, err)
				assert.Positive(t, ttl)
				assert.LessOrEqual(t, ttl, 90*time.Minute)
			} else {
				notifyLimitStore.Store("81004:concurrent", limitCount{Count: 3, Timestamp: time.Now().Add(-91 * time.Minute)})
				ok, err := CheckNotificationLimit(81004, "concurrent")
				require.NoError(t, err)
				assert.True(t, ok)
			}
		})
	}
	for _, limit := range []int{0, -1} {
		constant.NotifyLimitCount = limit
		ok, err := CheckNotificationLimit(81005, "disabled")
		require.NoError(t, err)
		assert.False(t, ok)
	}
}

func TestQuotaReminderThresholdCrossing(t *testing.T) {
	notificationLimitFixture(t)
	for _, tc := range []struct {
		name          string
		before, after int64
		threshold     float64
		enabled       *bool
		site          bool
		want          bool
	}{
		{"default crossing", 100, 99, 0, nil, true, true},
		{"already below", 99, 80, 0, nil, true, false},
		{"exact threshold", 110, 100, 0, nil, true, false},
		{"refund", 99, 120, 0, nil, true, false},
		{"no charge", 99, 99, 0, nil, true, false},
		{"topup rearmed", 120, 90, 0, nil, true, true},
		{"personal threshold", 200, 149, 150, nil, true, true},
		{"personal disabled", 100, 90, 0, common.GetPointer(false), true, false},
		{"site disabled", 100, 90, 0, common.GetPointer(true), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.QuotaRemindEnabled = tc.site
			assert.Equal(t, tc.want, quotaReminderDue(dto.UserSetting{QuotaWarningThreshold: tc.threshold, QuotaWarningEnabled: tc.enabled}, tc.before, tc.after))
		})
	}
}

func TestDisabledQuotaReminderDoesNotConsumeNotificationSlot(t *testing.T) {
	notificationLimitFixture(t)
	key := "81006:" + dto.NotifyTypeQuotaExceed
	notifyLimitStore.Delete(key)
	t.Cleanup(func() { notifyLimitStore.Delete(key) })
	notice := dto.NewNotify(dto.NotifyTypeQuotaExceed, "balance", "balance", nil)
	require.NoError(t, NotifyUser(81006, "", dto.UserSetting{QuotaWarningEnabled: common.GetPointer(false)}, notice))
	common.QuotaRemindEnabled = false
	require.NoError(t, NotifyUser(81006, "", dto.UserSetting{}, notice))
	common.QuotaRemindEnabled = true
	for i := 0; i < 3; i++ {
		ok, err := CheckNotificationLimit(81006, dto.NotifyTypeQuotaExceed)
		require.NoError(t, err)
		assert.True(t, ok)
	}
}
