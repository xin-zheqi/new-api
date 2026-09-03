package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type legacyTopUpWithoutCreditedQuota struct {
	Id      int
	UserId  int
	Amount  int64
	Money   float64
	TradeNo string `gorm:"unique;type:varchar(255);index"`
}

func testTopUpCreditedQuotaMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	tableName := fmt.Sprintf("topup_quota_%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = db.Migrator().DropTable(tableName) })

	legacyDB := db.Table(tableName)
	require.NoError(t, legacyDB.AutoMigrate(&legacyTopUpWithoutCreditedQuota{}))
	require.NoError(t, legacyDB.Create(map[string]interface{}{
		"id": 1, "user_id": 2, "amount": 3, "money": 4, "trade_no": "legacy",
	}).Error)
	assert.False(t, db.Migrator().HasColumn(tableName, "credited_quota"))

	for range 2 {
		require.NoError(t, db.Table(tableName).AutoMigrate(&TopUp{}))
	}

	assert.True(t, db.Migrator().HasColumn(tableName, "credited_quota"))
	var topUp TopUp
	require.NoError(t, db.Table(tableName).Where("trade_no = ?", "legacy").First(&topUp).Error)
	assert.Zero(t, topUp.CreditedQuota)
	assert.Equal(t, int64(3), topUp.Amount)
}

func TestTopUpCreditedQuotaMigrationSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy-topup.db")), &gorm.Config{})
	require.NoError(t, err)
	testTopUpCreditedQuotaMigration(t, db)
}

func TestTopUpCreditedQuotaMigrationMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	testTopUpCreditedQuotaMigration(t, db)
}

func TestTopUpCreditedQuotaMigrationPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: dsn, PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	testTopUpCreditedQuotaMigration(t, db)
}
