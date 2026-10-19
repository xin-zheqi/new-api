package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRecentTokenRequestsOnlyExposeOwnLastTwentyBillingSummaries(t *testing.T) {
	type databaseCase struct {
		name      string
		kind      common.DatabaseType
		dialector gorm.Dialector
	}
	cases := []databaseCase{{
		name:      "sqlite",
		kind:      common.DatabaseTypeSQLite,
		dialector: sqlite.Open(filepath.Join(t.TempDir(), "recent-requests.db")),
	}}
	if dsn := os.Getenv("TEST_MYSQL_DSN"); dsn != "" {
		cases = append(cases, databaseCase{"mysql", common.DatabaseTypeMySQL, mysql.Open(dsn)})
	}
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		cases = append(cases, databaseCase{"postgres", common.DatabaseTypePostgreSQL, postgres.Open(dsn)})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(tc.dialector, &gorm.Config{})
			require.NoError(t, err)
			connection, err := db.DB()
			require.NoError(t, err)
			oldLogDB := LOG_DB
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			LOG_DB = db
			common.SetDatabaseTypes(oldMainType, tc.kind)
			t.Cleanup(func() {
				LOG_DB = oldLogDB
				common.SetDatabaseTypes(oldMainType, oldLogType)
				_ = db.Migrator().DropTable(&Log{})
				_ = connection.Close()
			})
			require.NoError(t, db.AutoMigrate(&Log{}))

			const userID, tokenID = 41001, 42001
			for i := range 22 {
				log := Log{
					UserId: userID, TokenId: tokenID, Type: LogTypeConsume,
					CreatedAt: int64(1000 + i), ModelName: "model-a", Quota: 100 + i,
					PromptTokens: 20 + i, CompletionTokens: 5, Content: "private request content",
				}
				if i == 21 {
					log.Other = `{"cache_tokens":12,"cache_creation_tokens_5m":3,"cache_creation_tokens_1h":4,"audio_input":2,"admin_info":{"secret":"private"}}`
				}
				require.NoError(t, db.Create(&log).Error)
			}
			require.NoError(t, db.Create(&Log{UserId: userID, TokenId: tokenID, Type: LogTypeError, CreatedAt: 1022, ModelName: "model-failed", Content: "private failure"}).Error)
			require.NoError(t, db.Create(&Log{UserId: userID + 1, TokenId: tokenID, Type: LogTypeConsume, CreatedAt: 1023, ModelName: "other-user"}).Error)
			require.NoError(t, db.Create(&Log{UserId: userID, TokenId: tokenID + 1, Type: LogTypeConsume, CreatedAt: 1024, ModelName: "other-key"}).Error)
			require.NoError(t, db.Create(&Log{UserId: userID, TokenId: tokenID, Type: LogTypeManage, CreatedAt: 1025, ModelName: "not-a-request"}).Error)

			rows, err := GetRecentTokenRequests(context.Background(), userID, tokenID)
			require.NoError(t, err)
			require.Len(t, rows, 20)
			assert.Equal(t, "model-a", rows[0].ModelName)
			assert.Equal(t, 121, rows[0].Quota)
			assert.Equal(t, 12, rows[0].CacheReadTokens)
			assert.Equal(t, 3, rows[0].CacheWrite5mTokens)
			assert.Equal(t, 4, rows[0].CacheWrite1hTokens)
			assert.Equal(t, 2, rows[0].AudioInputTokens)
			assert.EqualValues(t, 1002, rows[len(rows)-1].CreatedAt)
			encoded, err := common.Marshal(rows)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "private")
			assert.NotContains(t, string(encoded), "other-user")
			assert.NotContains(t, string(encoded), "other-key")
			assert.NotContains(t, string(encoded), "not-a-request")
			assert.NotContains(t, string(encoded), "model-failed")
		})
	}
}
