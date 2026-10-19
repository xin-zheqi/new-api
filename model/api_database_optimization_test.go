package model

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestAPIOptimizationDatabaseMatrix(t *testing.T) {
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
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "api_perf_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousType, previousMemory := DB, common.MainDatabaseType(), common.MemoryCacheEnabled
			previousChannels, previousGroups, previousConfigs := channelsIDM, group2model2channels, channel2advancedCustomConfig
			common.OptionMapRWMutex.Lock()
			previousOptions := common.OptionMap
			common.OptionMap = make(map[string]string)
			common.OptionMapRWMutex.Unlock()
			DB, common.MemoryCacheEnabled = db, false
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&Ability{}, &Channel{}, &Option{}))
				DB, common.MemoryCacheEnabled = previousDB, previousMemory
				common.SetMainDatabaseType(previousType)
				initCol()
				channelSyncLock.Lock()
				channelsIDM, group2model2channels, channel2advancedCustomConfig = previousChannels, previousGroups, previousConfigs
				channelSyncLock.Unlock()
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &Option{}))
			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s %s", dialect, version)

			t.Run("routing preserves eligibility and selected credentials", func(t *testing.T) {
				channels := []Channel{
					{Id: 940001, Type: constant.ChannelTypeOpenAI, Models: "ordinary"},
					{Id: 940002, Type: constant.ChannelTypeAdvancedCustom, Models: "custom"},
					{Id: 940003, Type: constant.ChannelTypeAdvancedCustom, Models: "custom"},
					{Id: 940004, Type: constant.ChannelTypeTaskPlugin, Models: "plugin", Setting: common.GetPointer(`{"task_plugin_key":"alpha"}`)},
					{Id: 940005, Type: constant.ChannelTypeTaskPlugin, Models: "plugin", Setting: common.GetPointer(`{"task_plugin_key":"beta"}`)},
				}
				for i := range channels {
					ch := &channels[i]
					ch.Name, ch.Key = fmt.Sprintf("channel-%d", ch.Id), fmt.Sprintf("credential-%d", ch.Id)
					ch.Group, ch.Status, ch.UsedQuota = "perf", common.ChannelStatusEnabled, 12345
					ch.ParamOverride = common.GetPointer(`{"temperature":0}`)
					if ch.Type == constant.ChannelTypeAdvancedCustom {
						path := "/v1/responses"
						if i == 2 {
							path = "/v1/messages"
						}
						ch.SetOtherSettings(kitdto.ChannelOtherSettings{AdvancedCustom: &kitdto.AdvancedCustomConfig{Routes: []kitdto.AdvancedCustomRoute{{IncomingPath: path}}}})
						ch.Setting = common.GetPointer(`{"responses_websocket_enabled":true}`)
					}
					require.NoError(t, db.Create(ch).Error)
					require.NoError(t, ch.AddAbilities(db))
				}
				for _, cached := range []bool{false, true} {
					common.MemoryCacheEnabled = cached
					InitChannelCache()
					for _, tc := range []struct {
						model   string
						filters []dto.ChannelFilter
						want    int
					}{
						{"ordinary", []dto.ChannelFilter{{Kind: dto.FilterRequestPath, RequestPath: "/v1/chat/completions"}}, 940001},
						{"custom", []dto.ChannelFilter{{Kind: dto.FilterRequestPath, RequestPath: "/v1/responses"}, {Kind: dto.FilterResponsesWebSocket}}, 940002},
						{"plugin", []dto.ChannelFilter{{Kind: dto.FilterTaskPluginIdentity, TaskPluginKey: "alpha"}}, 940004},
						{"plugin", []dto.ChannelFilter{{Kind: dto.FilterTaskPluginIdentity, TaskPluginKey: "missing"}}, 0},
						{"custom", []dto.ChannelFilter{{Kind: dto.FilterRequestPath, RequestPath: "/v1/chat/completions"}}, 0},
					} {
						ch, err := GetRandomSatisfiedChannel("perf", tc.model, 0, tc.filters)
						require.NoError(t, err)
						if tc.want == 0 {
							assert.Nil(t, ch, "cache=%t model=%s", cached, tc.model)
							continue
						}
						require.NotNil(t, ch)
						assert.Equal(t, tc.want, ch.Id)
						assert.Equal(t, fmt.Sprintf("credential-%d", tc.want), ch.Key)
						assert.Equal(t, fmt.Sprintf("channel-%d", tc.want), ch.Name)
						assert.Equal(t, int64(12345), ch.UsedQuota)
						require.NotNil(t, ch.ParamOverride)
						assert.Equal(t, `{"temperature":0}`, *ch.ParamOverride)
					}
				}
				// Updating a route must immediately replace its parsed cache entry.
				ch := channels[1]
				ch.SetOtherSettings(kitdto.ChannelOtherSettings{AdvancedCustom: &kitdto.AdvancedCustomConfig{Routes: []kitdto.AdvancedCustomRoute{{IncomingPath: "/v1/chat/completions"}}}})
				CacheUpdateChannel(&ch)
				selected, err := GetRandomSatisfiedChannel("perf", "custom", 0, []dto.ChannelFilter{{Kind: dto.FilterRequestPath, RequestPath: "/v1/responses"}})
				require.NoError(t, err)
				assert.Nil(t, selected, "a removed route must not survive in the parsed cache")
				selected, err = GetRandomSatisfiedChannel("perf", "custom", 0, []dto.ChannelFilter{{Kind: dto.FilterRequestPath, RequestPath: "/v1/chat/completions"}})
				require.NoError(t, err)
				require.NotNil(t, selected)
				assert.Equal(t, ch.Id, selected.Id)
			})

			t.Run("malformed settings repair preserves unrelated and newer data", func(t *testing.T) {
				ch := Channel{Id: 940010, Key: "keep-secret", Name: "keep-name", UsedQuota: 9876, Setting: common.GetPointer("{"), OtherSettings: "{"}
				require.NoError(t, db.Create(&ch).Error)
				var partial Channel
				require.NoError(t, db.Select("id", "type", "setting", "settings").First(&partial, ch.Id).Error)
				partial.GetSetting()
				partial.GetOtherSettings()
				var got Channel
				require.NoError(t, db.First(&got, ch.Id).Error)
				assert.Equal(t, ch.Key, got.Key)
				assert.Equal(t, ch.Name, got.Name)
				assert.Equal(t, ch.UsedQuota, got.UsedQuota)
				assert.Nil(t, got.Setting)
				assert.Equal(t, "{}", got.OtherSettings)
				require.NoError(t, db.Model(&Channel{}).Where("id = ?", ch.Id).Updates(map[string]any{"setting": `{"responses_websocket_enabled":true}`, "settings": `{"api_version":"new"}`}).Error)
				// This older in-memory copy must not erase a later admin repair.
				ch.GetSetting()
				ch.GetOtherSettings()
				require.NoError(t, db.First(&got, ch.Id).Error)
				require.NotNil(t, got.Setting)
				assert.Equal(t, `{"responses_websocket_enabled":true}`, *got.Setting)
				assert.Equal(t, `{"api_version":"new"}`, got.OtherSettings)
			})

			t.Run("bulk save keeps empty values and rolls back all batches", func(t *testing.T) {
				values := make(map[string]string)
				for i := range 205 {
					values[fmt.Sprintf("perf_fixture_%03d", i)] = "before"
				}
				// Large route/price maps must round-trip without being forced into
				// the same statement as every other setting in the request.
				values["perf_fixture_large_a"] = strings.Repeat("a", 700<<10)
				values["perf_fixture_large_b"] = strings.Repeat("b", 700<<10)
				require.NoError(t, UpdateOptionsBulk(values))
				values["perf_fixture_000"] = ""
				values["perf_fixture_001"] = "0"
				values["perf_fixture_002"] = "false"
				require.NoError(t, UpdateOptionsBulk(values))
				require.NoError(t, UpdateOptionsBulk(values))
				var rows []Option
				require.NoError(t, db.Find(&rows).Error)
				actual := make(map[string]string)
				for _, row := range rows {
					actual[row.Key] = row.Value
				}
				assert.Equal(t, values, actual)
				before := maps.Clone(values)
				for key := range values {
					values[key] = "after"
				}
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("perf_reject_last_option", func(tx *gorm.DB) {
					if options, ok := tx.Statement.Dest.([]Option); ok {
						for _, option := range options {
							if option.Key == "perf_fixture_204" {
								tx.AddError(errors.New("last option rejected"))
							}
						}
					}
				}))
				assert.Error(t, UpdateOptionsBulk(values))
				require.NoError(t, db.Callback().Create().Remove("perf_reject_last_option"))
				require.NoError(t, db.Find(&rows).Error)
				actual = make(map[string]string)
				for _, row := range rows {
					actual[row.Key] = row.Value
				}
				assert.Equal(t, before, actual, "earlier batches must roll back too")
				common.OptionMapRWMutex.RLock()
				assert.Equal(t, before, common.OptionMap, "failed saves must not publish runtime settings")
				common.OptionMapRWMutex.RUnlock()
			})
		})
	}
}
