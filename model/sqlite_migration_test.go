package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateDBKeepsExistingSQLiteLotteriesTable(t *testing.T) {
	originalDB := DB
	originalLogDB := LOG_DB
	originalMainDBType := common.MainDatabaseType()
	originalLogDBType := common.LogDatabaseType()
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDBType, originalLogDBType)
		initCol()
	})

	dbPath := filepath.Join(t.TempDir(), "one-api.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()

	err = db.Exec(`CREATE TABLE lotteries (
		id integer,
		title varchar(128) NOT NULL,
		description text,
		prize_name varchar(128) NOT NULL,
		mode varchar(16) NOT NULL DEFAULT 'once',
		status integer NOT NULL DEFAULT 1,
		winner_count integer NOT NULL DEFAULT 1,
		prize_per_winner integer NOT NULL DEFAULT 1,
		min_recharge_amount decimal(10,6) NOT NULL DEFAULT 0.000000,
		created_at integer,
		updated_at integer,
		PRIMARY KEY (id)
	)`).Error
	require.NoError(t, err)
	require.NoError(t, db.Exec(`INSERT INTO lotteries (id, title, prize_name) VALUES (1, 'old lottery', 'old prize')`).Error)

	require.NoError(t, migrateDB())

	var title string
	require.NoError(t, db.Raw("SELECT title FROM lotteries WHERE id = ?", 1).Scan(&title).Error)
	require.Equal(t, "old lottery", title)

	for _, column := range []string{
		"require_recharge",
		"recharge_window_days",
		"count_redemption_as_recharge",
		"min_account_age_days",
		"min_request_count",
		"require_email_verified",
		"registration_start",
		"registration_end",
		"draw_time",
		"schedule_weekdays",
		"schedule_start_time",
		"schedule_end_time",
		"schedule_draw_time",
		"created_by",
		"deleted_at",
	} {
		require.True(t, db.Migrator().HasColumn("lotteries", column), "missing column %s", column)
	}
}
