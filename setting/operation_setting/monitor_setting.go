package operation_setting

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled   bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes   float64 `json:"auto_test_channel_minutes"`
	AutoTestChannelTimeRange string  `json:"auto_test_channel_time_range"`
	AutoTestOnlyAutoDisabled bool    `json:"auto_test_only_auto_disabled"`
	ChannelTestMode          string  `json:"channel_test_mode"`
	ChannelTestConcurrency   int     `json:"channel_test_concurrency"`
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModeAutoBanOnly     = "auto_ban_only"
	ChannelTestModePassiveRecovery = "passive_recovery"

	ChannelTestConcurrencyOptionKey = "monitor_setting.channel_test_concurrency"
	DefaultChannelTestConcurrency   = 1
	MaxChannelTestConcurrency       = 32
)

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:   false,
	AutoTestChannelMinutes:   10,
	AutoTestChannelTimeRange: "00:00-23:59",
	AutoTestOnlyAutoDisabled: false,
	ChannelTestMode:          ChannelTestModeScheduledAll,
	ChannelTestConcurrency:   DefaultChannelTestConcurrency,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
			monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
			monitorSetting.AutoTestOnlyAutoDisabled = false
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			monitorSetting.AutoTestChannelEnabled = parsed
		}
	}
	monitorSetting.ChannelTestMode = EffectiveChannelTestMode(&monitorSetting)
	monitorSetting.AutoTestOnlyAutoDisabled = monitorSetting.ChannelTestMode == ChannelTestModePassiveRecovery
	monitorSetting.ChannelTestConcurrency = NormalizeChannelTestConcurrency(monitorSetting.ChannelTestConcurrency)
	return &monitorSetting
}

func EffectiveChannelTestMode(setting *MonitorSetting) string {
	if setting == nil {
		return ChannelTestModeScheduledAll
	}
	if setting.ChannelTestMode == ChannelTestModePassiveRecovery || setting.AutoTestOnlyAutoDisabled {
		return ChannelTestModePassiveRecovery
	}
	if setting.ChannelTestMode == ChannelTestModeAutoBanOnly {
		return ChannelTestModeAutoBanOnly
	}
	return ChannelTestModeScheduledAll
}

func parseClockMinute(value string) (int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("time must use HH:MM format")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hour")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minute")
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("time out of range")
	}
	return hour*60 + minute, nil
}

func ParseAutoTestChannelTimeRange(value string) (int, int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = "00:00-23:59"
	}
	parts := strings.Split(trimmed, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("time range must use HH:MM-HH:MM format")
	}
	start, err := parseClockMinute(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start time: %w", err)
	}
	end, err := parseClockMinute(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end time: %w", err)
	}
	return start, end, nil
}

func IsMinuteInAutoTestChannelTimeRange(minute int, value string) bool {
	if minute < 0 || minute >= 24*60 {
		return false
	}
	start, end, err := ParseAutoTestChannelTimeRange(value)
	if err != nil {
		return true
	}
	if start == end {
		return true
	}
	if start < end {
		return minute >= start && minute <= end
	}
	return minute >= start || minute <= end
}

func IsNowInAutoTestChannelTimeRange(now time.Time, value string) bool {
	return IsMinuteInAutoTestChannelTimeRange(now.Hour()*60+now.Minute(), value)
}

func NormalizeChannelTestConcurrency(concurrency int) int {
	if concurrency < 1 {
		return DefaultChannelTestConcurrency
	}
	if concurrency > MaxChannelTestConcurrency {
		return MaxChannelTestConcurrency
	}
	return concurrency
}

func ValidateChannelTestConcurrency(value string) error {
	concurrency, err := strconv.Atoi(value)
	if err != nil || concurrency < 1 || concurrency > MaxChannelTestConcurrency {
		return fmt.Errorf("channel test concurrency must be between 1 and %d", MaxChannelTestConcurrency)
	}
	return nil
}
