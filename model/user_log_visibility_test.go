package model

import (
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

func TestUserLogVisibilityDatabaseMatrix(t *testing.T) {
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
			database, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := database.DB()
			require.NoError(t, err)
			fixture := database.Table("user_visibility_test_logs")
			require.NoError(t, fixture.AutoMigrate(&Log{}))
			previousLogDB, previousType := LOG_DB, common.LogDatabaseType()
			LOG_DB = database.Table("user_visibility_test_logs AS logs").Session(&gorm.Session{})
			common.SetLogDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				assert.NoError(t, database.Migrator().DropTable("user_visibility_test_logs"))
				LOG_DB = previousLogDB
				common.SetLogDatabaseType(previousType)
				initCol()
				assert.NoError(t, sqlDB.Close())
			})
			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, database.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s %s", dialect, version)

			fixtures := []Log{
				{Id: 1, UserId: 10, Type: LogTypeConsume, Username: "alice", ModelName: "chat-model", TokenName: "work", CreatedAt: 100, Quota: 12, RequestId: "success", Other: `{"admin_info":{"secret":true},"root_info":{"secret":true}}`},
				{Id: 2, UserId: 10, Type: LogTypeError, Username: "alice", ModelName: "chat-model", TokenName: "work", CreatedAt: 200, RequestId: "upstream-error"},
				{Id: 3, UserId: 10, Type: LogTypeConsume, Username: "alice", ModelName: "chat-model", TokenName: "work", CreatedAt: 300, Quota: 7, RequestId: "charged-partial-stream", IsStream: true, Other: `{"stream_status":{"status":"error","response_status":"failed"}}`},
				{Id: 4, UserId: 10, Type: LogTypeRefund, Username: "alice", TokenName: "refund", CreatedAt: 400, RequestId: "refund"},
				{Id: 5, UserId: 20, Type: LogTypeConsume, Username: "bob", ModelName: "chat-model", CreatedAt: 500, RequestId: "other-user"},
				{Id: 6, UserId: 10, Type: LogTypeTopup, Username: "alice", CreatedAt: 600, RequestId: "topup"},
			}
			require.NoError(t, fixture.Create(&fixtures).Error)

			t.Run("user pagination excludes errors but retains charges and account records", func(t *testing.T) {
				logs, total, err := GetUserLogs(10, LogTypeUnknown, 0, 0, "", "", 0, 2, "", "", "")
				require.NoError(t, err)
				assert.Equal(t, int64(4), total)
				require.Len(t, logs, 2)
				assert.Equal(t, []string{"topup", "refund"}, []string{logs[0].RequestId, logs[1].RequestId})
				logs, total, err = GetUserLogs(10, LogTypeUnknown, 0, 0, "", "", 2, 2, "", "", "")
				require.NoError(t, err)
				assert.Equal(t, int64(4), total)
				require.Len(t, logs, 2)
				assert.Equal(t, []string{"charged-partial-stream", "success"}, []string{logs[0].RequestId, logs[1].RequestId})
				assert.Equal(t, 19, logs[0].Quota+logs[1].Quota)
				assert.Contains(t, logs[0].Other, `"response_status":"failed"`)
				assert.NotContains(t, logs[1].Other, "admin_info")
				assert.NotContains(t, logs[1].Other, "root_info")
			})
			t.Run("user error type and error request filters return empty page", func(t *testing.T) {
				for _, filter := range []struct {
					logType int
					request string
				}{{LogTypeError, ""}, {LogTypeUnknown, "upstream-error"}} {
					logs, total, err := GetUserLogs(10, filter.logType, 0, 0, "", "", 0, 10, "", filter.request, "")
					require.NoError(t, err)
					assert.Zero(t, total)
					assert.Empty(t, logs)
				}
			})
			t.Run("consume filters keep both complete and billed partial streams", func(t *testing.T) {
				logs, total, err := GetUserLogs(10, LogTypeConsume, 100, 300, "chat-model", "work", 0, 10, "", "", "")
				require.NoError(t, err)
				assert.Equal(t, int64(2), total)
				require.Len(t, logs, 2)
				assert.Equal(t, 19, logs[0].Quota+logs[1].Quota)
			})
			t.Run("administrator queries retain error rows and all users", func(t *testing.T) {
				logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "")
				require.NoError(t, err)
				assert.Equal(t, int64(6), total)
				assert.Len(t, logs, 6)
				logs, total, err = GetAllLogs(LogTypeError, 0, 0, "", "", "", 0, 10, 0, "", "", "")
				require.NoError(t, err)
				assert.Equal(t, int64(1), total)
				require.Len(t, logs, 1)
				assert.Equal(t, "upstream-error", logs[0].RequestId)
			})
			t.Run("legacy rows without a type remain visible to their owner", func(t *testing.T) {
				legacy := Log{Id: 7, UserId: 10, RequestId: "legacy-null-type"}
				require.NoError(t, fixture.Create(&legacy).Error)
				require.NoError(t, fixture.Where("id = ?", legacy.Id).Update("type", nil).Error)
				logs, total, err := GetUserLogs(10, LogTypeUnknown, 0, 0, "", "", 0, 10, "", legacy.RequestId, "")
				require.NoError(t, err)
				assert.Equal(t, int64(1), total)
				require.Len(t, logs, 1)
				assert.Equal(t, legacy.RequestId, logs[0].RequestId)
			})
		})
	}
}
