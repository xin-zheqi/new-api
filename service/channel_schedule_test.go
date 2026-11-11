package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinnedChannelsCannotBypassScheduleAdmission(t *testing.T) {
	database := setupChannelSelectTestDB(t)
	future := time.Now().UTC().Add(12 * time.Minute)
	channel := &model.Channel{Name: "scheduled-pin", Key: "fixture", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "default", Models: "model"}
	settings := kitdto.ChannelSettings{ChannelSchedule: &kitdto.ChannelSchedule{Timezone: "UTC", Periods: []kitdto.ChannelSchedulePeriod{{Start: future.Format("15:04"), End: future.Add(time.Minute).Format("15:04")}}}}
	channel.SetSetting(settings)
	require.NoError(t, database.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	for _, memory := range []bool{false, true} {
		common.MemoryCacheEnabled = memory
		model.InitChannelCache()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: channel.Id, Source: dto.PinSourceToken, Rank: dto.PinRankToken})
		selected, _, selectErr := SelectChannelForRequest(c, "model", &RetryParam{Ctx: c, TokenGroup: "default", ModelName: "model"})
		require.NotNil(t, selectErr)
		assert.Equal(t, http.StatusForbidden, selectErr.StatusCode)
		assert.Nil(t, selected)
	}
	// Removing the schedule permits the same pinned channel again. No status
	// transition or unrelated channel change is needed.
	settings.ChannelSchedule = nil
	channel.SetSetting(settings)
	require.NoError(t, database.Model(channel).Update("setting", channel.Setting).Error)
	model.InitChannelCache()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	GetChannelConstraints(c).AddPin(dto.ChannelPin{ChannelId: channel.Id, Source: dto.PinSourceToken, Rank: dto.PinRankToken})
	selected, _, selectErr := SelectChannelForRequest(c, "model", &RetryParam{Ctx: c, TokenGroup: "default", ModelName: "model"})
	require.Nil(t, selectErr)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)
}
