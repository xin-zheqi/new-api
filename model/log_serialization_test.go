package model

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRecordConsumeLogSerializationDatabaseMatrix(t *testing.T) {
	previousConsumeLogging, previousExport := common.LogConsumeEnabled, common.DataExportEnabled
	common.LogConsumeEnabled, common.DataExportEnabled = true, false
	t.Cleanup(func() {
		common.LogConsumeEnabled, common.DataExportEnabled = previousConsumeLogging, previousExport
	})
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
			fixture := db.Table("consume_serialization_test_logs")
			require.NoError(t, fixture.AutoMigrate(&Log{}))
			previousLogDB, previousType := LOG_DB, common.LogDatabaseType()
			LOG_DB = db.Table("consume_serialization_test_logs").Session(&gorm.Session{})
			common.SetLogDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				assert.NoError(t, db.Migrator().DropTable("consume_serialization_test_logs"))
				LOG_DB = previousLogDB
				common.SetLogDatabaseType(previousType)
				initCol()
				assert.NoError(t, sqlDB.Close())
			})
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			testRecordConsumeLogSerialization(t)
		})
	}
}

func testRecordConsumeLogSerialization(t *testing.T) {
	t.Helper()
	tests := []struct {
		name    string
		other   map[string]interface{}
		request bool
	}{
		{name: "nil metadata", request: true},
		{name: "empty metadata", other: map[string]interface{}{}, request: true},
		{name: "existing admin metadata", request: true, other: map[string]interface{}{
			"cache_tokens": 12,
			"admin_info":   map[string]interface{}{"request_ip": "old-ip", "user_agent": "old-agent", "use_channel": []string{"1", "2"}},
			"root_info":    map[string]interface{}{"upstream_status": 200},
			"opaque":       common.RawMessage(`{"encrypted_content":"opaque+/=\\\"<>&","large_number":9007199254740993,"enabled":false,"zero":0}`),
		}},
		{name: "no HTTP request", request: false, other: map[string]interface{}{"source": "background"}},
		{name: "nil metadata without HTTP request", request: false},
		{name: "invalid admin metadata repaired by request context", request: true, other: map[string]interface{}{"admin_info": make(chan int), "source": "fixture"}},
		{name: "unencodable metadata preserves failure behavior", request: true, other: map[string]interface{}{"invalid": math.NaN()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, LOG_DB.Where("user_id = ?", 42).Delete(&Log{}).Error)
			c := newLogRequestTestContext()
			if !tt.request {
				c.Request = nil
			}
			params := RecordConsumeLogParams{
				ChannelId: 1, PromptTokens: 101, CompletionTokens: 17, ModelName: "gpt-test",
				TokenName: "test-token", Quota: 118, Content: "消费诊断 <>&", TokenId: 7,
				UseTimeSeconds: 3, IsStream: true, Group: "default", Other: tt.other,
			}
			// Snapshot the old diagnostic before request metadata is merged. The
			// original implementation intentionally did not print the merged IP/UA.
			wantDiagnostic, _ := common.Marshal(params)
			capture, err := os.CreateTemp(t.TempDir(), "consume-log-*.txt")
			require.NoError(t, err)
			common.LogWriterMu.Lock()
			originalWriter, originalErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
			gin.DefaultWriter, gin.DefaultErrorWriter = capture, capture
			common.LogWriterMu.Unlock()
			t.Cleanup(func() {
				common.LogWriterMu.Lock()
				gin.DefaultWriter, gin.DefaultErrorWriter = originalWriter, originalErrorWriter
				common.LogWriterMu.Unlock()
				require.NoError(t, capture.Close())
			})

			RecordConsumeLog(c, 42, params)

			output, err := os.ReadFile(capture.Name())
			require.NoError(t, err)
			_, diagnostic, found := strings.Cut(string(output), "record consume log: userId=42, params=")
			require.True(t, found)
			diagnostic, _, _ = strings.Cut(diagnostic, "\n")
			diagnostic = strings.TrimSuffix(diagnostic, " ")
			if len(wantDiagnostic) == 0 {
				assert.Empty(t, diagnostic)
			} else {
				assert.JSONEq(t, string(wantDiagnostic), diagnostic)
			}

			var stored Log
			require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", 42, LogTypeConsume).First(&stored).Error)
			// This is the previous persistence path, after its normal in-place
			// merge; no conversion through float64 may lose large JSON numbers.
			wantOther := common.MapToJsonStr(addLogRequestAdminInfo(c, tt.other))
			if wantOther == "" {
				assert.Empty(t, stored.Other)
			} else {
				assert.JSONEq(t, wantOther, stored.Other)
				if tt.name == "existing admin metadata" {
					assert.Contains(t, stored.Other, "9007199254740993")
					assert.NotContains(t, diagnostic, "203.0.113.9")
					assert.NotContains(t, diagnostic, "new-api-test")
					assert.Contains(t, stored.Other, "203.0.113.9")
					assert.Contains(t, stored.Other, "new-api-test")
				}
			}
			assert.Equal(t, params.Quota, stored.Quota)
			assert.Equal(t, params.PromptTokens, stored.PromptTokens)
			assert.Equal(t, params.CompletionTokens, stored.CompletionTokens)
			assert.Equal(t, params.Content, stored.Content)
			assert.Equal(t, params.ChannelId, stored.ChannelId)
			assert.Equal(t, params.TokenId, stored.TokenId)
			assert.Equal(t, params.ModelName, stored.ModelName)
			assert.Equal(t, params.TokenName, stored.TokenName)
			assert.Equal(t, params.UseTimeSeconds, stored.UseTime)
			assert.Equal(t, params.IsStream, stored.IsStream)
			assert.Equal(t, params.Group, stored.Group)
			assert.Equal(t, "req-test", stored.RequestId)
			assert.Equal(t, "log-user", stored.Username)
		})
	}
}
