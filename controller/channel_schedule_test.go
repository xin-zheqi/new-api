package controller

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomaticChannelTestsRespectIndependentScheduleGate(t *testing.T) {
	future := time.Now().UTC().Add(12 * time.Minute)
	closed := &dto.ChannelSchedule{Timezone: "UTC", Periods: []dto.ChannelSchedulePeriod{{Start: future.Format("15:04"), End: future.Add(time.Minute).Format("15:04")}}}
	fullDay := &dto.ChannelSchedule{Timezone: "UTC", Periods: []dto.ChannelSchedulePeriod{{Start: "00:00", End: "24:00"}}}
	closedEnabled := &model.Channel{Id: 1, Status: common.ChannelStatusEnabled}
	closedEnabled.SetSetting(dto.ChannelSettings{ChannelSchedule: closed})
	closedRecovery := &model.Channel{Id: 2, Status: common.ChannelStatusAutoDisabled}
	closedRecovery.SetSetting(dto.ChannelSettings{ChannelSchedule: closed})
	openEnabled := &model.Channel{Id: 3, Status: common.ChannelStatusEnabled}
	openEnabled.SetSetting(dto.ChannelSettings{ChannelSchedule: fullDay})
	openRecovery := &model.Channel{Id: 4, Status: common.ChannelStatusAutoDisabled}
	openRecovery.SetSetting(dto.ChannelSettings{ChannelSchedule: fullDay})
	manualDisabled := &model.Channel{Id: 5, Status: common.ChannelStatusManuallyDisabled}
	manualDisabled.SetSetting(dto.ChannelSettings{ChannelSchedule: fullDay})
	ordinary := &model.Channel{Id: 6, Status: common.ChannelStatusEnabled}
	channels := []*model.Channel{closedEnabled, closedRecovery, openEnabled, openRecovery, manualDisabled, ordinary}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)
	assert.Equal(t, []*model.Channel{openEnabled, openRecovery, ordinary}, selected)
	selected = selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)
	assert.Equal(t, []*model.Channel{openRecovery}, selected)
	// A channel may leave its window after queue selection. The worker checks
	// admission again and performs no upstream test or recovery write.
	result := testChannelForHealthCheck(context.Background(), closedRecovery, 1, true, 0)
	assert.Equal(t, channelTestSummary{}, result)
	assert.Equal(t, common.ChannelStatusAutoDisabled, closedRecovery.Status)
}

func TestAdminChannelReadReportsScheduleWithoutReplacingStatus(t *testing.T) {
	future := time.Now().UTC().Add(12 * time.Minute)
	channel := &model.Channel{Id: 1, Key: "fixture-key", Status: common.ChannelStatusEnabled}
	channel.SetSetting(dto.ChannelSettings{ChannelSchedule: &dto.ChannelSchedule{Timezone: "UTC", Periods: []dto.ChannelSchedulePeriod{{Start: future.Format("15:04"), End: future.Add(time.Minute).Format("15:04")}}}})
	sanitizeChannelForRead(channel, false)
	require.NotNil(t, channel.ScheduleActive)
	assert.False(t, *channel.ScheduleActive)
	assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
	assert.Nil(t, channel.Setting, "restricted readers must not receive sensitive settings")
	assert.Empty(t, channel.Key)
}
