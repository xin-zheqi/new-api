package helper

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
)

type prefixedResponseBody struct {
	io.Reader
	closer io.Closer
}

func (b *prefixedResponseBody) Close() error {
	if b == nil || b.closer == nil {
		return nil
	}
	return b.closer.Close()
}

// ValidateUpstreamTextResponse keeps retry-safe response data private until an
// enabled channel produces substantive output or reports positive output token usage.
func ValidateUpstreamTextResponse(resp *http.Response, info *relaycommon.RelayInfo) *types.NewAPIError {
	if resp == nil || !shouldValidateUpstreamTextResponse(info) {
		return nil
	}
	if resp.Body == nil {
		return NewEmptyUpstreamResponseError()
	}
	if info.IsStream || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return validateUpstreamTextStream(resp, info)
	}
	return validateUpstreamTextBody(resp, info)
}

func shouldValidateUpstreamTextResponse(info *relaycommon.RelayInfo) bool {
	if info == nil || !info.ChannelSetting.RejectEmptyResponse {
		return false
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions,
		relayconstant.RelayModeCompletions,
		relayconstant.RelayModeResponses,
		relayconstant.RelayModeResponsesCompact,
		relayconstant.RelayModeGemini,
		relayconstant.RelayModeEdits,
		relayconstant.RelayModeUnknown:
		return true
	default:
		return false
	}
}

func validateUpstreamTextBody(resp *http.Response, info *relaycommon.RelayInfo) *types.NewAPIError {
	originalBody := resp.Body
	body, err := io.ReadAll(originalBody)
	_ = originalBody.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	// A few gateways incorrectly label an event stream as JSON. Detect the
	// framing from the buffered body so an empty stream cannot bypass the
	// validator merely because its Content-Type is wrong.
	if looksLikeEventStream(body) {
		return validateUpstreamTextStream(resp, info)
	}
	return ValidateUpstreamTextPayload(body)
}

func looksLikeEventStream(body []byte) bool {
	// Only the first non-empty line determines framing; avoid allocating an
	// index for every line in a potentially large JSON response.
	for line := range bytes.SplitSeq(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		return bytes.HasPrefix(line, []byte("data:")) ||
			bytes.HasPrefix(line, []byte("event:")) ||
			bytes.HasPrefix(line, []byte(":"))
	}
	return false
}

// ValidateUpstreamTextPayload validates one complete JSON or newline-delimited
// text-generation response. Callers must apply the channel setting gate.
func ValidateUpstreamTextPayload(body []byte) *types.NewAPIError {
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	if cyberPolicy, message := service.DetectOpenAICyberPolicy(body); cyberPolicy {
		return newCyberPolicyResponseError(message)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return NewEmptyUpstreamResponseError()
	}
	var payload any
	if err := common.Unmarshal(body, &payload); err == nil {
		if failure := responsesFailureBeforeOutput(payload); failure != nil {
			return failure
		}
		if hasExplicitResponseError(payload) || hasSubstantiveResponse(payload) || hasPositiveResponseOutputUsage(payload) {
			return nil
		}
		return NewEmptyUpstreamResponseError()
	}

	// Ollama and a few compatible providers can return newline-delimited JSON
	// even for non-stream requests. Only classify it when every document parses.
	parsedDocument := false
	for line := range bytes.SplitSeq(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if cyberPolicy, message := service.DetectOpenAICyberPolicy(line); cyberPolicy {
			return newCyberPolicyResponseError(message)
		}
		if err := common.Unmarshal(line, &payload); err != nil {
			return nil
		}
		parsedDocument = true
		if failure := responsesFailureBeforeOutput(payload); failure != nil {
			return failure
		}
		if hasExplicitResponseError(payload) || hasSubstantiveResponse(payload) || hasPositiveResponseOutputUsage(payload) {
			return nil
		}
	}
	if parsedDocument {
		return NewEmptyUpstreamResponseError()
	}
	return nil
}

