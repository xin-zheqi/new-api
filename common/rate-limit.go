package common

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MaxRouteRateLimitRequests = 100_000_000

const (
	GlobalApiRateLimitEnabledOption  = "GlobalApiRateLimitEnabled"
	GlobalApiRateLimitCountOption    = "GlobalApiRateLimitCount"
	GlobalApiRateLimitDurationOption = "GlobalApiRateLimitDuration"
	GlobalWebRateLimitEnabledOption  = "GlobalWebRateLimitEnabled"
	GlobalWebRateLimitCountOption    = "GlobalWebRateLimitCount"
	GlobalWebRateLimitDurationOption = "GlobalWebRateLimitDuration"
	CriticalRateLimitEnabledOption   = "CriticalRateLimitEnabled"
	CriticalRateLimitCountOption     = "CriticalRateLimitCount"
	CriticalRateLimitDurationOption  = "CriticalRateLimitDuration"
)

type RouteRateLimitKind uint8

const (
	RouteRateLimitGlobalAPI RouteRateLimitKind = iota
	RouteRateLimitGlobalWeb
	RouteRateLimitCritical
)

type RouteRateLimitConfig struct {
	Enabled         bool
	MaxRequests     int
	DurationSeconds int64
}

var routeRateLimitConfigMutex sync.RWMutex

func GetRouteRateLimitConfig(kind RouteRateLimitKind) RouteRateLimitConfig {
	routeRateLimitConfigMutex.RLock()
	defer routeRateLimitConfigMutex.RUnlock()

	switch kind {
	case RouteRateLimitGlobalAPI:
		return RouteRateLimitConfig{GlobalApiRateLimitEnable, GlobalApiRateLimitNum, GlobalApiRateLimitDuration}
	case RouteRateLimitGlobalWeb:
		return RouteRateLimitConfig{GlobalWebRateLimitEnable, GlobalWebRateLimitNum, GlobalWebRateLimitDuration}
	case RouteRateLimitCritical:
		return RouteRateLimitConfig{CriticalRateLimitEnable, CriticalRateLimitNum, CriticalRateLimitDuration}
	default:
		return RouteRateLimitConfig{}
	}
}

func NormalizeRouteRateLimitSetting(key, value string) (string, bool, error) {
	trimmed := strings.TrimSpace(value)
	switch key {
	case GlobalApiRateLimitEnabledOption, GlobalWebRateLimitEnabledOption, CriticalRateLimitEnabledOption:
		parsed, err := strconv.ParseBool(trimmed)
		if err != nil {
			return "", true, fmt.Errorf("%s must be true or false", key)
		}
		return strconv.FormatBool(parsed), true, nil
	case GlobalApiRateLimitCountOption, GlobalWebRateLimitCountOption, CriticalRateLimitCountOption:
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil || parsed < 1 || parsed > MaxRouteRateLimitRequests {
			return "", true, fmt.Errorf("%s must be an integer between 1 and %d", key, MaxRouteRateLimitRequests)
		}
		return strconv.FormatInt(parsed, 10), true, nil
	case GlobalApiRateLimitDurationOption, GlobalWebRateLimitDurationOption, CriticalRateLimitDurationOption:
		maximum := int64(RateLimitKeyExpirationDuration / time.Second)
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil || parsed < 1 || parsed > maximum {
			return "", true, fmt.Errorf("%s must be an integer between 1 and %d", key, maximum)
		}
		return strconv.FormatInt(parsed, 10), true, nil
	default:
		return value, false, nil
	}
}

func ApplyRouteRateLimitSetting(key, value string) (bool, error) {
	normalized, recognized, err := NormalizeRouteRateLimitSetting(key, value)
	if !recognized || err != nil {
		return recognized, err
	}

	routeRateLimitConfigMutex.Lock()
	defer routeRateLimitConfigMutex.Unlock()

	switch key {
	case GlobalApiRateLimitEnabledOption:
		GlobalApiRateLimitEnable, _ = strconv.ParseBool(normalized)
	case GlobalApiRateLimitCountOption:
		GlobalApiRateLimitNum, _ = strconv.Atoi(normalized)
	case GlobalApiRateLimitDurationOption:
		GlobalApiRateLimitDuration, _ = strconv.ParseInt(normalized, 10, 64)
	case GlobalWebRateLimitEnabledOption:
		GlobalWebRateLimitEnable, _ = strconv.ParseBool(normalized)
	case GlobalWebRateLimitCountOption:
		GlobalWebRateLimitNum, _ = strconv.Atoi(normalized)
	case GlobalWebRateLimitDurationOption:
		GlobalWebRateLimitDuration, _ = strconv.ParseInt(normalized, 10, 64)
	case CriticalRateLimitEnabledOption:
		CriticalRateLimitEnable, _ = strconv.ParseBool(normalized)
	case CriticalRateLimitCountOption:
		CriticalRateLimitNum, _ = strconv.Atoi(normalized)
	case CriticalRateLimitDurationOption:
		CriticalRateLimitDuration, _ = strconv.ParseInt(normalized, 10, 64)
	}
	return true, nil
}

type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]int64)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		if len(*queue) < maxRequestNum {
			*queue = append(*queue, now)
			return true
		} else {
			if now-(*queue)[0] >= duration {
				*queue = (*queue)[1:]
				*queue = append(*queue, now)
				return true
			} else {
				return false
			}
		}
	} else {
		s := make([]int64, 0, 1)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
	}
	return true
}
