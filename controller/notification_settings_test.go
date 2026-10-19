package controller

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func notificationSettingsTestDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
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
	default:
		t.Fatalf("unsupported test dialect: %s", dialect)
	}
	db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "notification_settings_test_"}})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousMemory := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() {
		assert.NoError(t, db.Migrator().DropTable(&model.User{}, &model.Option{}))
		model.DB, common.MemoryCacheEnabled = previousDB, previousMemory
		assert.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}))
	versionQuery := "SELECT version()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database: %s %s", dialect, version)
	return db
}

func TestNotificationSettingsDatabaseMatrix(t *testing.T) {
	service.InitHttpClient()
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := notificationSettingsTestDB(t, dialect)
			oldRedis := common.RedisEnabled
			common.RedisEnabled = false
			oldEnabled := common.QuotaRemindEnabled
			common.OptionMapRWMutex.Lock()
			oldMap := common.OptionMap
			common.OptionMap = make(map[string]string)
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				common.RedisEnabled = oldRedis
				common.QuotaRemindEnabled = oldEnabled
				common.OptionMapRWMutex.Lock()
				common.OptionMap = oldMap
				common.OptionMapRWMutex.Unlock()
			})
			// Existing users have no category fields. Explicit false must survive JSON and DB round trips.
			for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
				user := model.User{Username: fmt.Sprintf("notify-%d", role), AffCode: fmt.Sprintf("notify-%d", role), Role: role, Status: common.UserStatusEnabled, Setting: `{"language":"en","sidebar_modules":"custom-sidebar","billing_preference":"wallet_only","webhook_secret":"obsolete"}`}
				require.NoError(t, db.Create(&user).Error)
				update := func(payload string, success bool) dto.UserSetting {
					t.Helper()
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Set("id", user.Id)
					c.Request = httptest.NewRequest("PUT", "/api/user/setting", strings.NewReader(payload))
					c.Request.Header.Set("Content-Type", "application/json")
					UpdateUserSetting(c)
					var response struct {
						Success bool `json:"success"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
					require.Equal(t, success, response.Success, recorder.Body.String())
					saved, err := model.GetUserSetting(user.Id, true)
					require.NoError(t, err)
					return saved
				}
				saved := update(`{"notify_type":"email","quota_warning_threshold":0,"quota_warning_enabled":false,"channel_notify_enabled":true,"upstream_model_update_notify_enabled":true}`, true)
				require.NotNil(t, saved.QuotaWarningEnabled)
				assert.False(t, *saved.QuotaWarningEnabled)
				assert.Zero(t, saved.QuotaWarningThreshold)
				assert.Equal(t, "en", saved.Language)
				assert.Equal(t, "custom-sidebar", saved.SidebarModules)
				assert.Equal(t, "wallet_only", saved.BillingPreference)
				assert.True(t, saved.RecordIpLog)
				assert.Empty(t, saved.WebhookSecret)
				if role >= common.RoleAdminUser {
					require.NotNil(t, saved.ChannelNotifyEnabled)
					assert.True(t, *saved.ChannelNotifyEnabled)
					assert.True(t, saved.UpstreamModelUpdateNotifyEnabled)
				} else {
					assert.Nil(t, saved.ChannelNotifyEnabled)
					assert.False(t, saved.UpstreamModelUpdateNotifyEnabled)
				}
				preserved := update(`{"notify_type":"email","quota_warning_threshold":0}`, true)
				assert.Equal(t, saved, preserved, "old clients must not clear new switches or unrelated preferences")
				rejected := update(`{"notify_type":"email","quota_warning_threshold":-1,"quota_warning_enabled":true}`, false)
				assert.Equal(t, saved, rejected)
				optedOut := update(`{"notify_type":"email","quota_warning_threshold":20,"quota_warning_enabled":true,"channel_notify_enabled":false}`, true)
				assert.True(t, *optedOut.QuotaWarningEnabled)
				if role >= common.RoleAdminUser {
					require.NotNil(t, optedOut.ChannelNotifyEnabled)
					assert.False(t, *optedOut.ChannelNotifyEnabled)
				}
			}
			t.Run("category recipient defaults and explicit overrides", func(t *testing.T) {
				var mu sync.Mutex
				var received []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					received = append(received, r.URL.Path)
					mu.Unlock()
					w.WriteHeader(http.StatusNoContent)
				}))
				defer server.Close()
				fetch := system_setting.GetFetchSetting()
				oldProtection := fetch.EnableSSRFProtection
				fetch.EnableSSRFProtection = false
				oldWorker := system_setting.WorkerUrl
				system_setting.WorkerUrl = ""
				oldLimit := constant.NotifyLimitCount
				constant.NotifyLimitCount = 100
				defer func() {
					fetch.EnableSSRFProtection = oldProtection
					system_setting.WorkerUrl = oldWorker
					constant.NotifyLimitCount = oldLimit
				}()
				for _, tc := range []struct {
					name         string
					role, status int
					channel      *bool
					upstream     bool
				}{
					{"root-default", 100, 1, nil, false}, {"root-off", 100, 1, common.GetPointer(false), true},
					{"admin-default", 10, 1, nil, false}, {"admin-on", 10, 1, common.GetPointer(true), false},
					{"disabled", 10, 2, common.GetPointer(true), true}, {"ordinary", 1, 1, common.GetPointer(true), true},
				} {
					settings, err := common.Marshal(dto.UserSetting{NotifyType: dto.NotifyTypeWebhook, WebhookUrl: server.URL + "/" + tc.name, ChannelNotifyEnabled: tc.channel, UpstreamModelUpdateNotifyEnabled: tc.upstream})
					require.NoError(t, err)
					require.NoError(t, db.Create(&model.User{Username: tc.name, AffCode: tc.name, Role: tc.role, Status: tc.status, Setting: string(settings)}).Error)
				}
				// The earlier form fixtures have no transport configured and send no HTTP requests.
				service.NotifyChannelWatchers("matrix-channel-"+dialect, "channel", "changed")
				mu.Lock()
				assert.ElementsMatch(t, []string{"/root-default", "/admin-on"}, received)
				received = nil
				mu.Unlock()
				service.NotifyUpstreamModelUpdateWatchers("models", "changed")
				mu.Lock()
				assert.Equal(t, []string{"/root-off"}, received)
				mu.Unlock()
			})
			for _, enabled := range []bool{false, true} {
				require.NoError(t, model.UpdateOption("QuotaRemindEnabled", fmt.Sprint(enabled)))
				assert.Equal(t, enabled, common.QuotaRemindEnabled)
				var option model.Option
				require.NoError(t, db.Where(&model.Option{Key: "QuotaRemindEnabled"}).First(&option).Error)
				assert.Equal(t, fmt.Sprint(enabled), option.Value)
			}
		})
	}
}