func validateUpstreamTextStream(resp *http.Response, info *relaycommon.RelayInfo) *types.NewAPIError {
	originalBody := resp.Body
	reader := bufio.NewReader(originalBody)
	var prefix bytes.Buffer
	var lineBuffer bytes.Buffer
	var eventDataLines [][]byte

	idleTimeout := time.Duration(constant.StreamingTimeout) * time.Second
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Second
	}
	idleTimeoutDone := make(chan struct{})
	idleTimer := time.AfterFunc(idleTimeout, func() {
		_ = originalBody.Close()
		close(idleTimeoutDone)
	})
	stopIdleTimer := func() bool {
		if idleTimer.Stop() {
			return true
		}
		<-idleTimeoutDone
		return false
	}

	for {
		fragment, readErr := reader.ReadSlice('\n')
		prefix.Write(fragment)
		lineBuffer.Write(fragment)

		if !stopIdleTimer() {
			return types.NewOpenAIError(
				errors.New("upstream stream produced no substantive output before timeout"),
				types.ErrorCodeEmptyResponse,
				http.StatusBadGateway,
			)
		}
		if prefix.Len() >= getScannerBufferSize() {
			restorePrefixedResponseBody(resp, &prefix, reader, originalBody)
			return nil
		}
		if errors.Is(readErr, bufio.ErrBufferFull) {
			idleTimer.Reset(idleTimeout)
			continue
		}

		line := lineBuffer.String()
		lineBuffer.Reset()
		trimmedLine := strings.TrimSpace(line)
		var payload []byte
		var done, candidate bool
		switch {
		case strings.HasPrefix(trimmedLine, "data:"):
			dataLine := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "data:"))
			eventDataLines = append(eventDataLines, []byte(dataLine))
			if readErr == nil {
				idleTimer.Reset(idleTimeout)
				continue
			}
			payload, done, candidate = responseStreamPayload(string(bytes.Join(eventDataLines, []byte{'\n'})))
			eventDataLines = nil
		case len(eventDataLines) > 0 && trimmedLine == "":
			payload, done, candidate = responseStreamPayload(string(bytes.Join(eventDataLines, []byte{'\n'})))
			eventDataLines = nil
		case len(eventDataLines) > 0 && isEventStreamControlLine(trimmedLine):
			if readErr == nil {
				idleTimer.Reset(idleTimeout)
				continue
			}
			payload, done, candidate = responseStreamPayload(string(bytes.Join(eventDataLines, []byte{'\n'})))
			eventDataLines = nil
		case len(eventDataLines) > 0:
			// Mixed raw/SSE framing is ambiguous. Preserve the full response and
			// leave provider-specific handling to the adaptor.
			restorePrefixedResponseBody(resp, &prefix, reader, originalBody)
			return nil
		default:
			payload, done, candidate = responseStreamPayload(line)
		}
		if done {
			_ = originalBody.Close()
			return NewEmptyUpstreamResponseError()
		}
		if candidate {
			// Preserve the existing first-response timeout semantics while the
			// validator waits for substantive output.
			info.MarkFirstResponseContent()
			if cyberPolicy, message := service.DetectOpenAICyberPolicy(payload); cyberPolicy {
				_ = originalBody.Close()
				return newCyberPolicyResponseError(message)
			}

			var value any
			if err := common.Unmarshal(payload, &value); err != nil {
				restorePrefixedResponseBody(resp, &prefix, reader, originalBody)
				return nil
			}
			if failure := responsesFailureBeforeOutput(value); failure != nil {
				// No buffered event has reached the client. Return the original
				// protocol failure to the normal error-log/retry path instead of
				// forwarding an HTTP 200 stream that has already failed.
				_ = originalBody.Close()
				return failure
			}
			if hasExplicitResponseError(value) {
				restorePrefixedResponseBody(resp, &prefix, reader, originalBody)
				return nil
			}
			if hasSubstantiveResponse(value) || hasPositiveResponseOutputUsage(value) {
				restorePrefixedResponseBody(resp, &prefix, reader, originalBody)
				return nil
			}
			if isTerminalResponseEvent(value) {
				_ = originalBody.Close()
				return NewEmptyUpstreamResponseError()
			}
		}

		if readErr == nil {
			idleTimer.Reset(idleTimeout)
			continue
		}
		if errors.Is(readErr, io.EOF) {
			_ = originalBody.Close()
			return NewEmptyUpstreamResponseError()
		}
		_ = originalBody.Close()
		if info.IsFirstResponseTimedOut() {
			return relaycommon.NewFirstResponseTimeoutError(info)
		}
		return types.NewOpenAIError(readErr, types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
}

