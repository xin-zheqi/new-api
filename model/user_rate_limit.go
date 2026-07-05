package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
)

const userRateLimitCacheTTL = 5 * time.Minute

type UserRateLimit struct {
	Enabled         bool
	DurationMinutes int
	TotalCount      int
	SuccessCount    int
}

func (limit UserRateLimit) Valid() bool {
	return limit.Enabled && limit.DurationMinutes > 0 && limit.SuccessCount > 0 && limit.TotalCount >= 0
}

func getUserRateLimitCacheKey(userId int) string {
	return fmt.Sprintf("user_rate_limit:%d", userId)
}

func InvalidateUserRateLimitCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisDelKey(getUserRateLimitCacheKey(userId))
}

func updateUserRateLimitCache(userId int, limit UserRateLimit) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHSetObj(getUserRateLimitCacheKey(userId), &limit, userRateLimitCacheTTL)
}

func getUserRateLimitCache(userId int) (UserRateLimit, error) {
	if !common.RedisEnabled {
		return UserRateLimit{}, fmt.Errorf("redis is not enabled")
	}
	var limit UserRateLimit
	if err := common.RedisHGetObj(getUserRateLimitCacheKey(userId), &limit); err != nil {
		return UserRateLimit{}, err
	}
	return limit, nil
}

func GetUserRateLimit(userId int) (limit UserRateLimit, found bool, err error) {
	if !common.RedisEnabled {
		return UserRateLimit{}, false, nil
	}
	var fromDB bool
	defer func() {
		if fromDB && err == nil {
			gopool.Go(func() {
				if cacheErr := updateUserRateLimitCache(userId, limit); cacheErr != nil {
					common.SysLog("failed to update user rate limit cache: " + cacheErr.Error())
				}
			})
		}
	}()

	if limit, err = getUserRateLimitCache(userId); err == nil {
		return limit, limit.Valid(), nil
	}

	fromDB = true
	var user User
	err = DB.Model(&User{}).Select("rate_limit_enabled", "rate_limit_duration_minutes", "rate_limit_total_count", "rate_limit_success_count").
		Where("id = ?", userId).First(&user).Error
	if err != nil {
		return UserRateLimit{}, false, err
	}
	limit = UserRateLimit{
		Enabled:         user.RateLimitEnabled,
		DurationMinutes: user.RateLimitDurationMinutes,
		TotalCount:      user.RateLimitTotalCount,
		SuccessCount:    user.RateLimitSuccessCount,
	}
	return limit, limit.Valid(), nil
}
