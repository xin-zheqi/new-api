package relay

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func preserveRequestedStreamMode(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) {
	if info == nil || resp == nil {
		return
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		return
	}
	if info.IsStream {
		return
	}
	logger.LogWarn(c, "upstream returned text/event-stream for a non-stream request; preserving downstream non-stream handling")
}
