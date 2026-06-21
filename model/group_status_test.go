package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetGroupStatusTimelinesBucketsSuccessErrorsAndEmptyAsHealthy(t *testing.T) {
	truncateTables(t)

	logs := []*Log{
		{Group: "default", Type: LogTypeConsume, CreatedAt: 1000},
		{Group: "default", Type: LogTypeError, CreatedAt: 1010},
		{Group: "default", Type: LogTypeConsume, CreatedAt: 1300},
		{Group: "vip", Type: LogTypeError, CreatedAt: 1600},
		{Group: "default", Type: LogTypeManage, CreatedAt: 1000},
		{Group: "other", Type: LogTypeConsume, CreatedAt: 1000},
	}
	for _, log := range logs {
		require.NoError(t, LOG_DB.Create(log).Error)
	}

	timelines, err := GetGroupStatusTimelines([]string{"default", "vip"}, 1000, 1900, 300)
	require.NoError(t, err)
	require.Len(t, timelines, 2)

	byGroup := map[string]GroupStatusTimeline{}
	for _, timeline := range timelines {
		byGroup[timeline.Group] = timeline
	}

	defaultTimeline := byGroup["default"]
	require.Len(t, defaultTimeline.Buckets, 3)
	require.EqualValues(t, 2, defaultTimeline.Buckets[0].Total)
	require.EqualValues(t, 1, defaultTimeline.Buckets[0].Success)
	require.InDelta(t, 50, defaultTimeline.Buckets[0].SuccessRate, 0.001)
	require.False(t, defaultTimeline.Buckets[0].NoRequests)
	require.EqualValues(t, 1, defaultTimeline.Buckets[1].Total)
	require.InDelta(t, 100, defaultTimeline.Buckets[1].SuccessRate, 0.001)
	require.True(t, defaultTimeline.Buckets[2].NoRequests)
	require.InDelta(t, 100, defaultTimeline.Buckets[2].SuccessRate, 0.001)
	require.InDelta(t, 100, defaultTimeline.CurrentRate, 0.001)

	vipTimeline := byGroup["vip"]
	require.Len(t, vipTimeline.Buckets, 3)
	require.True(t, vipTimeline.Buckets[0].NoRequests)
	require.EqualValues(t, 1, vipTimeline.Buckets[2].Total)
	require.EqualValues(t, 0, vipTimeline.Buckets[2].Success)
	require.InDelta(t, 0, vipTimeline.Buckets[2].SuccessRate, 0.001)
	require.InDelta(t, 0, vipTimeline.CurrentRate, 0.001)
}
