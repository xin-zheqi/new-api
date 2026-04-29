package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))
	normalizeEventStreamHeaders := shouldNormalizeEventStreamHeaders(c, src)

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		for k, v := range src.Header {
			// avoid setting Content-Length
			if k == "Content-Length" {
				continue
			}
			if normalizeEventStreamHeaders && shouldSkipStreamHeader(k) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}
	if normalizeEventStreamHeaders {
		c.Writer.Header().Set("Content-Type", detectResponseContentType(data))
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}

func shouldNormalizeEventStreamHeaders(c *gin.Context, src *http.Response) bool {
	if c == nil || src == nil {
		return false
	}
	if common.GetContextKeyBool(c, constant.ContextKeyIsStream) {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(src.Header.Get("Content-Type")))
	return strings.HasPrefix(contentType, "text/event-stream")
}

func shouldSkipStreamHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Content-Type", "Cache-Control", "Connection", "Transfer-Encoding", "X-Accel-Buffering":
		return true
	default:
		return false
	}
}

func detectResponseContentType(data []byte) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "application/json"
	}
	switch trimmed[0] {
	case '{', '[':
		return "application/json"
	default:
		return http.DetectContentType(data)
	}
}
