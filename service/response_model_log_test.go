package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseModelMismatchStaysInAdminLogMetadata(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "client-model", ChannelMeta: &relaycommon.ChannelMeta{}}
	for _, upstream := range []string{"client-model", "provider-model"} {
		info.ObserveUpstreamResponseModel(upstream)
		other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
		admin, ok := other["admin_info"].(map[string]interface{})
		require.True(t, ok)
		assert.NotContains(t, other, "response_model_mismatch")
		if upstream == "client-model" {
			assert.NotContains(t, admin, "response_model_mismatch")
			continue
		}
		audit, ok := admin["response_model_mismatch"].(*relaycommon.ResponseModelMismatch)
		require.True(t, ok)
		assert.Equal(t, "client-model", audit.RequestedModel)
		assert.Equal(t, []string{"provider-model"}, audit.ResponseModels)
		errorOther := map[string]interface{}{}
		AppendRelayLogAdminInfo(c, info, errorOther)
		assert.Equal(t, audit, errorOther["admin_info"].(map[string]interface{})["response_model_mismatch"])
	}
}
