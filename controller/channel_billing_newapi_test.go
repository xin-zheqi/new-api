package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func channelBalanceTestDB(t *testing.T, dialect string) *gorm.DB {
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
	db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "newapi_balance_test_"}})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousMemory := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() {
		assert.NoError(t, db.Migrator().DropTable(&model.Channel{}))
		model.DB, common.MemoryCacheEnabled = previousDB, previousMemory
		assert.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	versionQuery := "SELECT version()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database: %s %s", dialect, version)
	return db
}

func TestNewAPIBalanceDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := channelBalanceTestDB(t, dialect)
			for _, tc := range []struct {
				name       string
				usage      string
				status     string
				usageHTTP  int
				statusHTTP int
				balance    float64
				err        string
			}{
				{name: "limited key capped by owner", usage: `{"code":true,"data":{"total_available":4000000,"user_quota":{"total_available":1500000}}}`, balance: 3},
				{name: "limited key capped by token", usage: `{"code":true,"data":{"total_available":1500000,"user_quota":{"total_available":4000000}}}`, balance: 3},
				{name: "legacy upstream without owner quota", usage: `{"code":true,"data":{"total_available":1250000}}`, balance: 2.5},
				{name: "unlimited key ignores token quota", usage: `{"code":true,"data":{"total_available":-1,"unlimited_quota":true,"user_quota":{"total_available":2000000}}}`, balance: 4},
				{name: "upstream conversion rate", usage: `{"code":true,"data":{"total_available":1500}}`, status: `{"success":true,"data":{"quota_per_unit":1000}}`, balance: 1.5},
				{name: "explicit zero is saved", usage: `{"code":true,"data":{"total_available":0}}`, balance: 0},
				{name: "negative owner balance is preserved", usage: `{"code":true,"data":{"total_available":1000,"user_quota":{"total_available":-500000}}}`, balance: -1},
				{name: "unlimited key requires owner permission", usage: `{"code":true,"data":{"total_available":0,"unlimited_quota":true}}`, err: "Allow querying user quota"},
				{name: "missing usage data", usage: `{"code":true}`, err: "missing data"},
				{name: "null usage data", usage: `{"code":true,"data":null}`, err: "missing data"},
				{name: "missing token balance", usage: `{"code":true,"data":{"user_quota":{"total_available":100}}}`, err: "missing total_available"},
				{name: "null token balance", usage: `{"code":true,"data":{"total_available":null}}`, err: "missing total_available"},
				{name: "missing owner balance", usage: `{"code":true,"data":{"total_available":100,"user_quota":{}}}`, err: "missing total_available"},
				{name: "invalid usage JSON", usage: `<html>error</html>`, err: "invalid token usage"},
				{name: "usage rejection redacts key", usage: `{"code":false,"message":"invalid sk-balance-fixture"}`, err: "[REDACTED]"},
				{name: "usage HTTP rejection", usageHTTP: 403, err: "HTTP 403"},
				{name: "status HTTP rejection", statusHTTP: 502, err: "HTTP 502"},
				{name: "status rejection", status: `{"success":false,"data":{"quota_per_unit":500000}}`, err: "valid quota_per_unit"},
				{name: "invalid status JSON", status: `{`, err: "valid quota_per_unit"},
				{name: "missing quota conversion", status: `{"success":true,"data":{}}`, err: "valid quota_per_unit"},
				{name: "zero quota conversion", status: `{"success":true,"data":{"quota_per_unit":0}}`, err: "valid quota_per_unit"},
				{name: "negative quota conversion", status: `{"success":true,"data":{"quota_per_unit":-1}}`, err: "valid quota_per_unit"},
				{name: "balance overflow", usage: `{"code":true,"data":{"total_available":1e308}}`, status: `{"success":true,"data":{"quota_per_unit":0.01}}`, err: "invalid balance"},
				{name: "oversized usage body", usage: strings.Repeat(" ", 300<<10), err: "response is too large"},
				{name: "oversized status body", status: strings.Repeat(" ", 300<<10), err: "response is too large"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if tc.usage == "" {
						tc.usage = `{"code":true,"data":{"total_available":1000000}}`
					}
					if tc.status == "" {
						tc.status = `{"success":true,"data":{"quota_per_unit":500000}}`
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, http.MethodGet, r.Method)
						switch r.URL.Path {
						case "/gateway/api/usage/token/":
							assert.Equal(t, "Bearer sk-balance-fixture", r.Header.Get("Authorization"))
							if tc.usageHTTP != 0 {
								w.WriteHeader(tc.usageHTTP)
							}
							fmt.Fprint(w, tc.usage)
						case "/gateway/api/status":
							assert.Empty(t, r.Header.Get("Authorization"), "public status must not receive the key")
							if tc.statusHTTP != 0 {
								w.WriteHeader(tc.statusHTTP)
							}
							fmt.Fprint(w, tc.status)
						default:
							t.Errorf("unexpected upstream path: %s", r.URL.Path)
							w.WriteHeader(http.StatusNotFound)
						}
					}))
					defer upstream.Close()
					channel := model.Channel{
						Type: constant.ChannelTypeNewAPI, Name: "keep name", Key: " sk-balance-fixture ",
						BaseURL: common.GetPointer(upstream.URL + "/gateway/"), Status: common.ChannelStatusEnabled,
						Balance: 19, BalanceUpdatedTime: 123, UsedQuota: 77,
					}
					require.NoError(t, db.Create(&channel).Error)
					defer db.Delete(&channel)
					result, err := updateChannelBalance(&channel)
					var saved model.Channel
					require.NoError(t, db.First(&saved, channel.Id).Error)
					assert.Equal(t, channel.Key, saved.Key)
					assert.Equal(t, "keep name", saved.Name)
					assert.Equal(t, int64(77), saved.UsedQuota)
					assert.Equal(t, common.ChannelStatusEnabled, saved.Status)
					if tc.err != "" {
						require.ErrorContains(t, err, tc.err)
						assert.NotContains(t, err.Error(), "sk-balance-fixture")
						assert.Equal(t, float64(19), saved.Balance)
						assert.Equal(t, int64(123), saved.BalanceUpdatedTime)
						assert.Equal(t, saved.Balance, channel.Balance)
						assert.Equal(t, saved.BalanceUpdatedTime, channel.BalanceUpdatedTime)
						return
					}
					require.NoError(t, err)
					assert.Equal(t, tc.balance, result.Balance)
					assert.Empty(t, result.RawResponse)
					assert.Equal(t, tc.balance, saved.Balance)
					assert.Greater(t, saved.BalanceUpdatedTime, int64(123))
					assert.Equal(t, saved.Balance, channel.Balance)
					assert.Equal(t, saved.BalanceUpdatedTime, channel.BalanceUpdatedTime)
				})
			}

			t.Run("API and bulk updates preserve failed disabled and multi-key channels", func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/status" {
						fmt.Fprint(w, `{"success":true,"data":{"quota_per_unit":500000}}`)
						return
					}
					switch r.Header.Get("Authorization") {
					case "Bearer good":
						fmt.Fprint(w, `{"code":true,"data":{"total_available":1000000}}`)
					case "Bearer malformed":
						fmt.Fprint(w, `{"code":true,"data":{}}`)
					default:
						t.Errorf("disabled/multi-key channel was queried")
					}
				}))
				defer upstream.Close()
				channels := []model.Channel{
					{Key: "good", Status: common.ChannelStatusEnabled},
					{Key: "malformed", Status: common.ChannelStatusEnabled},
					{Key: "disabled", Status: common.ChannelStatusManuallyDisabled},
					{Key: "multi", Status: common.ChannelStatusEnabled, ChannelInfo: model.ChannelInfo{IsMultiKey: true}},
				}
				for i := range channels {
					channels[i].Type, channels[i].BaseURL = constant.ChannelTypeNewAPI, &upstream.URL
					channels[i].Balance, channels[i].BalanceUpdatedTime = 19, 123
					require.NoError(t, db.Create(&channels[i]).Error)
				}
				router := gin.New()
				router.GET("/api/channel/update_balance/:id", UpdateChannelBalance)
				router.GET("/api/channel/update_balance", UpdateAllChannelsBalance)
				for i, channel := range channels {
					if i == 2 {
						continue // manual querying of disabled channels remains allowed
					}
					w := httptest.NewRecorder()
					router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/update_balance/%d", channel.Id), nil))
					assert.Equal(t, http.StatusOK, w.Code)
					var body map[string]any
					require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
					assert.Equal(t, i == 0, body["success"])
					if i == 0 {
						assert.Equal(t, float64(2), body["balance"])
					} else {
						assert.NotEmpty(t, body["message"])
						assert.NotContains(t, body, "balance")
					}
				}
				previousInterval := common.RequestInterval
				common.RequestInterval = 0
				defer func() { common.RequestInterval = previousInterval }()
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/channel/update_balance", nil))
				assert.JSONEq(t, `{"success":true,"message":""}`, w.Body.String())
				for i, channel := range channels {
					var saved model.Channel
					require.NoError(t, db.First(&saved, channel.Id).Error)
					assert.Equal(t, channel.Status, saved.Status)
					if i == 0 {
						assert.Equal(t, float64(2), saved.Balance)
					} else {
						assert.Equal(t, float64(19), saved.Balance)
						assert.Equal(t, int64(123), saved.BalanceUpdatedTime)
					}
				}
			})
		})
	}
}

