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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
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
// enabled channel produces substantive output or reports non-zero token usage.
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
	for _, line := range bytes.Split(body, []byte{'\n'}) {
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
		if hasExplicitResponseError(payload) || hasSubstantiveResponse(payload) || hasNonZeroResponseUsage(payload) {
			return nil
		}
		return NewEmptyUpstreamResponseError()
	}

	// Ollama and a few compatible providers can return newline-delimited JSON
	// even for non-stream requests. Only classify it when every document parses.
	parsedDocument := false
	for _, line := range bytes.Split(body, []byte{'\n'}) {
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
		if hasExplicitResponseError(payload) || hasSubstantiveResponse(payload) || hasNonZeroResponseUsage(payload) {
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
			if hasExplicitResponseError(value) {
				restorePrefixedResponseBody(resp, &prefix, reader, originalBody)
				return nil
			}
			if hasSubstantiveResponse(value) || hasNonZeroResponseUsage(value) {
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
		case "error", "response.error", "response.failed", "response.incomplete", "response.cancelled":
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

func hasNonZeroResponseUsage(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if hasNonZeroResponseUsage(item) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			normalizedKey := normalizeResponseKey(key)
			if normalizedKey == "usage" || normalizedKey == "usagemetadata" {
				if usageContainsNonZeroTokens(item) {
					return true
				}
				continue
			}
			if (normalizedKey == "evalcount" || normalizedKey == "promptevalcount") && numericValueIsNonZero(item) {
				return true
			}
			if isResponseConfigurationKey(normalizedKey) {
				continue
			}
			if hasNonZeroResponseUsage(item) {
				return true
			}
		}
	}
	return false
}

func usageContainsNonZeroTokens(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if usageContainsNonZeroTokens(item) {
				return true
			}
		}
	case map[string]any:
		// Main input/output counters are authoritative when a provider includes
		// them. Anthropic reports cache counters separately, and those details
		// must not override explicit zero primary counters on an empty response.
		if nonZero, present := primaryUsageCountersAreNonZero(typed); present {
			return nonZero
		}
		for key, item := range typed {
			if isTokenUsageCounterKey(normalizeResponseKey(key)) && numericValueIsNonZero(item) {
				return true
			}
			if usageContainsNonZeroTokens(item) {
				return true
			}
		}
	}
	return false
}

func primaryUsageCountersAreNonZero(value map[string]any) (nonZero bool, present bool) {
	for key, item := range value {
		switch normalizeResponseKey(key) {
		case "inputtokens", "outputtokens", "prompttokens", "completiontokens",
			"inputtokencount", "outputtokencount", "prompttokencount", "candidatestokencount":
			present = true
			if numericValueIsNonZero(item) {
				nonZero = true
			}
		}
	}
	return nonZero, present
}

func isTokenUsageCounterKey(key string) bool {
	return strings.HasSuffix(key, "tokens") || strings.HasSuffix(key, "tokencount")
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

func numericValueIsNonZero(value any) bool {
	switch number := value.(type) {
	case float64:
		return number != 0
	case float32:
		return number != 0
	case int:
		return number != 0
	case int64:
		return number != 0
	case int32:
		return number != 0
	case uint:
		return number != 0
	case uint64:
		return number != 0
	case uint32:
		return number != 0
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
		errors.New("upstream returned an empty response with zero token usage"),
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
