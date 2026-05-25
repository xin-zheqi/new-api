package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPreserveRequestedStreamModeKeepsNonStreamRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{IsStream: false}
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"text/event-stream; charset=utf-8"},
		},
	}

	preserveRequestedStreamMode(c, info, resp)

	require.False(t, info.IsStream)
}

func TestPreserveRequestedStreamModeLeavesStreamRequestEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{IsStream: true}
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
	}

	preserveRequestedStreamMode(c, info, resp)

	require.True(t, info.IsStream)
}
