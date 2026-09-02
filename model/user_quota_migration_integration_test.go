package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEnsureUserQuotaColumnsUpgradesLegacyDatabases(t *testing.T) {
	testCases := []struct {
		name      string
		envName   string
		dbType    common.DatabaseType
		open      func(string) gorm.Dialector
		createSQL string
	}{
		{
			name:    "mysql",
			envName: "TEST_MYSQL_DSN",
			dbType:  common.DatabaseTypeMySQL,
			open: func(dsn string) gorm.Dialector {
				return gormmysql.Open(dsn)
			},
			createSQL: "CREATE TABLE users (id BIGINT PRIMARY KEY, quota INT DEFAULT 0, used_quota INT DEFAULT 0, aff_quota INT DEFAULT 0, aff_history INT DEFAULT 0)",
		},
		{
			name:      "postgresql",
			envName:   "TEST_POSTGRES_DSN",
			dbType:    common.DatabaseTypePostgreSQL,
			open:      postgres.Open,
			createSQL: `CREATE TABLE users (id BIGINT PRIMARY KEY, quota INTEGER DEFAULT 0, used_quota INTEGER DEFAULT 0, aff_quota INTEGER DEFAULT 0, aff_history INTEGER DEFAULT 0)`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.envName)
			if dsn == "" {
				t.Skipf("%s is not configured", tc.envName)
			}
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
			t.Cleanup(func() {
				require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
				sqlDB, dbErr := db.DB()
				if dbErr == nil {
					require.NoError(t, sqlDB.Close())
				}
			})
			require.NoError(t, db.Exec(tc.createSQL).Error)
			require.NoError(t, db.Exec(
				"INSERT INTO users (id, quota, used_quota, aff_quota, aff_history) VALUES (?, ?, ?, ?, ?)",
				1, 2_000_000_000, 1_900_000_000, 1_800_000_000, 1_700_000_000,
			).Error)

			require.NoError(t, ensureUserQuotaColumns(db, tc.dbType))
			require.NoError(t, ensureUserQuotaColumns(db, tc.dbType), "migration must be idempotent")

			columnTypes, err := db.Migrator().ColumnTypes(&User{})
			require.NoError(t, err)
			actualTypes := make(map[string]string, len(columnTypes))
			for _, columnType := range columnTypes {
				actualTypes[columnType.Name()] = columnType.DatabaseTypeName()
			}
			for _, quotaColumn := range userQuotaColumns {
				assert.True(t, is64BitIntegerType(tc.dbType, actualTypes[quotaColumn.column]), "%s should be a 64-bit integer", quotaColumn.column)
			}

			require.NoError(t, db.Exec("UPDATE users SET quota = ? WHERE id = ?", int64(4_000_000_000), 1).Error)
			var quota int64
			require.NoError(t, db.Table("users").Where("id = ?", 1).Pluck("quota", &quota).Error)
			assert.Equal(t, int64(4_000_000_000), quota)
		})
	}
}
