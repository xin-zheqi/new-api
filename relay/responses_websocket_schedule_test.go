package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExistingResponsesWebSocketRejectsNewTurnOutsideSchedule(t *testing.T) {
	database := setupRelayChannelDB(t)
	future := time.Now().UTC().Add(12 * time.Minute)
	channel := &model.Channel{Name: "scheduled-ws", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "fixture-key"}
	channel.SetSetting(dto.ChannelSettings{ResponsesWebSocketEnabled: true, ChannelSchedule: &dto.ChannelSchedule{Timezone: "UTC", Periods: []dto.ChannelSchedulePeriod{{Start: future.Format("15:04"), End: future.Add(time.Minute).Format("15:04")}}}})
	require.NoError(t, database.Create(channel).Error)
	previousMemory := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemory })
	common.MemoryCacheEnabled = false
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	session := &responsesWSSession{lockedChannelID: channel.Id, lockedKey: channel.Key, lockedModel: "model"}
	apiErr := session.restoreConnectionContext(c, "model")
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
}
