package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestContentAuditDatabaseMatrix(t *testing.T) {
	cases := []struct {
		name      string
		kind      common.DatabaseType
		dialector gorm.Dialector
	}{
		{"sqlite", common.DatabaseTypeSQLite, sqlite.Open(filepath.Join(t.TempDir(), "audit.db") + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate")},
	}
	if dsn := os.Getenv("TEST_MYSQL_DSN"); dsn != "" {
		cases = append(cases, struct {
			name      string
			kind      common.DatabaseType
			dialector gorm.Dialector
		}{"mysql", common.DatabaseTypeMySQL, mysql.Open(dsn)})
	}
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		cases = append(cases, struct {
			name      string
			kind      common.DatabaseType
			dialector gorm.Dialector
		}{"postgres", common.DatabaseTypePostgreSQL, postgres.Open(dsn)})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CRYPTO_SECRET", "content-audit-model-test-secret-0123456789")
			t.Setenv("SESSION_SECRET", "")
			previousAuditSettings := common.GetContentAuditSettings()
			require.NoError(t, common.ApplyContentAuditSettings(`{"enabled":true,"retention_days":7}`))
			t.Cleanup(func() {
				value, marshalErr := common.Marshal(previousAuditSettings)
				if marshalErr == nil {
					_ = common.ApplyContentAuditSettings(string(value))
				}
			})
			oldContentAuditDir := common.ContentAuditDir
			common.ContentAuditDir = filepath.Join(t.TempDir(), "content-audit")
			t.Cleanup(func() { common.ContentAuditDir = oldContentAuditDir })
			db, err := gorm.Open(tc.dialector, &gorm.Config{})
			require.NoError(t, err)
			versionQuery := "SELECT version()"
			if tc.kind == common.DatabaseTypeSQLite {
				versionQuery = "SELECT sqlite_version()"
			}
			var databaseVersion string
			require.NoError(t, db.Raw(versionQuery).Scan(&databaseVersion).Error)
			t.Logf("Database: %s", databaseVersion)
			connection, err := db.DB()
			require.NoError(t, err)
			oldDB, oldLog := DB, LOG_DB
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(tc.kind, common.DatabaseTypeSQLite)
			initCol()
			// A separate log database proves the export never accidentally reads main DB.
			logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
			require.NoError(t, err)
			LOG_DB = logDB
			logConnection, err := logDB.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				DB = oldDB
				LOG_DB = oldLog
				common.SetDatabaseTypes(oldMainType, oldLogType)
				initCol()
				_ = connection.Close()
				_ = logConnection.Close()
			})
			// Existing tables are unchanged from the pre-feature schema. Seed legacy
			// data first, then add the new tables and repeat migration for idempotency.
			require.NoError(t, db.AutoMigrate(&Token{}))
			require.NoError(t, logDB.AutoMigrate(&Log{}))
			token := Token{UserId: 31001, Name: "Agent_100%", Key: "never-export-this-secret"}
			require.NoError(t, db.Create(&token).Error)
			t.Cleanup(func() { _ = db.Unscoped().Where("user_id IN ?", []int{31001, 31002}).Delete(&Token{}).Error })
			for range 2 {
				require.NoError(t, db.AutoMigrate(&ContentAuditPolicy{}, &ContentAuditRecord{}, &ContentAuditSequence{}))
			}
			t.Cleanup(func() {
				_ = db.Where("user_id IN ?", []int{31001, 31002}).Delete(&ContentAuditPolicy{}).Error
				_ = db.Where("user_id IN ?", []int{31001, 31002}).Delete(&ContentAuditRecord{}).Error
			})
			require.True(t, db.Migrator().HasIndex(&ContentAuditRecord{}, "idx_content_audit_owner_time"))
			require.True(t, db.Migrator().HasIndex(&ContentAuditRecord{}, "idx_content_audit_records_id"))
			var legacy Token
			require.NoError(t, db.First(&legacy, token.Id).Error)
			assert.Equal(t, token.Key, legacy.Key)
			policy := contentaudit.Policy{Enabled: true, Rules: []contentaudit.Rule{{ID: "privacy", Name: "Private rule", Enabled: true, Pattern: "secret", Mode: "all", Action: "replace", Replacement: "safe"}}}
			ctx := context.Background()
			require.NoError(t, SaveContentAuditPolicy(ctx, 31001, policy))
			policy.Rules[0].Replacement = "redacted"
			require.NoError(t, SaveContentAuditPolicy(ctx, 31001, policy))
			stored, err := GetContentAuditPolicy(ctx, 31001)
			require.NoError(t, err)
			assert.NotContains(t, stored.EncryptedRules, "Private rule")
			decoded, err := DecodeContentAuditPolicy(stored)
			require.NoError(t, err)
			assert.Equal(t, policy, decoded)
			now := time.Now().Unix()
			details := contentaudit.Result{Action: "replace", Count: 1, Matches: []contentaudit.Match{{RuleID: "privacy", Original: "private original", Replacement: "redacted", Applied: true}}}
			require.NoError(t, InsertContentAuditRecord(ctx, ContentAuditRecord{UserID: 31001, CreatedAt: now, Action: "replace", MatchCount: 1, TokenName: token.Name}, details))
			rows, total, err := ListContentAuditRecords(ctx, 31001, ContentAuditFilter{Page: 1, PageSize: 20, Keyword: "_100%", Action: "replace"})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, rows, 1)
			assert.Empty(t, rows[0].EncryptedDetails)
			result, err := GetContentAuditDetails(ctx, 31001, rows[0].ID)
			require.NoError(t, err)
			assert.Equal(t, details, result)
			detailPath, err := contentAuditDetailsPath(31001, rows[0].ID)
			require.NoError(t, err)
			_, err = os.Stat(detailPath)
			require.NoError(t, err)
			_, err = GetContentAuditDetails(ctx, 31002, rows[0].ID)
			require.Error(t, err)
			require.NoError(t, DeleteContentAuditRecords(ctx, 31002, rows[0].ID))
			_, err = GetContentAuditDetails(ctx, 31001, rows[0].ID)
			require.NoError(t, err)
			// Concurrent inserts must allocate distinct slots and preserve both records.
			var wg sync.WaitGroup
			errors := make(chan error, 2)
			for _, id := range []string{"request-a", "request-b"} {
				wg.Add(1)
				go func(id string) {
					defer wg.Done()
					errors <- InsertContentAuditRecord(ctx, ContentAuditRecord{UserID: 31001, CreatedAt: now, RequestID: id, Action: "warn", MatchCount: 1}, details)
				}(id)
			}
			wg.Wait()
			close(errors)
			for err := range errors {
				require.NoError(t, err)
			}
			_, total, err = ListContentAuditRecords(ctx, 31001, ContentAuditFilter{Page: 1, PageSize: 20})
			require.NoError(t, err)
			assert.EqualValues(t, 3, total)
			// Escaping control characters plus encryption/base64 can exceed a
			// MySQL TEXT column even with the documented 48 x 160-byte limits.
			controlText := strings.Repeat("\x01", 160)
			largeRules, err := contentaudit.Compile(contentaudit.Policy{Rules: []contentaudit.Rule{{ID: "controls", Name: "Control text", Enabled: true, Pattern: "\x01+", Mode: "all", Action: "replace", Replacement: controlText}}})
			require.NoError(t, err)
			largeBody, err := common.Marshal(map[string]string{"input": strings.Repeat(controlText+" ", contentaudit.MaxDetails)})
			require.NoError(t, err)
			_, largeDetails, err := contentaudit.InspectJSON(largeBody, largeRules)
			require.NoError(t, err)
			require.NoError(t, InsertContentAuditRecord(ctx, ContentAuditRecord{UserID: 31001, CreatedAt: now, Action: "replace", RequestID: "escaped-details", MatchCount: largeDetails.Count}, largeDetails))
			largeRows, _, err := ListContentAuditRecords(ctx, 31001, ContentAuditFilter{Page: 1, PageSize: 20, Keyword: "escaped-details"})
			require.NoError(t, err)
			require.Len(t, largeRows, 1)
			readDetails, err := GetContentAuditDetails(ctx, 31001, largeRows[0].ID)
			require.NoError(t, err)
			assert.Equal(t, largeDetails.Action, readDetails.Action)
			assert.Equal(t, largeDetails.Count, readDetails.Count)
			assert.Equal(t, largeDetails.Truncated, readDetails.Truncated)
			assert.Equal(t, largeDetails.Matches, readDetails.Matches)
			require.NoError(t, DeleteContentAuditRecords(ctx, 31001, ""))
			_, err = os.Stat(detailPath)
			assert.ErrorIs(t, err, os.ErrNotExist)
			var remaining int64
			require.NoError(t, db.Unscoped().Model(&ContentAuditRecord{}).Where("user_id = ?", 31001).Count(&remaining).Error)
			assert.Zero(t, remaining)
			// Actually reuse an occupied expired slot: GORM UpdateAll alone
			// skips created_at and would make this new record expire at once.
			require.NoError(t, db.Model(&ContentAuditSequence{}).Where("id = ?", 1).Update("next_slot", 0).Error)
			require.NoError(t, InsertContentAuditRecord(ctx, ContentAuditRecord{UserID: 31002, CreatedAt: now - 10*86400, Action: "warn"}, details))
			require.NoError(t, db.Model(&ContentAuditSequence{}).Where("id = ?", 1).Update("next_slot", 0).Error)
			require.NoError(t, InsertContentAuditRecord(ctx, ContentAuditRecord{UserID: 31001, CreatedAt: now, Action: "warn", RequestID: "reused-slot"}, details))
			reused, _, err := ListContentAuditRecords(ctx, 31001, ContentAuditFilter{Page: 1, PageSize: 20, Keyword: "reused-slot"})
			require.NoError(t, err)
			require.Len(t, reused, 1)
			assert.Equal(t, now, reused[0].CreatedAt)
			require.NoError(t, DeleteContentAuditRecords(ctx, 31001, ""))
			// Slots are monotonic and are no longer wrapped by a site-wide cap.
			require.NoError(t, db.Model(&ContentAuditSequence{}).Where("id = ?", 1).Update("next_slot", 10000).Error)
			require.NoError(t, InsertContentAuditRecord(ctx, ContentAuditRecord{UserID: 31001, CreatedAt: now, Action: "warn", RequestID: "uncapped"}, details))
			require.NoError(t, InsertContentAuditRecord(ctx, ContentAuditRecord{UserID: 31002, CreatedAt: now - 10*86400, Action: "warn"}, details))
			require.NoError(t, PurgeExpiredContentAuditRecords(ctx, time.Now()))
			uncapped, _, err := ListContentAuditRecords(ctx, 31001, ContentAuditFilter{Page: 1, PageSize: 20, Keyword: "uncapped"})
			require.NoError(t, err)
			require.Len(t, uncapped, 1)
			_, total, err = ListContentAuditRecords(ctx, 31002, ContentAuditFilter{Page: 1, PageSize: 20})
			require.NoError(t, err)
			assert.Zero(t, total)
			logs := []Log{
				{UserId: 31001, TokenId: token.Id, Type: LogTypeConsume, CreatedAt: 100, ModelName: "a", PromptTokens: 10, CompletionTokens: 2, Quota: 600},
				{UserId: 31001, TokenId: token.Id, Type: LogTypeConsume, CreatedAt: 200, ModelName: "a", PromptTokens: 20, CompletionTokens: 3, Quota: 700},
				{UserId: 31001, TokenId: token.Id, Type: LogTypeConsume, CreatedAt: 200, ModelName: "b", PromptTokens: 5, CompletionTokens: 1, Quota: 200},
				{UserId: 31001, TokenId: token.Id, Type: LogTypeConsume, CreatedAt: 99, ModelName: "out-of-range", Quota: 999},
				{UserId: 31002, TokenId: token.Id, Type: LogTypeConsume, CreatedAt: 150, ModelName: "other-user", Quota: 999},
				{UserId: 31001, TokenId: token.Id, Type: LogTypeError, CreatedAt: 150, ModelName: "error", Quota: 999},
			}
			require.NoError(t, logDB.Create(&logs).Error)
			exported, err := GetTokenUsageExport(ctx, 31001, "_100%", 100, 200)
			require.NoError(t, err)
			require.Len(t, exported, 2)
			assert.Equal(t, TokenUsageExportRow{TokenID: token.Id, TokenName: token.Name, ModelName: "a", Requests: 2, PromptTokens: 30, CompletionTokens: 5, Quota: 1300, FirstRequest: 100, LastRequest: 200}, exported[0])
			assert.Equal(t, "b", exported[1].ModelName)
			encoded, err := common.Marshal(exported)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), token.Key)
			// Also exercise log aggregation in the same engine dialect as main DB.
			require.NoError(t, db.AutoMigrate(&Log{}))
			for i := range logs {
				logs[i].Id = 0
			}
			require.NoError(t, db.Create(&logs).Error)
			t.Cleanup(func() { _ = db.Where("user_id IN ?", []int{31001, 31002}).Delete(&Log{}).Error })
			LOG_DB = db
			common.SetLogDatabaseType(tc.kind)
			sameEngine, err := GetTokenUsageExport(ctx, 31001, "_100%", 100, 200)
			require.NoError(t, err)
			assert.Equal(t, exported, sameEngine)
			// Retention, not a record count, controls how long hits remain.
			require.NoError(t, DeleteContentAuditRecords(ctx, 31001, ""))
			auditRows := make([]ContentAuditRecord, 1001)
			for i := range auditRows {
				auditRows[i] = ContentAuditRecord{Slot: 11000 + i, ID: uuid.NewString(), UserID: 31001, CreatedAt: now, Action: "warn"}
			}
			require.NoError(t, db.CreateInBatches(auditRows, 100).Error)
			require.NoError(t, db.Model(&ContentAuditSequence{}).Where("id = ?", 1).Update("next_slot", 0).Error)
			require.NoError(t, InsertContentAuditRecord(ctx, ContentAuditRecord{UserID: 31001, CreatedAt: now + 1, Action: "warn", RequestID: "newest"}, details))
			retained, total, err := ListContentAuditRecords(ctx, 31001, ContentAuditFilter{Page: 1, PageSize: 20})
			require.NoError(t, err)
			assert.EqualValues(t, 1002, total)
			require.NotEmpty(t, retained)
			assert.Equal(t, "newest", retained[0].RequestID)
			t.Logf("verified dialect %s, fresh audit tables, repeated migrations, preserved legacy token, isolated and same-engine log DB", tc.name)
		})
	}
}
