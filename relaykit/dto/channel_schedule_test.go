package dto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelScheduleLocalBoundariesAndDST(t *testing.T) {
	for _, tc := range []struct {
		name     string
		timezone string
		periods  []ChannelSchedulePeriod
		instant  string
		active   bool
	}{
		{"before start", "Asia/Shanghai", []ChannelSchedulePeriod{{"09:00", "17:00"}}, "2026-11-11T00:59:59Z", false},
		{"inclusive start", "Asia/Shanghai", []ChannelSchedulePeriod{{"09:00", "17:00"}}, "2026-11-11T01:00:00Z", true},
		{"before end", "Asia/Shanghai", []ChannelSchedulePeriod{{"09:00", "17:00"}}, "2026-11-11T08:59:59Z", true},
		{"exclusive end", "Asia/Shanghai", []ChannelSchedulePeriod{{"09:00", "17:00"}}, "2026-11-11T09:00:00Z", false},
		{"overnight evening", "UTC", []ChannelSchedulePeriod{{"22:00", "02:00"}}, "2026-11-11T23:00:00Z", true},
		{"overnight next day", "UTC", []ChannelSchedulePeriod{{"22:00", "02:00"}}, "2026-11-12T01:59:59Z", true},
		{"overnight end", "UTC", []ChannelSchedulePeriod{{"22:00", "02:00"}}, "2026-11-12T02:00:00Z", false},
		{"multiple windows gap", "UTC", []ChannelSchedulePeriod{{"09:00", "12:00"}, {"14:00", "18:00"}}, "2026-11-11T13:00:00Z", false},
		{"multiple windows second", "UTC", []ChannelSchedulePeriod{{"09:00", "12:00"}, {"14:00", "18:00"}}, "2026-11-11T14:00:00Z", true},
		{"whole day midnight", "UTC", []ChannelSchedulePeriod{{"00:00", "24:00"}}, "2026-11-11T00:00:00Z", true},
		{"whole day end", "UTC", []ChannelSchedulePeriod{{"00:00", "24:00"}}, "2026-11-11T23:59:59Z", true},
		{"DST repeated first hour", "America/New_York", []ChannelSchedulePeriod{{"01:00", "02:00"}}, "2026-11-01T05:30:00Z", true},
		{"DST repeated second hour", "America/New_York", []ChannelSchedulePeriod{{"01:00", "02:00"}}, "2026-11-01T06:30:00Z", true},
		{"DST spring skips nonexistent hour", "America/New_York", []ChannelSchedulePeriod{{"02:00", "03:00"}}, "2026-03-08T07:00:00Z", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schedule, err := (&ChannelSchedule{Timezone: tc.timezone, Periods: tc.periods}).Compile()
			require.NoError(t, err)
			instant, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			assert.Equal(t, tc.active, schedule.IsActive(instant))
		})
	}
	var absent *ChannelSchedule
	compiled, err := absent.Compile()
	require.NoError(t, err)
	assert.True(t, compiled.IsActive(time.Time{}))
}

func TestChannelScheduleRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		schedule ChannelSchedule
	}{
		{"empty schedule", ChannelSchedule{}},
		{"no periods", ChannelSchedule{Timezone: "UTC"}},
		{"too many periods", ChannelSchedule{Timezone: "UTC", Periods: make([]ChannelSchedulePeriod, MaxChannelSchedulePeriods+1)}},
		{"missing timezone", ChannelSchedule{Periods: []ChannelSchedulePeriod{{"09:00", "17:00"}}}},
		{"machine-local timezone", ChannelSchedule{Timezone: "Local", Periods: []ChannelSchedulePeriod{{"09:00", "17:00"}}}},
		{"unknown timezone", ChannelSchedule{Timezone: "Unknown/Nowhere", Periods: []ChannelSchedulePeriod{{"09:00", "17:00"}}}},
		{"equal times", ChannelSchedule{Timezone: "UTC", Periods: []ChannelSchedulePeriod{{"09:00", "09:00"}}}},
		{"invalid start24", ChannelSchedule{Timezone: "UTC", Periods: []ChannelSchedulePeriod{{"24:00", "09:00"}}}},
		{"invalid end24", ChannelSchedule{Timezone: "UTC", Periods: []ChannelSchedulePeriod{{"09:00", "24:01"}}}},
		{"missing leading zero", ChannelSchedule{Timezone: "UTC", Periods: []ChannelSchedulePeriod{{"9:00", "17:00"}}}},
		{"seconds unsupported", ChannelSchedule{Timezone: "UTC", Periods: []ChannelSchedulePeriod{{"09:00:00", "17:00"}}}},
		{"invalid minute", ChannelSchedule{Timezone: "UTC", Periods: []ChannelSchedulePeriod{{"09:60", "17:00"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.schedule.Compile()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "channel_schedule")
		})
	}
}
