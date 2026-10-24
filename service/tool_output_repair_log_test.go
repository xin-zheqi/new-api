package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestToolOutputRepairAuditContainsOnlyCountsAndOnlyWhenApplied(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	for _, stats := range []*relaycommon.ToolOutputRepairStats{nil, {MissingOutputs: 2, RelinkedOutputs: 1}, {RenamedCalls: 2}} {
		info.ToolOutputRepairStats = stats
		other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
		encoded, err := common.Marshal(other)
		require.NoError(t, err)
		marker := gjson.GetBytes(encoded, "admin_info.tool_output_repair")
		if stats == nil {
			assert.False(t, marker.Exists())
			continue
		}
		if stats.RenamedCalls > 0 {
			assert.JSONEq(t, `{"missing_outputs":0,"relinked_outputs":0,"renamed_calls":2}`, marker.Raw)
		} else {
			assert.JSONEq(t, `{"missing_outputs":2,"relinked_outputs":1}`, marker.Raw)
		}
		assert.False(t, gjson.GetBytes(encoded, "tool_output_repair").Exists(), "repair diagnostics stay in the admin-only log section")
	}
}
