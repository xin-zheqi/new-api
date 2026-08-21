package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionReturnsPersistenceErrorWithoutUpdatingMemory(t *testing.T) {
	originalDB := DB
	originalTopUpLink := common.TopUpLink
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{"TopUpLink": "before"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = originalDB
		common.TopUpLink = originalTopUpLink
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	common.TopUpLink = "before"

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "options.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB = db

	err = UpdateOption("TopUpLink", "after")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `persist option "TopUpLink"`)
	assert.Equal(t, "before", common.TopUpLink)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "before", common.OptionMap["TopUpLink"])
	common.OptionMapRWMutex.RUnlock()
}

func TestGetOptionValueReadsCurrentDatabaseState(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "read-option.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "payment_setting.mall_enabled", Value: "true"}).Error)
	DB = db

	value, found, err := GetOptionValue("payment_setting.mall_enabled")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "true", value)

	value, found, err = GetOptionValue("missing")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
}

func TestLoadInvoiceSettingsUsesCurrentDatabaseStateAndRejectsInvalidPolicy(t *testing.T) {
	originalDB := DB
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{
		"InvoiceEnabled":        "false",
		"InvoiceApplicationDay": "1",
		"InvoiceLookbackDays":   "1",
		"InvoiceMonthlyLimit":   "1",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = originalDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invoice-settings.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create([]Option{
		{Key: "InvoiceEnabled", Value: "true"},
		{Key: "InvoiceApplicationDay", Value: "18"},
		{Key: "InvoiceLookbackDays", Value: "365"},
		{Key: "InvoiceMonthlyLimit", Value: "4"},
	}).Error)
	DB = db

	settings, err := LoadInvoiceSettings()
	require.NoError(t, err)
	assert.True(t, settings.Enabled)
	assert.Equal(t, 18, settings.ApplicationDay)
	assert.Equal(t, 365, settings.LookbackDays)
	assert.Equal(t, 4, settings.MonthlyLimit)

	require.NoError(t, db.Model(&Option{}).
		Where("key = ?", "InvoiceLookbackDays").
		Update("value", "invalid").Error)
	settings, err = LoadInvoiceSettings()
	require.Error(t, err)
	assert.False(t, settings.Enabled)
}

func TestUpdateOptionsBulkRollsBackTheWholePairBeforeUpdatingMemory(t *testing.T) {
	originalDB := DB
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{
		"payment_setting.mall_enabled": "false",
		"MallURL":                      "https://old.example.com",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = originalDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "bulk-options.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create([]Option{
		{Key: "payment_setting.mall_enabled", Value: "false"},
		{Key: "MallURL", Value: "https://old.example.com"},
	}).Error)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER reject_mall_url_update
		BEFORE UPDATE ON options
		WHEN NEW.key = 'MallURL'
		BEGIN
			SELECT RAISE(FAIL, 'blocked mall URL update');
		END;
	`).Error)
	DB = db

	err = UpdateOptionsBulk(map[string]string{
		"payment_setting.mall_enabled": "true",
		"MallURL":                      "https://new.example.com",
	})
	require.Error(t, err)

	var options []Option
	require.NoError(t, db.Order("key").Find(&options).Error)
	require.Len(t, options, 2)
	values := map[string]string{}
	for _, option := range options {
		values[option.Key] = option.Value
	}
	assert.Equal(t, "false", values["payment_setting.mall_enabled"])
	assert.Equal(t, "https://old.example.com", values["MallURL"])
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "false", common.OptionMap["payment_setting.mall_enabled"])
	assert.Equal(t, "https://old.example.com", common.OptionMap["MallURL"])
	common.OptionMapRWMutex.RUnlock()
}
