package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMonitorSetting_ChannelTestEnabledEnvOverridesEnabledConfig(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("CHANNEL_TEST_ENABLED", "false")
	t.Setenv("CHANNEL_TEST_FREQUENCY", "5")
	monitorSetting = MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 20,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.False(t, setting.AutoTestChannelEnabled)
	assert.Equal(t, float64(5), setting.AutoTestChannelMinutes)
}

func TestGetMonitorSetting_ChannelTestEnabledEnvCanEnableDisabledConfig(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("CHANNEL_TEST_ENABLED", "true")
	monitorSetting = MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 12,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.True(t, setting.AutoTestChannelEnabled)
	assert.Equal(t, float64(12), setting.AutoTestChannelMinutes)
}

func TestGetMonitorSetting_LegacyAutoDisabledFlagMapsToPassiveRecovery(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	monitorSetting = MonitorSetting{
		ChannelTestMode:          ChannelTestModeScheduledAll,
		AutoTestOnlyAutoDisabled: true,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.Equal(t, ChannelTestModePassiveRecovery, setting.ChannelTestMode)
	assert.True(t, setting.AutoTestOnlyAutoDisabled)
}

func TestGetMonitorSetting_ChannelTestModeSyncsLegacyFlag(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	monitorSetting = MonitorSetting{
		ChannelTestMode:          ChannelTestModePassiveRecovery,
		AutoTestOnlyAutoDisabled: false,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.Equal(t, ChannelTestModePassiveRecovery, setting.ChannelTestMode)
	assert.True(t, setting.AutoTestOnlyAutoDisabled)
}

func TestIsMinuteInAutoTestChannelTimeRange(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		minute int
		want   bool
	}{
		{name: "inside normal range", value: "08:00-23:59", minute: 8 * 60, want: true},
		{name: "outside normal range", value: "08:00-23:59", minute: 7*60 + 59, want: false},
		{name: "inside cross midnight before midnight", value: "22:00-06:00", minute: 23 * 60, want: true},
		{name: "inside cross midnight after midnight", value: "22:00-06:00", minute: 5 * 60, want: true},
		{name: "outside cross midnight", value: "22:00-06:00", minute: 12 * 60, want: false},
		{name: "same start and end means all day", value: "00:00-00:00", minute: 12 * 60, want: true},
		{name: "invalid range falls back to allowed", value: "bad", minute: 12 * 60, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMinuteInAutoTestChannelTimeRange(tt.minute, tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseAutoTestChannelTimeRange(t *testing.T) {
	start, end, err := ParseAutoTestChannelTimeRange("08:00-23:59")

	require.NoError(t, err)
	assert.Equal(t, 8*60, start)
	assert.Equal(t, 23*60+59, end)

	_, _, err = ParseAutoTestChannelTimeRange("24:00-08:00")
	require.Error(t, err)
}
