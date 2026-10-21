package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesStreamReasoningRepairPreservesUsageAndEvents(t *testing.T) {
	events := []string{
		`{"type":"response.reasoning_text.delta","item_id":"r1","output_index":0,"content_index":0,"delta":"actual thought"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"r1","type":"reasoning","content":null}}`,
		`{"type":"response.completed","response":{"id":"resp1","status":"completed","output":[{"id":"r1","type":"reasoning","content":null}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"output_tokens_details":{"reasoning_tokens":15}}}}`,
	}
	for _, enabled := range []bool{false, true} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		info := &relaycommon.RelayInfo{OriginModelName: "deepseek-v4.1-flash", ReasoningContentRepairActive: enabled, DisablePing: true}
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + strings.Join(events, "\n\ndata: ") + "\n\n"))}
		usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
		require.Nil(t, apiErr)
		require.NotNil(t, usage)
		assert.Equal(t, 10, usage.PromptTokens)
		assert.Equal(t, 20, usage.CompletionTokens)
		assert.Equal(t, 15, usage.CompletionTokenDetails.ReasoningTokens)
		var forwarded []string
		for _, line := range strings.Split(w.Body.String(), "\n") {
			if strings.HasPrefix(line, "data: ") {
				forwarded = append(forwarded, strings.TrimPrefix(line, "data: "))
			}
		}
		require.Len(t, forwarded, len(events))
		assert.Equal(t, events[0], forwarded[0])
		if enabled {
			assert.Equal(t, "actual thought", gjson.Get(forwarded[1], "item.content.0.text").String())
			assert.Equal(t, "actual thought", gjson.Get(forwarded[2], "response.output.0.content.0.text").String())
		} else {
			assert.Equal(t, events, forwarded)
		}
	}
}