func TestNewAPIBalanceProxyAndDatabaseFailure(t *testing.T) {
	db := channelBalanceTestDB(t, "sqlite")
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "newapi.invalid", r.URL.Host)
		if r.URL.Path == "/api/usage/token/" {
			assert.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
			fmt.Fprint(w, `{"code":true,"data":{"total_available":1000000}}`)
			return
		}
		assert.Equal(t, "/api/status", r.URL.Path)
		assert.Empty(t, r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"success":true,"data":{"quota_per_unit":500000}}`)
	}))
	defer proxy.Close()
	defer service.InvalidateProxyClient(proxy.URL)
	settings, err := common.Marshal(map[string]string{"proxy": proxy.URL})
	require.NoError(t, err)
	channel := model.Channel{Type: constant.ChannelTypeNewAPI, Key: "fixture", BaseURL: common.GetPointer("http://newapi.invalid"), Setting: common.GetPointer(string(settings)), Balance: 19, BalanceUpdatedTime: 123}
	require.NoError(t, db.Create(&channel).Error)
	writeError := errors.New("balance write rejected")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("reject_balance_update", func(tx *gorm.DB) { tx.AddError(writeError) }))
	_, err = updateChannelBalance(&channel)
	require.ErrorIs(t, err, writeError)
	assert.Equal(t, float64(19), channel.Balance)
	assert.Equal(t, int64(123), channel.BalanceUpdatedTime)
	var saved model.Channel
	require.NoError(t, db.First(&saved, channel.Id).Error)
	assert.Equal(t, channel.Balance, saved.Balance)
	assert.Equal(t, channel.BalanceUpdatedTime, saved.BalanceUpdatedTime)
}

func TestNewAPIBalanceRejectsInvalidConfiguration(t *testing.T) {
	for _, baseURL := range []string{"", "newapi.invalid", "ftp://newapi.invalid", "http://user:password@newapi.invalid", "http://newapi.invalid?key=secret", "http://newapi.invalid#fragment"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := updateChannelNewAPIBalance(&model.Channel{BaseURL: &baseURL, Key: "fixture"})
			require.ErrorContains(t, err, "valid HTTP(S) base URL")
		})
	}
	_, err := updateChannelNewAPIBalance(&model.Channel{BaseURL: common.GetPointer("https://newapi.invalid"), Key: " "})
	require.ErrorContains(t, err, "missing an API key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = getChannelBalanceResponse(ctx, &model.Channel{}, "http://newapi.invalid/api/status", nil, "New API")
	require.ErrorContains(t, err, "context canceled")
}
