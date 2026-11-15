package model

import (
	"math"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTokenModelUsageTotalsSummaryPreservesInt64Range(t *testing.T) {
	for _, tc := range []struct {
		name    string
		totals  []*tokenModelUsageTotalAggregate
		want    TokenModelUsageSummary
		wantErr bool
	}{
		{
			name: "no requests",
			want: TokenModelUsageSummary{TotalKeyCount: 4},
		},
		{
			name: "free requests remain active and reporting is not clamped to billing limits",
			totals: []*tokenModelUsageTotalAggregate{
				{Quota: 3_000_000_000, PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, Requests: 2},
				{Quota: 0, PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10, Requests: 1},
			},
			want: TokenModelUsageSummary{TotalKeyCount: 4, ActiveKeyCount: 2, Quota: 3_000_000_000, PromptTokens: 17, CompletionTokens: 8, TotalTokens: 25, Requests: 3},
		},
		{
			name: "positive intermediate overflow can be canceled by historical negative values",
			totals: []*tokenModelUsageTotalAggregate{
				{Quota: math.MaxInt64}, {Quota: 1}, {Quota: -1},
			},
			want: TokenModelUsageSummary{TotalKeyCount: 4, ActiveKeyCount: 3, Quota: math.MaxInt64},
		},
		{
			name: "negative intermediate overflow can be canceled by historical positive values",
			totals: []*tokenModelUsageTotalAggregate{
				{PromptTokens: math.MinInt64}, {PromptTokens: -1}, {PromptTokens: 1},
			},
			want: TokenModelUsageSummary{TotalKeyCount: 4, ActiveKeyCount: 3, PromptTokens: math.MinInt64},
		},
		{
			name: "unrepresentable positive total returns an error",
			totals: []*tokenModelUsageTotalAggregate{
				{Quota: math.MaxInt64}, {Quota: 1},
			},
			wantErr: true,
		},
		{
			name: "unrepresentable negative total returns an error",
			totals: []*tokenModelUsageTotalAggregate{
				{TotalTokens: math.MinInt64}, {TotalTokens: -1},
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := summarizeTokenModelUsageTotals(4, tc.totals)
			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, TokenModelUsageSummary{}, got, "never return a wrapped or partial summary")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTokenModelUsageReusedSummaryDatabaseMatrix(t *testing.T) {
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
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "usage_summary_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(common.DatabaseType(dialect), common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				assert.NoError(t, db.Migrator().DropTable(&Log{}, &Token{}))
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
				assert.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Token{}, &Log{}))
			versionSQL := "SELECT version()"
			if dialect == "sqlite" {
				versionSQL = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
			t.Logf("database: %s %s", dialect, version)
			testTokenModelUsageReusedSummary(t)
		})
	}
}