func restorePrefixedResponseBody(resp *http.Response, prefix *bytes.Buffer, reader *bufio.Reader, originalBody io.Closer) {
	resp.Body = &prefixedResponseBody{
		Reader: io.MultiReader(bytes.NewReader(prefix.Bytes()), reader),
		closer: originalBody,
	}
}

func responseStreamPayload(line string) (payload []byte, done bool, candidate bool) {
	trimmed := strings.TrimSpace(line)
	if isEventStreamControlLine(trimmed) {
		return nil, false, false
	}
	if strings.HasPrefix(trimmed, "data:") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	}
	if trimmed == "" {
		return nil, false, false
	}
	if strings.EqualFold(trimmed, "[DONE]") {
		return nil, true, false
	}
	return []byte(trimmed), false, true
}

func isEventStreamControlLine(trimmed string) bool {
	return trimmed == "" || strings.HasPrefix(trimmed, ":") || strings.HasPrefix(trimmed, "event:") ||
		strings.HasPrefix(trimmed, "id:") || strings.HasPrefix(trimmed, "retry:")
}

func isTerminalResponseEvent(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	eventType, _ := object["type"].(string)
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "response.completed", "response.done", "response.complete", "response.end", "message_stop":
		return true
	}
	done, _ := object["done"].(bool)
	return done
}

// responsesFailureBeforeOutput bridges explicit Responses failures carried by
// HTTP 200 to the existing relay retry policy. It does not reinterpret provider
// safety results, incomplete/cancelled responses, or arbitrary nested "error"
// fields. Callers still honor RejectEmptyResponse and the controller's written
// response/client cancellation guards; this function never initiates a retry.
func responsesFailureBeforeOutput(value any) *types.NewAPIError {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	eventType, _ := object["type"].(string)
	response := object
	switch eventType {
	case "response.failed", "response.error":
		if nested, ok := object["response"].(map[string]any); ok {
			response = nested
		}
	case "error":
		// Responses error events have flat code/message fields. Other
		// providers' nested error envelopes remain their adaptor's concern.
		if _, ok := object["code"].(string); !ok {
			return nil
		}
	case "":
		if object["object"] != "response" || object["status"] != "failed" {
			return nil
		}
	default:
		return nil
	}
	// A terminal failure can include partial text, completed tools, or billed
	// output. Preserve that response and its existing settlement path rather
	// than replaying work that may already have produced side effects.
	if hasSubstantiveResponse(value) || hasPositiveResponseOutputUsage(value) {
		return nil
	}
	upstreamError := dto.GetOpenAIError(response["error"])
	if upstreamError == nil {
		upstreamError = &types.OpenAIError{}
		upstreamError.Code = object["code"]
		upstreamError.Message, _ = object["message"].(string)
		upstreamError.Param, _ = object["param"].(string)
	}
	code, isStringCode := upstreamError.Code.(string)
	if strings.HasPrefix(code, "channel:") {
		// This prefix belongs to local channel-selection errors, which have
		// special retry rules. Never promote an upstream-supplied code into
		// that namespace; retain the existing adaptor handling instead.
		return nil
	}
	classification := strings.ToLower(strings.TrimSpace(code))
	if upstreamError.Code == nil || isStringCode && classification == "" {
		classification = strings.ToLower(strings.TrimSpace(upstreamError.Type))
	}
	if upstreamError.Code == nil || isStringCode && strings.TrimSpace(code) == "" {
		upstreamError.Code = "response_failed"
	}
	if strings.TrimSpace(upstreamError.Message) == "" {
		upstreamError.Message = "upstream Responses request failed"
	}
	status := http.StatusBadGateway
	var options []types.NewAPIErrorOptions
	switch classification {
	case "upstream_error", "server_error", "internal_error", "internal_server_error", "bad_gateway":
		// HTTP 200 describes the transport, not the failed generation. Use a
		// gateway error so configured retry status ranges can make the decision.
	case "overloaded_error", "service_unavailable":
		status = http.StatusServiceUnavailable
	case "rate_limit_exceeded", "rate_limit_error":
		status = http.StatusTooManyRequests
	case "timeout", "request_timeout", "gateway_timeout":
		status = http.StatusGatewayTimeout
	case "invalid_api_key", "authentication_error":
		status = http.StatusUnauthorized
	case "permission_denied", "permission_error":
		status = http.StatusForbidden
	case "invalid_request_error", "invalid_request", "invalid_argument", "context_length_exceeded",
		"content_filter", "content_policy_violation", "safety_violation", "cyber_policy":
		status = http.StatusBadRequest
		options = append(options, types.ErrOptionWithSkipRetry())
	default:
		// Unknown failures are recorded with their original details, but do
		// not guess that retrying the same request is appropriate.
		options = append(options, types.ErrOptionWithSkipRetry())
	}
	return types.WithOpenAIError(*upstreamError, status, options...)
}

