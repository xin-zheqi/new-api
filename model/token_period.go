package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	QuotaPeriodDay   = "day"
	QuotaPeriodWeek  = "week"
	QuotaPeriodMonth = "month"
)

func NormalizeQuotaPeriod(period string) string {
	switch period {
	case QuotaPeriodDay, QuotaPeriodWeek, QuotaPeriodMonth:
		return period
	default:
		return ""
	}
}

func IsQuotaPeriod(period string) bool {
	return NormalizeQuotaPeriod(period) != ""
}

// QuotaPeriodStart returns the start of the current calendar period in the
// server's configured local timezone. It intentionally uses calendar dates,
// not a fixed 24/168-hour window, so daylight-saving changes do not drift a
// daily or monthly limit.
func QuotaPeriodStart(period string, now time.Time) int64 {
	period = NormalizeQuotaPeriod(period)
	local := now.In(time.Local)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	switch period {
	case QuotaPeriodWeek:
		// Monday is the first day of the weekly quota window.
		back := (int(start.Weekday()) + 6) % 7
		start = start.AddDate(0, 0, -back)
	case QuotaPeriodMonth:
		start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	}
	return start.Unix()
}

func (token *Token) HasQuotaPeriod() bool {
	return token != nil && IsQuotaPeriod(token.QuotaPeriod) && token.QuotaPeriodLimit > 0
}

func currentTokenPeriod(token *Token, now time.Time) {
	if !token.HasQuotaPeriod() {
		return
	}
	start := QuotaPeriodStart(token.QuotaPeriod, now)
	if token.QuotaPeriodStart != start {
		token.QuotaPeriodStart = start
		token.QuotaPeriodUsed = 0
	}
}

func GetTokenPeriodRemaining(token *Token) int {
	if token == nil || !token.HasQuotaPeriod() {
		return 0
	}
	currentTokenPeriod(token, time.Now())
	remaining := token.QuotaPeriodLimit - token.QuotaPeriodUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TryReserveTokenPeriodQuota atomically resets an expired period and reserves
// quota within the current period. It uses a transaction so MySQL/PostgreSQL
// get a row lock; SQLite serializes the write transaction itself.
func TryReserveTokenPeriodQuota(id int, key string, period string, quota int) (bool, error) {
	if err := common.ValidateQuota(quota); err != nil {
		return false, err
	}
	period = NormalizeQuotaPeriod(period)
	if period == "" {
		return false, errors.New("invalid token quota period")
	}

	reserved := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", id).First(&token).Error; err != nil {
			return err
		}
		if token.QuotaPeriod != period || token.QuotaPeriodLimit <= 0 {
			return fmt.Errorf("token quota period is not configured")
		}
		currentTokenPeriod(&token, time.Now())
		if quota == 0 {
			reserved = token.QuotaPeriodUsed < token.QuotaPeriodLimit
			return nil
		}
		if token.QuotaPeriodUsed > token.QuotaPeriodLimit-quota {
			return nil
		}
		token.QuotaPeriodUsed += quota
		token.AccessedTime = common.GetTimestamp()
		if err := tx.Model(&Token{}).Where("id = ?", id).Updates(map[string]interface{}{
			"quota_period_used":  token.QuotaPeriodUsed,
			"quota_period_start": token.QuotaPeriodStart,
			"accessed_time":      token.AccessedTime,
		}).Error; err != nil {
			return err
		}
		reserved = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if common.RedisEnabled && key != "" {
		// The period counters are not part of the atomic legacy quota cache. Drop
		// the snapshot so the next metadata read observes the reset/usage state.
		if err := invalidateTokenCacheForMutation(key); err != nil {
			common.SysLog("failed to invalidate periodic token cache: " + err.Error())
		}
	}
	return reserved, nil
}

// AdjustTokenPeriodQuota applies a settlement delta. Negative deltas release
// a reservation after an upstream failure; they are clamped at zero to avoid
// turning a refund into credit for a later period.
func AdjustTokenPeriodQuota(id int, key string, period string, delta int) error {
	if err := common.ValidateQuotaDelta(delta); err != nil {
		return err
	}
	if delta == 0 {
		return nil
	}
	period = NormalizeQuotaPeriod(period)
	if period == "" {
		return errors.New("invalid token quota period")
	}
	exceeded := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", id).First(&token).Error; err != nil {
			return err
		}
		if token.QuotaPeriod != period || token.QuotaPeriodLimit <= 0 {
			return fmt.Errorf("token quota period is not configured")
		}
		currentTokenPeriod(&token, time.Now())
		nextUsed := token.QuotaPeriodUsed + delta
		if nextUsed < 0 {
			nextUsed = 0
		}
		if nextUsed > token.QuotaPeriodLimit {
			// The upstream's final cost can exceed the pre-request estimate.
			// The response has already been served and the full charge is settled
			// separately; exhaust this key without turning success into an error.
			nextUsed = token.QuotaPeriodLimit
			exceeded = true
		}
		return tx.Model(&Token{}).Where("id = ?", id).Updates(map[string]interface{}{
			"quota_period_used":  nextUsed,
			"quota_period_start": token.QuotaPeriodStart,
			"accessed_time":      common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		return err
	}
	if exceeded {
		common.SysLog(fmt.Sprintf("token period limit reached by settlement (tokenId=%d, delta=%d)", id, delta))
	}
	if common.RedisEnabled && key != "" {
		if err := invalidateTokenCacheForMutation(key); err != nil {
			common.SysLog("failed to invalidate periodic token cache: " + err.Error())
		}
	}
	return nil
}
