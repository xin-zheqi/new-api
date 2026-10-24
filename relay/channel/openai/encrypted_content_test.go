package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNativeResponsesPreserveOpaqueOutput(t *testing.T) {
	const output = `[{"type":"reasoning","id":"rs_fixture","summary":[],"encrypted_content":"gAAAAAB-provider-model+/=\\opaque\u0026"},{"type":"compaction","id":"cmp_fixture","encrypted_content":"gAAAAAB-compact+/="},{"type":"message","role":"assistant","id":"amsg_fixture","content":[{"type":"output_text","text":"original"}],"encrypted_content":"gAAAAAB-message+/="}]`
	const response = `{"id":"resp_fixture","object":"response","model":"provider-model","status":"completed","output":` + output + `,"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`
	for _, mode := range []string{"buffered", "stream", "compact"} {
		for _, repair := range []bool{false, true} {
			name := mode + "/off"
			if repair {
				name = mode + "/on"
			}
			t.Run(name, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				info := mappedResponseTestInfo()
				info.ReasoningContentRepairActive = repair
				wire, contentType := response, "application/json"
				if mode == "stream" {
					// A plaintext delta must not be used to fill the encrypted item.
					events := []string{
						`{"type":"response.output_item.added","output_index":0,"item":` + gjson.Get(output, "0").Raw + `}`,
						`{"type":"response.reasoning_text.delta","output_index":0,"item_id":"rs_fixture","content_index":0,"delta":"do not insert"}`,
						`{"type":"response.output_item.done","output_index":0,"item":` + gjson.Get(output, "0").Raw + `}`,
						`{"type":"response.completed","response":` + response + `}`,
					}
					wire, contentType = "data: "+strings.Join(events, "\n\ndata: ")+"\n\n", "text/event-stream"
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(wire))}
				handler := OaiResponsesHandler
				if mode == "stream" {
					handler = OaiResponsesStreamHandler
				} else if mode == "compact" {
					handler = OaiResponsesCompactionHandler
				}
				usage, apiErr := handler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, 2, usage.PromptTokens)
				assert.Equal(t, 3, usage.CompletionTokens)
				assert.Equal(t, 5, usage.TotalTokens)
				result := w.Body.String()
				if mode == "stream" {
					result = ""
					for _, line := range strings.Split(w.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data:") {
							continue
						}
						event := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
						if item := gjson.Get(event, "item"); item.Exists() {
							assert.Equal(t, gjson.Get(output, "0").Raw, item.Raw)
						}
						if gjson.Get(event, "type").String() == "response.completed" {
							result = gjson.Get(event, "response").Raw
						}
					}
				}
				require.NotEmpty(t, result)
				assert.Equal(t, output, gjson.Get(result, "output").Raw, "opaque output retains its exact bytes")
				assert.Equal(t, "public-alias", gjson.Get(result, "model").String())
			})
		}
	}
}
