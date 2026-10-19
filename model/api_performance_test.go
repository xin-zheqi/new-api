package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Exercise the same path and plugin filters used by the relay distributor.
func BenchmarkRelayChannelFiltering(b *testing.B) {
	for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeAdvancedCustom} {
		b.Run(fmt.Sprintf("type_%d", channelType), func(b *testing.B) {
			channelSyncLock.Lock()
			previousChannels, previousConfigs := channelsIDM, channel2advancedCustomConfig
			channelsIDM = make(map[int]*Channel)
			channel2advancedCustomConfig = make(map[int]*kitdto.AdvancedCustomConfig)
			b.Cleanup(func() {
				channelsIDM, channel2advancedCustomConfig = previousChannels, previousConfigs
				channelSyncLock.Unlock()
			})
			ids := make([]int, 32)
			for i := range ids {
				id := i + 1
				ids[i] = id
				channel := &Channel{Id: id, Type: channelType}
				if channelType == constant.ChannelTypeAdvancedCustom {
					config := &kitdto.AdvancedCustomConfig{Routes: []kitdto.AdvancedCustomRoute{
						{IncomingPath: "/v1/chat/completions", Models: []string{"gpt-4"}},
						{IncomingPath: "/v1/responses", Models: []string{"gpt-4"}},
					}}
					channel.SetOtherSettings(kitdto.ChannelOtherSettings{AdvancedCustom: config})
					channel2advancedCustomConfig[id] = config
				}
				channelsIDM[id] = channel
			}
			filters := []dto.ChannelFilter{
				{Kind: dto.FilterRequestPath, RequestPath: "/v1/chat/completions"},
				{Kind: dto.FilterTaskPluginIdentity},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				kept, _ := filterCandidateIDs(ids, "gpt-4", filters)
				if len(kept) != len(ids) {
					b.Fatal("eligible channels were lost")
				}
			}
		})
	}
}

// Measure a settings save against a real SQLite engine, including transaction
// and runtime publication. SQL/op makes the reduction independent of host load.
func BenchmarkUpdateOptionsBulk(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)
	sqlDB, err := db.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, db.AutoMigrate(&Option{}))
	previousDB := DB
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	DB = db
	b.Cleanup(func() {
		DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(b, sqlDB.Close())
	})
	values := make(map[string]string)
	for i := range 20 {
		values[fmt.Sprintf("perf_fixture_%02d", i)] = "value"
	}
	require.NoError(b, UpdateOptionsBulk(values))
	var statements int
	count := func(*gorm.DB) { statements++ }
	require.NoError(b, db.Callback().Query().After("gorm:query").Register("perf_count", count))
	require.NoError(b, db.Callback().Create().After("gorm:create").Register("perf_count", count))
	require.NoError(b, db.Callback().Update().After("gorm:update").Register("perf_count", count))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := UpdateOptionsBulk(values); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(statements)/float64(b.N), "SQL/op")
}