func hasExplicitResponseError(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if hasExplicitResponseError(item) {
				return true
			}
		}
	case map[string]any:
		eventType, _ := typed["type"].(string)
		switch strings.ToLower(strings.TrimSpace(eventType)) {
		case "error", "response.error", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			return true
		}
		for key, item := range typed {
			normalizedKey := normalizeResponseKey(key)
			if isResponseConfigurationKey(normalizedKey) {
				continue
			}
			if normalizedKey == "error" && responseErrorValueIsPresent(item) {
				return true
			}
			// Gemini exposes prompt safety failures as promptFeedback.blockReason.
			// Leave explicit provider outcomes to the adaptor instead of retrying
			// them as transport-level empty responses.
			if normalizedKey == "blockreason" && responseErrorValueIsPresent(item) {
				return true
			}
			if hasExplicitResponseError(item) {
				return true
			}
		}
	}
	return false
}

func responseErrorValueIsPresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case bool:
		return typed
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func hasPositiveResponseOutputUsage(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if hasPositiveResponseOutputUsage(item) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			normalizedKey := normalizeResponseKey(key)
			if normalizedKey == "usage" || normalizedKey == "usagemetadata" {
				if usageContainsOutputTokens(item) {
					return true
				}
				continue
			}
			if normalizedKey == "evalcount" && numericValueIsPositive(item) {
				return true
			}
			if isResponseConfigurationKey(normalizedKey) {
				continue
			}
			if hasPositiveResponseOutputUsage(item) {
				return true
			}
		}
	}
	return false
}

func usageContainsOutputTokens(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if usageContainsOutputTokens(item) {
				return true
			}
		}
	case map[string]any:
		// Only output counters prove that generation occurred. Input, total and
		// cache counters may be positive even when the model returns nothing.
		for key, item := range typed {
			switch normalizeResponseKey(key) {
			case "outputtokens", "completiontokens", "outputtokencount", "completiontokencount",
				"candidatestokencount", "thoughtstokencount":
				// Gemini reports generated thinking separately from candidates.
				if numericValueIsPositive(item) {
					return true
				}
			}
			if usageContainsOutputTokens(item) {
				return true
			}
		}
	}
	return false
}

