package model

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Channel settings remain JSON text in the existing column. Exercise updating
// a legacy row, reading the mapping back, and removing it on every dialect.
func TestChannelReasoningEffortMappingDatabaseMatrix(t *testing.T) {
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
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "effort_mapping_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() {
				assert.NoError(t, db.Migrator().DropTable(&Channel{}))
				assert.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Channel{}))
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)

			legacySetting := `{"thinking_to_content":true,"reasoning_content_repair":true,"tool_output_repair":true,"proxy":"http://localhost:8080"}`
			channel := Channel{Name: "legacy-channel", Key: "fixture-key", Status: 1, Setting: &legacySetting}
			require.NoError(t, db.Create(&channel).Error)
			var loaded Channel
			require.NoError(t, db.First(&loaded, channel.Id).Error)
			settings := loaded.GetSetting()
			assert.Empty(t, settings.ReasoningEffortMapping)
			settings.ReasoningEffortMapping = map[string]string{"xhigh": "max", "high": "medium"}
			loaded.SetSetting(settings)
			require.NoError(t, loaded.ValidateSettings())
			require.NoError(t, db.Model(&loaded).Select("setting").Updates(&loaded).Error)
			require.NoError(t, db.First(&loaded, channel.Id).Error)
			assert.Equal(t, settings, loaded.GetSetting())
			assert.Equal(t, "fixture-key", loaded.Key)
			assert.Equal(t, "legacy-channel", loaded.Name)

			settings.ReasoningEffortMapping = nil
			loaded.SetSetting(settings)
			require.NoError(t, db.Model(&loaded).Select("setting").Updates(&loaded).Error)
			require.NoError(t, db.First(&loaded, channel.Id).Error)
			assert.Equal(t, settings, loaded.GetSetting())
			require.NotNil(t, loaded.Setting)
			assert.NotContains(t, *loaded.Setting, "reasoning_effort_mapping")
			assert.True(t, loaded.GetSetting().ReasoningContentRepair)
			assert.True(t, loaded.GetSetting().ToolOutputRepair)
			assert.Equal(t, "http://localhost:8080", loaded.GetSetting().Proxy)
		})
	}
}
