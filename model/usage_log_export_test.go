package model

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// This fixture intentionally has no production indexes or migration hooks.
// Database matrix runs touch only a dedicated export fixture table.
type usageLogExportFixture struct {
	ID                int `gorm:"primaryKey"`
	UserID            int
	Type              int
	Username          string
	TokenName         string
	ModelName         string
	PromptTokens      int
	CompletionTokens  int
	Quota             int
	CreatedAt         int64
	Other             string
	ChannelID         int
	Group             string
	RequestID         string
	UpstreamRequestID string
	IsStream          bool
	UseTime           int
}

func TestUsageLogExportDatabaseMatrix(t *testing.T) {
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
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			fixture := db.Table("usage_export_test_logs")
			require.NoError(t, fixture.AutoMigrate(&usageLogExportFixture{}))
			previousDB, previousType := LOG_DB, common.LogDatabaseType()
			LOG_DB = db.Table("usage_export_test_logs AS logs")
			common.SetLogDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable("usage_export_test_logs"))
				LOG_DB = previousDB
				common.SetLogDatabaseType(previousType)
				initCol()
				require.NoError(t, sqlDB.Close())
			})
			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s %s", dialect, version)
			fixtures := []usageLogExportFixture{
				{ID: 1, UserID: 10, Type: LogTypeConsume, Username: "alice", TokenName: "token", ModelName: "model_one", PromptTokens: 100, CompletionTokens: 12, Quota: 345, CreatedAt: 100, Other: `{"cache_tokens":30,"cache_creation_tokens":99,"cache_creation_tokens_5m":4,"cache_creation_tokens_1h":6,"request_path":"/v1/responses","frt":1250.5,"admin_info":{"secret":"never exported"}}`, ChannelID: 2, Group: "default", RequestID: "req1", UpstreamRequestID: "up1", IsStream: true, UseTime: 7},
				{ID: 2, UserID: 20, Type: LogTypeConsume, Username: "bob", TokenName: "other", ModelName: "modelXone", CreatedAt: 200, Other: `{"cache_tokens":2,"cache_creation_tokens":7}`, ChannelID: 3, Group: "vip", RequestID: "req2", UpstreamRequestID: "up2"},
				{ID: 3, UserID: 10, Type: LogTypeError, Username: "alice", TokenName: "token", ModelName: "model_one", CreatedAt: 300, ChannelID: 2, Group: "default"},
				{ID: 4, UserID: 10, Type: LogTypeRefund, Username: "alice", TokenName: "refund", ModelName: "model_two", CreatedAt: 50, Other: `{`},
				{ID: 5, UserID: 10, Type: LogTypeConsume, Username: "alice", TokenName: "last", ModelName: "model_three", CreatedAt: 100, Other: `{"cache_tokens":0,"cache_creation_tokens":8,"cache_creation_tokens_5m":0,"cache_creation_tokens_1h":0}`},
			}
			require.NoError(t, db.Table("usage_export_test_logs").Create(&fixtures).Error)
			t.Run("all matching rows preserve counters and order without errors", func(t *testing.T) {
				var got []UsageLogExportRow
				require.NoError(t, StreamUsageLogs(context.Background(), UsageLogExportFilter{}, func(row UsageLogExportRow) error {
					got = append(got, row)
					return nil
				}))
				require.Len(t, got, 4)
				assert.Equal(t, []string{"other", "last", "token", "refund"}, []string{got[0].TokenName, got[1].TokenName, got[2].TokenName, got[3].TokenName})
				assert.Equal(t, float64(7), got[0].CacheWriteTokens)
				assert.Equal(t, float64(8), got[1].CacheWriteTokens)
				firstResponseTime := 1250.5
				assert.Equal(t, UsageLogExportRow{Username: "alice", TokenName: "token", ModelName: "model_one", PromptTokens: 100, CompletionTokens: 12, Quota: 345, CreatedAt: 100, CacheReadTokens: 30, CacheWriteTokens: 10, IsStream: true, RequestPath: "/v1/responses", FirstResponseTimeMS: &firstResponseTime, UseTime: 7}, got[2])
				assert.Zero(t, got[3].CacheReadTokens)
				assert.Zero(t, got[3].CacheWriteTokens)
			})
			userID := 10
			for _, tc := range []struct {
				name   string
				filter UsageLogExportFilter
				want   []string
			}{
				{"self scope and order ignore admin-only filters", UsageLogExportFilter{UserID: &userID, Username: "bob", Channel: 999}, []string{"last", "refund", "token"}},
				{"full admin filter", UsageLogExportFilter{Type: LogTypeConsume, Username: "al%", ModelName: "model_o%", TokenName: "token", Channel: 2, Group: "default", StartTimestamp: 100, EndTimestamp: 100, RequestID: "req1", UpstreamRequestID: "up1"}, []string{"token"}},
				{"model exact", UsageLogExportFilter{ModelName: "model_one"}, []string{"token"}},
				{"wildcards escape underscore", UsageLogExportFilter{ModelName: "model_o%"}, []string{"token"}},
				{"username", UsageLogExportFilter{Username: "bob"}, []string{"other"}},
				{"channel", UsageLogExportFilter{Channel: 3}, []string{"other"}},
				{"group", UsageLogExportFilter{Group: "vip"}, []string{"other"}},
				{"request id", UsageLogExportFilter{RequestID: "missing"}, nil},
				{"upstream request id", UsageLogExportFilter{UpstreamRequestID: "missing"}, nil},
				{"type", UsageLogExportFilter{Type: LogTypeRefund}, []string{"refund"}},
				{"errors excluded even when requested", UsageLogExportFilter{Type: LogTypeError}, nil},
				{"time range", UsageLogExportFilter{StartTimestamp: 101}, []string{"other"}},
				{"time range end", UsageLogExportFilter{EndTimestamp: 50}, []string{"refund"}},
				{"empty selection", UsageLogExportFilter{TokenName: "missing"}, nil},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var got []string
					require.NoError(t, StreamUsageLogs(context.Background(), tc.filter, func(row UsageLogExportRow) error {
						got = append(got, row.TokenName)
						return nil
					}))
					assert.Equal(t, tc.want, got)
				})
			}
			t.Run("invalid wildcard remains rejected", func(t *testing.T) {
				assert.Error(t, StreamUsageLogs(context.Background(), UsageLogExportFilter{ModelName: "%%"}, func(UsageLogExportRow) error {
					t.Fatal("invalid filters must not emit data")
					return nil
				}))
			})
			t.Run("legacy null type remains exportable", func(t *testing.T) {
				legacy := usageLogExportFixture{ID: 6, UserID: 10, TokenName: "legacy-null", RequestID: "legacy-null"}
				require.NoError(t, db.Table("usage_export_test_logs").Create(&legacy).Error)
				require.NoError(t, db.Table("usage_export_test_logs").Where("id = ?", legacy.ID).Update("type", nil).Error)
				var got []string
				require.NoError(t, StreamUsageLogs(context.Background(), UsageLogExportFilter{RequestID: "legacy-null"}, func(row UsageLogExportRow) error {
					got = append(got, row.TokenName)
					return nil
				}))
				assert.Equal(t, []string{"legacy-null"}, got)
				require.NoError(t, db.Table("usage_export_test_logs").Where("id = ?", legacy.ID).Delete(&usageLogExportFixture{}).Error)
			})
			t.Run("legacy null request columns remain exportable", func(t *testing.T) {
				legacy := usageLogExportFixture{ID: 7, UserID: 10, TokenName: "legacy-request", RequestID: "legacy-request"}
				require.NoError(t, db.Table("usage_export_test_logs").Create(&legacy).Error)
				require.NoError(t, db.Table("usage_export_test_logs").Where("id = ?", legacy.ID).Updates(map[string]any{"is_stream": nil, "use_time": nil, "other": nil}).Error)
				var got []UsageLogExportRow
				require.NoError(t, StreamUsageLogs(context.Background(), UsageLogExportFilter{RequestID: "legacy-request"}, func(row UsageLogExportRow) error {
					got = append(got, row)
					return nil
				}))
				require.Len(t, got, 1)
				assert.False(t, got[0].IsStream)
				assert.Empty(t, got[0].RequestPath)
				assert.Nil(t, got[0].FirstResponseTimeMS)
				assert.Zero(t, got[0].UseTime)
				require.NoError(t, db.Table("usage_export_test_logs").Where("id = ?", legacy.ID).Delete(&usageLogExportFixture{}).Error)
			})
			t.Run("request metadata validates independently of cache counters", func(t *testing.T) {
				zero, fractional := float64(0), 250.25
				for _, tc := range []struct {
					name, metadata, path string
					firstResponseTime    *float64
				}{
					{"chat fractional milliseconds", `{"request_path":"/v1/chat/completions","frt":250.25,"cache_tokens":3}`, "/v1/chat/completions", &fractional},
					{"zero is a recorded duration", `{"request_path":"/v1/messages","frt":0,"cache_tokens":3}`, "/v1/messages", &zero},
					{"invalid path keeps timing", `{"request_path":{},"frt":250.25,"cache_tokens":3}`, "", &fractional},
					{"missing timing remains unavailable", `{"request_path":"/v1/responses","cache_tokens":3}`, "/v1/responses", nil},
					{"null timing remains unavailable", `{"frt":null,"cache_tokens":3}`, "", nil},
					{"negative timing remains unavailable", `{"frt":-1,"cache_tokens":3}`, "", nil},
					{"string timing remains unavailable", `{"frt":"250","cache_tokens":3}`, "", nil},
					{"overflow timing remains unavailable", `{"frt":1e400,"cache_tokens":3}`, "", nil},
				} {
					t.Run(tc.name, func(t *testing.T) {
						require.NoError(t, db.Table("usage_export_test_logs").Where("id = ?", 1).Update("other", tc.metadata).Error)
						var got []UsageLogExportRow
						require.NoError(t, StreamUsageLogs(context.Background(), UsageLogExportFilter{RequestID: "req1"}, func(row UsageLogExportRow) error {
							got = append(got, row)
							return nil
						}))
						require.Len(t, got, 1)
						assert.Equal(t, tc.path, got[0].RequestPath)
						assert.Equal(t, tc.firstResponseTime, got[0].FirstResponseTimeMS)
						assert.Equal(t, float64(3), got[0].CacheReadTokens)
						assert.True(t, got[0].IsStream)
						assert.Equal(t, int64(7), got[0].UseTime)
						assert.Equal(t, int64(345), got[0].Quota)
					})
				}
			})
			t.Run("legacy cache fields validate independently", func(t *testing.T) {
				for _, tc := range []struct {
					name        string
					metadata    string
					read, write float64
				}{
					{"invalid write keeps read", `{"cache_tokens":30,"cache_creation_tokens":"bad"}`, 30, 0},
					{"invalid read keeps write", `{"cache_tokens":{},"cache_creation_tokens":7}`, 0, 7},
					{"negative split is zero before addition", `{"cache_tokens":-1,"cache_creation_tokens":99,"cache_creation_tokens_5m":-3,"cache_creation_tokens_1h":4}`, 0, 4},
					{"invalid split keeps other split", `{"cache_creation_tokens":99,"cache_creation_tokens_5m":"3","cache_creation_tokens_1h":4}`, 0, 4},
					{"nonpositive splits use aggregate", `{"cache_creation_tokens":8,"cache_creation_tokens_5m":-3,"cache_creation_tokens_1h":0}`, 0, 8},
					{"decimal numeric fields preserve table display", `{"cache_tokens":1.5,"cache_creation_tokens_5m":2.5,"cache_creation_tokens_1h":1}`, 1.5, 3.5},
					{"invalid numeric range is zero", `{"cache_tokens":1e400,"cache_creation_tokens":7}`, 0, 7},
					{"finite split sum overflow is zero", `{"cache_tokens":3,"cache_creation_tokens_5m":1e308,"cache_creation_tokens_1h":1e308}`, 3, 0},
				} {
					t.Run(tc.name, func(t *testing.T) {
						require.NoError(t, db.Table("usage_export_test_logs").Where("id = ?", 1).Update("other", tc.metadata).Error)
						var got []UsageLogExportRow
						require.NoError(t, StreamUsageLogs(context.Background(), UsageLogExportFilter{RequestID: "req1"}, func(row UsageLogExportRow) error {
							got = append(got, row)
							return nil
						}))
						require.Len(t, got, 1)
						assert.Equal(t, tc.read, got[0].CacheReadTokens)
						assert.Equal(t, tc.write, got[0].CacheWriteTokens)
						assert.Equal(t, int64(345), got[0].Quota)
					})
				}
			})
			t.Run("callback failure and cancellation close cursor", func(t *testing.T) {
				writeError := errors.New("client disconnected")
				assert.ErrorIs(t, StreamUsageLogs(context.Background(), UsageLogExportFilter{}, func(UsageLogExportRow) error { return writeError }), writeError)
				ctx, cancel := context.WithCancel(context.Background())
				assert.ErrorIs(t, StreamUsageLogs(ctx, UsageLogExportFilter{}, func(UsageLogExportRow) error {
					cancel()
					return nil
				}), context.Canceled)
				// A single-connection pool can execute again only if both cursors closed.
				assert.NoError(t, StreamUsageLogs(context.Background(), UsageLogExportFilter{Type: LogTypeError}, func(UsageLogExportRow) error { return nil }))
			})
		})
	}
}
