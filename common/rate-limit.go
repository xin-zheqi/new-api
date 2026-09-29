package common

import (
	"container/list"
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

// rateLimitRequest stores one accepted request in a key's sliding window.
type rateLimitRequest struct {
	next      *rateLimitRequest
	timestamp int64
}

// rateLimitQueue keeps accepted requests ordered from oldest to newest.
type rateLimitQueue struct {
	head   *rateLimitRequest
	tail   *rateLimitRequest
	length int
}

func (q *rateLimitQueue) append(timestamp int64) {
	request := &rateLimitRequest{timestamp: timestamp}
	if q.tail == nil {
		q.head = request
		q.tail = request
	} else {
		q.tail.next = request
		q.tail = request
	}
	q.length++
}

func (q *rateLimitQueue) removeExpired(now int64, duration int64) {
	if q.head == nil || now-q.head.timestamp < duration {
		return
	}
	if now-q.tail.timestamp >= duration {
		q.clear()
		return
	}
	for q.head != nil && now-q.head.timestamp >= duration {
		expired := q.head
		q.head = expired.next
		expired.next = nil
		q.length--
	}
	if q.head == nil {
		q.tail = nil
	}
}

func (q *rateLimitQueue) clear() {
	q.head = nil
	q.tail = nil
	q.length = 0
}

type rateLimitEntry struct {
	lastActive time.Time
	element    *list.Element
	requests   rateLimitQueue
	key        string
}

// InMemoryRateLimiter implements a sliding-window limiter with idle-key eviction.
// Reservations are tracked independently so an in-flight request survives key eviction.
type InMemoryRateLimiter struct {
	store              map[string]*rateLimitEntry
	lru                *list.List
	reservations       map[string]int
	mutex              sync.Mutex
	expirationDuration time.Duration
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.store != nil {
		return
	}
	l.store = make(map[string]*rateLimitEntry)
	l.lru = list.New()
	l.reservations = make(map[string]int)
	l.expirationDuration = expirationDuration
	if expirationDuration > 0 {
		go l.clearExpiredItems(time.NewTicker(expirationDuration).C)
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems(ticks <-chan time.Time) {
	for now := range ticks {
		l.deleteExpiredEntries(now)
	}
}

func (l *InMemoryRateLimiter) deleteExpiredEntries(now time.Time) {
	if l.expirationDuration <= 0 {
		return
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for {
		oldest := l.lru.Back()
		if oldest == nil {
			return
		}
		entry := oldest.Value.(*rateLimitEntry)
		if now.Sub(entry.lastActive) < l.expirationDuration {
			return
		}
		delete(l.store, entry.key)
		l.lru.Remove(oldest)
		entry.element = nil
		entry.requests.clear()
	}
}

func (l *InMemoryRateLimiter) touchEntry(key string, now time.Time) *rateLimitEntry {
	entry, ok := l.store[key]
	if !ok {
		entry = &rateLimitEntry{key: key, lastActive: now}
		entry.element = l.lru.PushFront(entry)
		l.store[key] = entry
		return entry
	}
	entry.lastActive = now
	l.lru.MoveToFront(entry.element)
	return entry
}

// Request parameter duration's unit is seconds.
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now()
	entry := l.touchEntry(key, now)
	entry.requests.removeExpired(now.Unix(), duration)
	if entry.requests.length >= maxRequestNum {
		return false
	}
	entry.requests.append(now.Unix())
	return true
}

// RateLimitReservation counts an active request until its protocol outcome is known.
type RateLimitReservation struct {
	limiter *InMemoryRateLimiter
	key     string
	once    sync.Once
}

func (l *InMemoryRateLimiter) Reserve(key string, maxRequests int, duration int64) *RateLimitReservation {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now()
	entry := l.touchEntry(key, now)
	entry.requests.removeExpired(now.Unix(), duration)
	if entry.requests.length+l.reservations[key] >= maxRequests {
		return nil
	}
	l.reservations[key]++
	return &RateLimitReservation{limiter: l, key: key}
}

func (r *RateLimitReservation) Complete(success bool) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		l := r.limiter
		l.mutex.Lock()
		defer l.mutex.Unlock()
		if l.reservations[r.key] > 0 {
			l.reservations[r.key]--
		}
		if l.reservations[r.key] == 0 {
			delete(l.reservations, r.key)
		}
		if success {
			l.touchEntry(r.key, time.Now()).requests.append(time.Now().Unix())
		}
	})
}
