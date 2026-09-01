package model

import (
	"bytes"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestGormLoggerDoesNotRenderQueryParameters(t *testing.T) {
	var output bytes.Buffer
	config := newGormConfig(false, &output)
	config.Logger = config.Logger.LogMode(gormlogger.Info)
	db, err := gorm.Open(sqlite.Open(":memory:"), config)
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE credentials (value TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO credentials (value) VALUES (?)", "database-secret-value").Error)

	assert.NotContains(t, output.String(), "database-secret-value")
	assert.Contains(t, output.String(), "INSERT INTO credentials")
}
