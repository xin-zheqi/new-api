package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/types"
)

type FirstResponseTimeoutGuard struct {
	timeout time.Duration
	started time.Time

	cancel context.CancelFunc
	timer  *time.Timer

	mu        sync.Mutex
	completed bool
	timedOut  bool
}

func NewFirstResponseTimeoutGuard(timeoutSeconds int) *FirstResponseTimeoutGuard {
	if timeoutSeconds <= 0 {
		return nil
	}
	return &FirstResponseTimeoutGuard{
		timeout: time.Duration(timeoutSeconds) * time.Second,
		started: time.Now(),
	}
}

func (g *FirstResponseTimeoutGuard) WithCancel(parent context.Context) (context.Context, context.CancelFunc) {
	if g == nil {
		return parent, func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	g.mu.Lock()
	g.cancel = cancel
	g.mu.Unlock()
	g.timer = time.AfterFunc(g.timeout, func() {
		g.mu.Lock()
		if g.completed {
			g.mu.Unlock()
			return
		}
		g.timedOut = true
		cancel()
		g.mu.Unlock()
	})
	return ctx, cancel
}

func (g *FirstResponseTimeoutGuard) MarkContent() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if !g.completed {
		g.completed = true
		if g.timer != nil {
			g.timer.Stop()
		}
	}
	g.mu.Unlock()
}

func (g *FirstResponseTimeoutGuard) Cancel() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if g.timer != nil {
		g.timer.Stop()
	}
	cancel := g.cancel
	g.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (g *FirstResponseTimeoutGuard) IsTimedOut() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.timedOut
}

func (g *FirstResponseTimeoutGuard) TimeoutSeconds() int {
	if g == nil {
		return 0
	}
	return int(g.timeout / time.Second)
}

func (g *FirstResponseTimeoutGuard) Elapsed() time.Duration {
	if g == nil || g.started.IsZero() {
		return 0
	}
	return time.Since(g.started)
}

func (info *RelayInfo) GetFirstResponseTimeoutSeconds() int {
	if info == nil {
		return 0
	}
	return info.ChannelSetting.FirstResponseTimeoutSeconds
}

func (info *RelayInfo) EnsureFirstResponseTimeoutGuard() *FirstResponseTimeoutGuard {
	if info == nil {
		return nil
	}
	if info.FirstResponseTimeoutGuard != nil {
		return info.FirstResponseTimeoutGuard
	}
	info.FirstResponseTimeoutGuard = NewFirstResponseTimeoutGuard(info.GetFirstResponseTimeoutSeconds())
	return info.FirstResponseTimeoutGuard
}

func (info *RelayInfo) StartFirstResponseTimeoutGuard() *FirstResponseTimeoutGuard {
	if info == nil {
		return nil
	}
	if info.FirstResponseTimeoutGuard != nil {
		info.FirstResponseTimeoutGuard.Cancel()
	}
	info.FirstResponseTimeoutGuard = NewFirstResponseTimeoutGuard(info.GetFirstResponseTimeoutSeconds())
	return info.FirstResponseTimeoutGuard
}

func (info *RelayInfo) MarkFirstResponseContent() {
	if info == nil {
		return
	}
	if info.FirstResponseTimeoutGuard != nil {
		info.FirstResponseTimeoutGuard.MarkContent()
	}
}

func (info *RelayInfo) CancelFirstResponseTimeoutGuard() {
	if info == nil {
		return
	}
	if info.FirstResponseTimeoutGuard != nil {
		info.FirstResponseTimeoutGuard.Cancel()
	}
}

func (info *RelayInfo) IsFirstResponseTimedOut() bool {
	if info == nil || info.FirstResponseTimeoutGuard == nil {
		return false
	}
	return info.FirstResponseTimeoutGuard.IsTimedOut()
}

func (info *RelayInfo) ShouldFailFirstResponseTimeout() bool {
	if info == nil || info.ReceivedResponseCount > 0 {
		return false
	}
	if info.IsFirstResponseTimedOut() {
		return true
	}
	guard := info.FirstResponseTimeoutGuard
	if guard == nil {
		return false
	}
	timeoutSeconds := guard.TimeoutSeconds()
	return timeoutSeconds > 0 && guard.Elapsed() >= time.Duration(timeoutSeconds)*time.Second
}

func NewFirstResponseTimeoutError(info *RelayInfo) *types.NewAPIError {
	timeoutSeconds := 0
	elapsed := time.Duration(0)
	if info != nil && info.FirstResponseTimeoutGuard != nil {
		timeoutSeconds = info.FirstResponseTimeoutGuard.TimeoutSeconds()
		elapsed = info.FirstResponseTimeoutGuard.Elapsed()
	} else if info != nil {
		timeoutSeconds = info.GetFirstResponseTimeoutSeconds()
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = int(elapsed / time.Second)
	}
	message := fmt.Sprintf("upstream first response timeout after %ds", timeoutSeconds)
	if elapsed > 0 {
		message = fmt.Sprintf("%s (elapsed %.3fs)", message, elapsed.Seconds())
	}
	return types.NewOpenAIError(
		errors.New(message),
		types.ErrorCodeChannelFirstResponseTimeout,
		http.StatusBadGateway,
	)
}
