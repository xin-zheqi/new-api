package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPeriodicTokenQuotaReservesAndSettlesAcrossDatabaseDialects(t *testing.T) {
	type databaseCase struct {
		name      string
		kind      common.DatabaseType
		dialector gorm.Dialector
	}
	cases := []databaseCase{{
		name: "sqlite", kind: common.DatabaseTypeSQLite,
		dialector: sqlite.Open(filepath.Join(t.TempDir(), "period-dialect.db")),
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
			previousDB := DB
			previousRedis := common.RedisEnabled
			mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.RedisEnabled = false
			common.SetDatabaseTypes(tc.kind, logType)
			t.Cleanup(func() {
				DB = previousDB
				common.RedisEnabled = previousRedis
				common.SetDatabaseTypes(mainType, logType)
				_ = connection.Close()
			})
			require.NoError(t, db.AutoMigrate(&Token{}))
			start := QuotaPeriodStart(QuotaPeriodDay, time.Now())
			token := Token{
				UserId: 1, Key: "period-dialect-" + common.GetRandomString(12),
				Name: "period-dialect", Status: common.TokenStatusEnabled, ExpiredTime: -1,
				UnlimitedQuota: true, QuotaPeriod: QuotaPeriodDay,
				QuotaPeriodLimit: 100, QuotaPeriodStart: start,
			}
			require.NoError(t, db.Create(&token).Error)
			t.Cleanup(func() { _ = db.Unscoped().Delete(&token).Error })

			reserved, err := TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 80)
			require.NoError(t, err)
			require.True(t, reserved)
			reserved, err = TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 21)
			require.NoError(t, err)
			assert.False(t, reserved)
			require.NoError(t, AdjustTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 40))
			var stored Token
			require.NoError(t, db.First(&stored, token.Id).Error)
			assert.Equal(t, 100, stored.QuotaPeriodUsed)
			reserved, err = TryReserveTokenPeriodQuota(token.Id, token.Key, QuotaPeriodDay, 0)
			require.NoError(t, err)
			assert.False(t, reserved)
		})
	}
}