func testTokenModelUsageReusedSummary(t *testing.T) {
	t.Helper()
	tokens := []Token{
		{Id: 101, UserId: 1010, Name: "team paid", Key: "test-private-usage-key-101", Status: 1},
		{Id: 102, UserId: 1010, Name: "team%_! free", Key: "test-private-usage-key-102", Status: 1},
		{Id: 103, UserId: 1010, Name: "team idle", Key: "test-private-usage-key-103", Status: 1},
		{Id: 104, UserId: 1010, Name: "other", Key: "test-private-usage-key-104", Status: 1},
		{Id: 105, UserId: 1010, Name: "team deleted", Key: "test-private-usage-key-105", Status: 1},
		{Id: 106, UserId: 2020, Name: "team other user", Key: "test-private-usage-key-106", Status: 1},
	}
	require.NoError(t, DB.Create(&tokens).Error)
	require.NoError(t, DB.Delete(&Token{}, 105).Error)
	logs := []Log{
		{UserId: 1010, TokenId: 101, Type: LogTypeConsume, CreatedAt: 1000, ModelName: "model-a", Quota: 100, PromptTokens: 10, CompletionTokens: 2},
		{UserId: 1010, TokenId: 101, Type: LogTypeConsume, CreatedAt: 1100, ModelName: "model-a", Quota: 50, PromptTokens: 3, CompletionTokens: 1},
		{UserId: 1010, TokenId: 102, Type: LogTypeConsume, CreatedAt: 1200, ModelName: "", Quota: 0, PromptTokens: 7, CompletionTokens: 3},
		{UserId: 1010, TokenId: 101, Type: LogTypeConsume, CreatedAt: 999, ModelName: "before-range", Quota: 999},
		{UserId: 1010, TokenId: 101, Type: LogTypeConsume, CreatedAt: 1201, ModelName: "after-range", Quota: 999},
		{UserId: 1010, TokenId: 101, Type: LogTypeError, CreatedAt: 1100, ModelName: "error-log", Quota: 999},
		{UserId: 1010, TokenId: 104, Type: LogTypeConsume, CreatedAt: 1100, ModelName: "other-name", Quota: 999},
		{UserId: 1010, TokenId: 105, Type: LogTypeConsume, CreatedAt: 1100, ModelName: "deleted-key", Quota: 999},
		{UserId: 2020, TokenId: 101, Type: LogTypeConsume, CreatedAt: 1100, ModelName: "wrong-log-owner", Quota: 999},
		{UserId: 2020, TokenId: 106, Type: LogTypeConsume, CreatedAt: 1100, ModelName: "other-user", Quota: 999},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)
	allSummary := TokenModelUsageSummary{TotalKeyCount: 3, ActiveKeyCount: 2, ModelCount: 1, Quota: 150, PromptTokens: 20, CompletionTokens: 6, TotalTokens: 26, Requests: 3}
	for _, tc := range []struct {
		name, keyword string
		start, end    int64
		offset, limit int
		wantIDs       []int
		wantSummary   TokenModelUsageSummary
	}{
		{name: "all matching tokens", keyword: " team ", start: 1000, end: 1200, limit: 100, wantIDs: []int{101, 103, 102}, wantSummary: allSummary},
		{name: "one page retains full summary", keyword: "team", start: 1000, end: 1200, offset: 1, limit: 1, wantIDs: []int{103}, wantSummary: allSummary},
		{name: "literal keyword characters and free empty-model request", keyword: "%_!", start: 1000, end: 1200, limit: 100, wantIDs: []int{102}, wantSummary: TokenModelUsageSummary{TotalKeyCount: 1, ActiveKeyCount: 1, PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10, Requests: 1}},
		{name: "time boundary is inclusive", keyword: "team", start: 1200, end: 1200, limit: 100, wantIDs: []int{103, 102, 101}, wantSummary: TokenModelUsageSummary{TotalKeyCount: 3, ActiveKeyCount: 1, PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10, Requests: 1}},
		{name: "matching tokens without logs", keyword: "team", start: 5000, end: 6000, limit: 100, wantIDs: []int{103, 102, 101}, wantSummary: TokenModelUsageSummary{TotalKeyCount: 3}},
		{name: "no matching tokens", keyword: "missing", start: 1000, end: 1200, limit: 100, wantIDs: []int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, total, summary, err := GetUserTokenModelUsage(1010, tc.keyword, tc.start, tc.end, tc.offset, tc.limit)
			require.NoError(t, err)
			assert.Equal(t, tc.wantSummary.TotalKeyCount, total)
			assert.Equal(t, tc.wantSummary, summary)
			ids := make([]int, 0, len(items))
			for _, item := range items {
				ids = append(ids, item.TokenId)
				assert.NotContains(t, item.Key, "private")
				assert.NotContains(t, item.TokenName, "deleted")
				assert.NotContains(t, item.TokenName, "other user")
			}
			assert.Equal(t, tc.wantIDs, ids)
			standalone, err := GetUserTokenModelUsageSummary(1010, tc.keyword, tc.start, tc.end)
			require.NoError(t, err)
			assert.Equal(t, standalone, summary)
		})
	}
}
