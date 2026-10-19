package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	modelRateLimitTimeFormat              = "2006-01-02T15:04:05.000Z"
)

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, err := rdb.LIndex(ctx, key, -1).Result()
	if err == redis.Nil {
		// The key can expire between LLEN and LINDEX. An absent window has no
		// remaining successful requests to limit; it is not a Redis failure.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	oldTime, err := time.Parse(modelRateLimitTimeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	// Preserve the stored millisecond precision without formatting and parsing
	// the current timestamp again on every full-window check.
	nowTime := time.Now().UTC().Truncate(time.Millisecond)
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(duration)*time.Second)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) error {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return nil
	}

	now := time.Now().UTC().Format(modelRateLimitTimeFormat)
	// MULTI/EXEC keeps the three operations in one network round trip and
	// prevents a disconnect before EXEC from leaving a partially recorded hit.
	// go-redis v8 does not replay a TxPipeline after a reply-read failure; an
	// ordinary Pipeline can replay LPUSH and count the same request twice.
	_, err := rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, key, now)
		pipe.LTrim(ctx, key, 0, int64(maxCount-1))
		pipe.Expire(ctx, key, time.Duration(duration)*time.Second)
		return nil
	})
	return err
}

// Redis限流处理器
func redisRateLimitHandler(subject string, duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()
		rdb := common.RDB
		durationMinutes := int(duration / 60)

		// 1. 检查成功请求数限制
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, subject)
		allowed, err := checkRedisRateLimit(ctx, rdb, successKey, successMaxCount, duration)
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", durationMinutes, successMaxCount))
			return
		}

		//2.检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s", subject)
			// 初始化
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(rateLimitCapacity(totalMaxCount, duration)),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)

			if err != nil {
				fmt.Println("检查总请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}

			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", durationMinutes, totalMaxCount))
				return
			}
		}

		// 4. 处理请求
		c.Next()

		// 5. 如果请求成功，记录成功请求
		if modelRequestSucceeded(c) {
			if err := recordRedisRequest(ctx, rdb, successKey, successMaxCount, duration); err != nil {
				// The upstream response is already complete. Report the accounting
				// failure without appending an error to a successful JSON/SSE body.
				logger.LogError(c, "failed to record successful request for rate limit: "+err.Error())
			}
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(subject string, duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(duration) * time.Second)

	return func(c *gin.Context) {
		totalKey := ModelRequestRateLimitCountMark + subject
		successKey := ModelRequestRateLimitSuccessCountMark + subject

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		var reservation *common.RateLimitReservation
		if successMaxCount > 0 {
			reservation = inMemoryRateLimiter.Reserve(successKey, successMaxCount, duration)
			if reservation == nil {
				c.AbortWithStatus(http.StatusTooManyRequests)
				return
			}
			defer reservation.Complete(false)
		}

		// 3. 处理请求
		c.Next()

		reservation.Complete(modelRequestSucceeded(c))
	}
}

func modelRequestSucceeded(c *gin.Context) bool {
	status, _ := common.GetContextKeyType[*relaycommon.StreamStatus](c, constant.ContextKeyResponseStreamStatus)
	return c.Writer.Status() < 400 && !status.ResponseFailed()
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := rateLimitDurationSeconds(setting.ModelRequestRateLimitDurationMinutes)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		userId := c.GetInt("id")
		userOverride := false
		if userLimit, found, err := model.GetUserRateLimit(userId); err == nil && found {
			userOverride = true
			duration = int64(userLimit.DurationMinutes * 60)
			totalMaxCount = userLimit.TotalCount
			successMaxCount = userLimit.SuccessCount
		} else if err != nil {
			common.SysLog(fmt.Sprintf("failed to get user rate limit for user %d: %s", userId, err.Error()))
		}

		// 用户专属限流仍优先，并跨分组共享；只有独立的分组限流才分开计数。
		subject := strconv.Itoa(userId)
		if !userOverride {
			// 获取分组
			group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
			if group == "" {
				group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
			}

			//获取分组的限流配置
			groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
			if found {
				totalMaxCount = groupTotalCount
				successMaxCount = groupSuccessCount
				subject += ":" + group
			}
		}

		if duration <= 0 || successMaxCount <= 0 || totalMaxCount < 0 {
			c.Next()
			return
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(subject, duration, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(subject, duration, totalMaxCount, successMaxCount)(c)
		}
	}
}

func rateLimitDurationSeconds(durationMinutes int) int64 {
	if durationMinutes <= 0 {
		return 0
	}
	minutes := int64(durationMinutes)
	if minutes > math.MaxInt64/60 {
		return math.MaxInt64
	}
	return minutes * 60
}

func rateLimitCapacity(count int, durationSeconds int64) int64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	c := int64(count)
	if c > math.MaxInt64/durationSeconds {
		return math.MaxInt64
	}
	return c * durationSeconds
}
