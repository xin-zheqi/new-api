package common

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type StreamEndReason string

const (
	StreamEndReasonNone                 StreamEndReason = ""
	StreamEndReasonDone                 StreamEndReason = "done"
	StreamEndReasonTimeout              StreamEndReason = "timeout"
	StreamEndReasonFirstResponseTimeout StreamEndReason = "first_response_timeout"
	StreamEndReasonClientGone           StreamEndReason = "client_gone"
	StreamEndReasonScannerErr           StreamEndReason = "scanner_error"
	StreamEndReasonHandlerStop          StreamEndReason = "handler_stop"
	StreamEndReasonEOF                  StreamEndReason = "eof"
	StreamEndReasonPanic                StreamEndReason = "panic"
	StreamEndReasonPingFail             StreamEndReason = "ping_fail"
)

// ResponseOutcome records the protocol-level result independently of the
// transport result. A WebSocket can close normally while the Responses
// protocol still reports a failed or incomplete response.
type ResponseOutcome string

const (
	ResponseOutcomeUnknown    ResponseOutcome = ""
	ResponseOutcomeCompleted  ResponseOutcome = "completed"
	ResponseOutcomeFailed     ResponseOutcome = "failed"
	ResponseOutcomeIncomplete ResponseOutcome = "incomplete"
	ResponseOutcomeCancelled  ResponseOutcome = "cancelled"
)

const maxStreamErrorEntries = 20

type StreamErrorEntry struct {
	Message   string
	Timestamp time.Time
}

type StreamStatus struct {
	EndReason StreamEndReason
	EndError  error
	endOnce   sync.Once

	mu         sync.Mutex
	Errors     []StreamErrorEntry
	ErrorCount int

	response         ResponseOutcome
	errorCode        string
	errorType        string
	errorStatus      int
	incompleteReason string
	expectsTerminal  bool
}

// StreamOutcome exposes classification facts without retaining upstream
// response messages, which may contain user input or provider data.
type StreamOutcome struct {
	EndReason        StreamEndReason
	HasErrors        bool
	ExpectsTerminal  bool
	Response         ResponseOutcome
	ErrorCode        string
	ErrorType        string
	ErrorStatus      int
	IncompleteReason string
}

func NewStreamStatus() *StreamStatus {
	return &StreamStatus{}
}

func (s *StreamStatus) SetEndReason(reason StreamEndReason, err error) {
	if s == nil {
		return
	}
	s.endOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.EndReason = reason
		s.EndError = err
	})
}

// MarkClientGone gives downstream cancellation precedence over a concurrent
// socket/read error. The reader may observe the closed connection just before
// the request context cancellation becomes visible; retaining scanner_error in
// that race would incorrectly mark a healthy channel as failed.
func (s *StreamStatus) MarkClientGone(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.EndReason != StreamEndReasonNone && s.EndReason != StreamEndReasonScannerErr {
		s.mu.Unlock()
		return
	}
	s.EndReason = StreamEndReasonClientGone
	s.EndError = err
	s.mu.Unlock()
}

func (s *StreamStatus) RecordError(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
	if len(s.Errors) < maxStreamErrorEntries {
		s.Errors = append(s.Errors, StreamErrorEntry{
			Message:   msg,
			Timestamp: time.Now(),
		})
	}
}

// RequireTerminal marks protocols that must emit an explicit terminal event.
// It is used by the Responses transports to distinguish a cut stream from a
// completed response when the socket closes without a terminal frame.
func (s *StreamStatus) RequireTerminal() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expectsTerminal = true
}

func (s *StreamStatus) MarkCompleted() {
	s.markTerminal(ResponseOutcomeCompleted, "")
}

func (s *StreamStatus) MarkIncomplete(reason string) {
	s.markTerminal(ResponseOutcomeIncomplete, reason)
}

func (s *StreamStatus) MarkCancelled() {
	s.markTerminal(ResponseOutcomeCancelled, "")
}

func (s *StreamStatus) markTerminal(outcome ResponseOutcome, incompleteReason string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.response != ResponseOutcomeUnknown {
		return
	}
	s.response = outcome
	s.incompleteReason = incompleteReason
}

// MarkFailed always wins over an earlier terminal outcome because a protocol
// error after a terminal frame still indicates an unsuccessful exchange.
func (s *StreamStatus) MarkFailed(code, errorType string, status int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.response = ResponseOutcomeFailed
	if code != "" {
		s.errorCode = code
	}
	if errorType != "" {
		s.errorType = errorType
	}
	if status != 0 {
		s.errorStatus = status
	}
}

func (s *StreamStatus) ResponseOutcome() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.response)
}

func (s *StreamStatus) ResponseFailed() bool {
	return s.ResponseOutcome() == string(ResponseOutcomeFailed)
}

func (s *StreamStatus) OutcomeSnapshot() StreamOutcome {
	if s == nil {
		return StreamOutcome{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return StreamOutcome{
		EndReason:        s.EndReason,
		HasErrors:        s.ErrorCount > 0,
		ExpectsTerminal:  s.expectsTerminal,
		Response:         s.response,
		ErrorCode:        s.errorCode,
		ErrorType:        s.errorType,
		ErrorStatus:      s.errorStatus,
		IncompleteReason: s.incompleteReason,
	}
}

func (s *StreamStatus) HasErrors() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount > 0
}

func (s *StreamStatus) TotalErrorCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount
}

func (s *StreamStatus) IsNormalEnd() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	reason := s.EndReason
	s.mu.Unlock()
	return reason == StreamEndReasonDone ||
		reason == StreamEndReasonEOF ||
		reason == StreamEndReasonHandlerStop
}

func (s *StreamStatus) Summary() string {
	if s == nil {
		return "StreamStatus<nil>"
	}
	b := &strings.Builder{}
	s.mu.Lock()
	reason, endError := s.EndReason, s.EndError
	if s.ErrorCount > 0 {
		fmt.Fprintf(b, " soft_errors=%d", s.ErrorCount)
	}
	s.mu.Unlock()
	fmt.Fprintf(b, "reason=%s", reason)
	if endError != nil {
		fmt.Fprintf(b, " end_error=%q", endError.Error())
	}
	return b.String()
}