func isResponseConfigurationKey(key string) bool {
	switch key {
	case "metadata", "instructions", "input", "request", "requestbody", "tools", "toolchoice",
		"parameters", "schema", "jsonschema", "format", "config", "configuration", "settings":
		return true
	default:
		return false
	}
}

func numericValueIsPositive(value any) bool {
	switch number := value.(type) {
	case float64:
		return number > 0
	case float32:
		return number > 0
	case int:
		return number > 0
	case int64:
		return number > 0
	case int32:
		return number > 0
	case uint:
		return number > 0
	case uint64:
		return number > 0
	case uint32:
		return number > 0
	default:
		return false
	}
}

func hasSubstantiveResponse(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if hasSubstantiveContentValue(item) {
				return true
			}
		}
	case map[string]any:
		if responseObjectRepresentsCall(typed) && substantiveCallValue(typed) {
			return true
		}
		for key, item := range typed {
			switch normalizeResponseKey(key) {
			case "content", "text", "output", "outputtext", "response", "completion", "answer", "result",
				"reasoning", "reasoningcontent", "reasoningdetails", "thinking", "analysis", "refusal",
				"partialjson", "transcript", "delta", "arguments":
				if hasSubstantiveContentValue(item) {
					return true
				}
			case "toolcalls", "functioncall", "functioncalls", "tooluse", "functioncalloutput":
				if substantiveCallValue(item) {
					return true
				}
			case "imageurl", "b64json", "inlinedata", "filedata", "audio":
				if responseCollectionIsNonEmpty(item) {
					return true
				}
			case "choices", "choice", "message", "messages", "candidate", "candidates", "parts", "part",
				"item", "items", "data", "payload", "results", "generation", "generations", "reply", "replies",
				"summary", "summaries", "contentblock", "contentblocks", "events", "chunk":
				if hasSubstantiveResponse(item) {
					return true
				}
			}
		}
	}
	return false
}

func hasSubstantiveContentValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case map[string]any:
		return hasSubstantiveResponse(typed)
	case []any:
		for _, item := range typed {
			if hasSubstantiveContentValue(item) {
				return true
			}
		}
	default:
		return false
	}
	return false
}

func responseObjectRepresentsCall(value map[string]any) bool {
	typeName, _ := value["type"].(string)
	typeName = normalizeResponseKey(typeName)
	return strings.HasSuffix(typeName, "call") || strings.Contains(typeName, "tooluse") ||
		strings.Contains(typeName, "calloutput")
}

func substantiveCallValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		for _, item := range typed {
			if substantiveCallValue(item) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			if isCallMetadataKey(normalizeResponseKey(key)) {
				continue
			}
			if substantiveCallPayload(item) {
				return true
			}
		}
	}
	return false
}

func substantiveCallPayload(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case bool:
		return typed
	case float64, float32, int, int64, int32, uint, uint64, uint32:
		return true
	case []any:
		for _, item := range typed {
			if substantiveCallPayload(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if substantiveCallPayload(item) {
				return true
			}
		}
	}
	return false
}

func isCallMetadataKey(key string) bool {
	switch key {
	case "type", "id", "index", "status", "callid", "itemid", "object", "role",
		"created", "createdat", "finishreason":
		return true
	default:
		return false
	}
}

func responseCollectionIsNonEmpty(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return value != nil
	}
}

func normalizeResponseKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

// NewEmptyUpstreamResponseError creates the retryable error shared by HTTP and
// non-HTTP channel transports.
func NewEmptyUpstreamResponseError() *types.NewAPIError {
	return types.NewOpenAIError(
		errors.New("upstream returned an empty response with zero or missing output token usage"),
		types.ErrorCodeEmptyResponse,
		http.StatusBadGateway,
	)
}

func newCyberPolicyResponseError(message string) *types.NewAPIError {
	if strings.TrimSpace(message) == "" {
		message = "request blocked by upstream cyber policy"
	}
	return types.NewOpenAIError(
		errors.New(message),
		types.ErrorCodeCyberPolicy,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}
