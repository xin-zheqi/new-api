package model

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestChannelSchedulePreservesIndependentStatus(t *testing.T) {
	instant := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	for _, status := range []int{common.ChannelStatusEnabled, common.ChannelStatusManuallyDisabled, common.ChannelStatusAutoDisabled} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			channel := &Channel{Id: 5, Status: status}
			channel.SetSetting(kitdto.ChannelSettings{ReasoningContentRepair: true, ChannelSchedule: &kitdto.ChannelSchedule{Timezone: "UTC", Periods: []kitdto.ChannelSchedulePeriod{{Start: "09:00", End: "11:00"}}}})
			assert.True(t, channel.IsScheduleActiveAt(instant))
			assert.Equal(t, status == common.ChannelStatusEnabled, channel.IsAvailableAt(instant))
			assert.False(t, channel.IsAvailableAt(instant.Add(2*time.Hour)))
			channel.SetScheduleReadState(instant.Add(2 * time.Hour))
			require.NotNil(t, channel.ScheduleActive)
			assert.False(t, *channel.ScheduleActive)
			assert.Equal(t, status, channel.Status)
			channel.SetSetting(kitdto.ChannelSettings{ReasoningContentRepair: true})
			assert.Equal(t, status == common.ChannelStatusEnabled, channel.IsAvailableAt(instant))
			channel.SetScheduleReadState(instant)
			assert.Nil(t, channel.ScheduleActive)
			assert.True(t, channel.GetSetting().ReasoningContentRepair)
		})
	}
}

func TestChannelScheduleInvalidStoredConfigurationFailsClosedLocally(t *testing.T) {
	for _, setting := range []string{
		`{"channel_schedule":{"timezone":"Unknown/Nowhere","periods":[{"start":"09:00","end":"17:00"}]}}`,
		`{"channel_schedule":{"timezone":"UTC","periods":[]}}`,
		`{"channel_schedule":"not-an-object"}`,
		`{"\u0063hannel_schedule":{"timezone":"UTC","periods":[]}}`,
	} {
		channel := &Channel{Status: 1, Setting: &setting}
		require.Error(t, channel.ValidateSettings())
		assert.False(t, channel.IsAvailableAt(time.Time{}))
		assert.Equal(t, setting, *channel.Setting)
		assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
	}
	assert.True(t, (&Channel{Status: 1}).IsAvailableAt(time.Time{}))
}

// Verify the existing text settings column and both routing implementations
// against real dialects. No schema or per-minute status writes are introduced.
func TestChannelScheduleDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "schedule_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousMemory, previousType := DB, common.MemoryCacheEnabled, common.MainDatabaseType()
			previousChannels, previousGroups, previousConfigs := channelsIDM, group2model2channels, channel2advancedCustomConfig
			DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				assert.NoError(t, db.Migrator().DropTable(&Ability{}, &Channel{}))
				DB, common.MemoryCacheEnabled = previousDB, previousMemory
				common.SetMainDatabaseType(previousType)
				initCol()
				channelSyncLock.Lock()
				channelsIDM, group2model2channels, channel2advancedCustomConfig = previousChannels, previousGroups, previousConfigs
				channelSyncLock.Unlock()
				assert.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)

			priorityHigh, priorityLow := int64(100), int64(1)
			scheduled := Channel{Name: "scheduled", Key: "scheduled-key", Status: 1, Group: "schedule-group", Models: "schedule-model", Priority: &priorityHigh}
			fallback := Channel{Name: "fallback", Key: "fallback-key", Status: 1, Group: "schedule-group", Models: "schedule-model", Priority: &priorityLow}
			require.NoError(t, scheduled.Insert())
			require.NoError(t, fallback.Insert())
			assert.Nil(t, scheduled.GetSetting().ChannelSchedule)
			closedStart := time.Now().UTC().Add(12 * time.Minute)
			settings := kitdto.ChannelSettings{ReasoningContentRepair: true, ChannelSchedule: &kitdto.ChannelSchedule{Timezone: "UTC", Periods: []kitdto.ChannelSchedulePeriod{{Start: closedStart.Format("15:04"), End: closedStart.Add(time.Minute).Format("15:04")}}}}
			scheduled.SetSetting(settings)
			require.NoError(t, scheduled.ValidateSettings())
			require.NoError(t, db.Model(&scheduled).Select("setting").Updates(&scheduled).Error)
			var loaded Channel
			require.NoError(t, db.First(&loaded, scheduled.Id).Error)
			assert.Equal(t, settings, loaded.GetSetting())
			assert.False(t, loaded.IsAvailable())
			assert.Equal(t, 1, loaded.Status)

			for _, memory := range []bool{false, true} {
				t.Run(fmt.Sprintf("memory=%v", memory), func(t *testing.T) {
					common.MemoryCacheEnabled = memory
					InitChannelCache()
					selected, err := GetRandomSatisfiedChannel("schedule-group", "schedule-model", 0, nil)
					require.NoError(t, err)
					require.NotNil(t, selected)
					assert.Equal(t, fallback.Id, selected.Id)
					assert.Equal(t, "fallback-key", selected.Key)
					assert.False(t, IsChannelEnabledForGroupModel("schedule-group", "schedule-model", scheduled.Id))
					assert.False(t, IsChannelEnabledForAnyGroupModel([]string{"schedule-group"}, "schedule-model", scheduled.Id))
					assert.True(t, IsChannelEnabledForGroupModel("schedule-group", "schedule-model", fallback.Id))
				})
			}
			// A health recovery status write cannot open a closed schedule.
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", scheduled.Id).Update("status", common.ChannelStatusEnabled).Error)
			require.NoError(t, db.First(&loaded, scheduled.Id).Error)
			assert.False(t, loaded.IsAvailable())
			// Clearing just the schedule restores admission without changing keys,
			// priorities, other compatibility switches, or a different channel.
			settings.ChannelSchedule = nil
			loaded.SetSetting(settings)
			require.NoError(t, db.Model(&loaded).Select("setting").Updates(&loaded).Error)
			require.NoError(t, db.First(&loaded, scheduled.Id).Error)
			assert.NotContains(t, *loaded.Setting, "channel_schedule")
			assert.True(t, loaded.GetSetting().ReasoningContentRepair)
			assert.Equal(t, "scheduled-key", loaded.Key)
			for _, memory := range []bool{false, true} {
				common.MemoryCacheEnabled = memory
				InitChannelCache()
				selected, err := GetRandomSatisfiedChannel("schedule-group", "schedule-model", 0, nil)
				require.NoError(t, err)
				require.NotNil(t, selected)
				assert.Equal(t, scheduled.Id, selected.Id)
			}
		})
	}
}
