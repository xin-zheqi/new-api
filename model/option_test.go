package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func useOptionTestDB(t *testing.T, db *gorm.DB, databaseType common.DatabaseType) {
	t.Helper()
	originalDB := DB
	originalType := common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(databaseType)
	initCol()
	t.Cleanup(func() {
		DB = originalDB
		common.SetMainDatabaseType(originalType)
		initCol()
	})
}

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

func TestUpdateOptionRejectsInvalidRatioBeforePersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invalid-ratio.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	useOptionTestDB(t, db, common.DatabaseTypeSQLite)

	err = UpdateOption("ModelRatio", `{"model":-1}`)
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&Option{}).Where(commonKeyCol+" = ?", "ModelRatio").Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateOptionRejectsInvalidPaymentAmountConfigurationBeforePersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invalid-payment-config.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	useOptionTestDB(t, db, common.DatabaseTypeSQLite)

	testCases := []struct {
		key   string
		value string
	}{
		{key: "payment_setting.amount_options", value: `[10,-1]`},
		{key: "payment_setting.amount_discount", value: `{"100":0}`},
		{key: "payment_setting.amount_discount", value: `{"invalid":0.8}`},
		{key: "payment_setting.amount_discount", value: `{" 100 ":0.8}`},
	}
	for _, testCase := range testCases {
		err = UpdateOption(testCase.key, testCase.value)
		require.Error(t, err)
	}

	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateOptionRejectsInvalidExchangeRateBeforePersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invalid-exchange-rate.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	useOptionTestDB(t, db, common.DatabaseTypeSQLite)

	for _, value := range []string{"0", "-1", "NaN", "+Inf"} {
		err = UpdateOption("USDExchangeRate", value)
		require.Error(t, err, "exchange rate %q must be rejected", value)
	}

	var count int64
	require.NoError(t, db.Model(&Option{}).Where(commonKeyCol+" = ?", "USDExchangeRate").Count(&count).Error)
	assert.Zero(t, count)
}

func TestValidatedNumericOptionsAreNormalizedBeforePersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "normalized-number.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	useOptionTestDB(t, db, common.DatabaseTypeSQLite)

	originalQuotaPerUnit := common.QuotaPerUnit
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, UpdateOption("QuotaPerUnit", " 7.25 "))
	assert.Equal(t, 7.25, common.QuotaPerUnit)
	var option Option
	require.NoError(t, db.Where(commonKeyCol+" = ?", "QuotaPerUnit").First(&option).Error)
	assert.Equal(t, "7.25", option.Value)

	require.NoError(t, UpdateOptionsBulk(map[string]string{"QuotaPerUnit": " 8.5 "}))
	assert.Equal(t, 8.5, common.QuotaPerUnit)
	require.NoError(t, db.Where(commonKeyCol+" = ?", "QuotaPerUnit").First(&option).Error)
	assert.Equal(t, "8.5", option.Value)
}

func TestGetOptionValueReadsCurrentDatabaseState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "read-option.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "payment_setting.mall_enabled", Value: "true"}).Error)
	useOptionTestDB(t, db, common.DatabaseTypeSQLite)

	value, found, err := GetOptionValue("payment_setting.mall_enabled")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "true", value)

	value, found, err = GetOptionValue("missing")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
}

func TestOptionReadsWorkAcrossSupportedServerDatabases(t *testing.T) {
	testCases := []struct {
		name      string
		envName   string
		dbType    common.DatabaseType
		dialector func(string) gorm.Dialector
	}{
		{
			name:    "mysql",
			envName: "TEST_MYSQL_DSN",
			dbType:  common.DatabaseTypeMySQL,
			dialector: func(dsn string) gorm.Dialector {
				return mysql.Open(dsn)
			},
		},
		{
			name:    "postgresql",
			envName: "TEST_POSTGRES_DSN",
			dbType:  common.DatabaseTypePostgreSQL,
			dialector: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{
					DSN:                  dsn,
					PreferSimpleProtocol: true,
				})
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(testCase.envName))
			if dsn == "" {
				t.Skipf("%s is not configured", testCase.envName)
			}

			db, err := gorm.Open(testCase.dialector(dsn), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&Option{}))

			useOptionTestDB(t, db, testCase.dbType)

			const key = "test.option.reserved-key-read"
			require.NoError(t, db.Where(commonKeyCol+" = ?", key).Delete(&Option{}).Error)
			require.NoError(t, db.Create(&Option{Key: key, Value: "preserved"}).Error)
			t.Cleanup(func() {
				require.NoError(t, db.Where(commonKeyCol+" = ?", key).Delete(&Option{}).Error)
				sqlDB, dbErr := db.DB()
				if dbErr == nil {
					require.NoError(t, sqlDB.Close())
				}
			})

			value, found, err := GetOptionValue(key)
			require.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, "preserved", value)

			values, err := GetOptionValues([]string{key, "missing"})
			require.NoError(t, err)
			assert.Equal(t, map[string]string{key: "preserved"}, values)
		})
	}
}

func TestLoadInvoiceSettingsUsesCurrentDatabaseStateAndRejectsInvalidPolicy(t *testing.T) {
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
	useOptionTestDB(t, db, common.DatabaseTypeSQLite)

	settings, err := LoadInvoiceSettings()
	require.NoError(t, err)
	assert.True(t, settings.Enabled)
	assert.Equal(t, 18, settings.ApplicationDay)
	assert.Equal(t, 365, settings.LookbackDays)
	assert.Equal(t, 4, settings.MonthlyLimit)

	require.NoError(t, db.Model(&Option{}).
		Where(commonKeyCol+" = ?", "InvoiceLookbackDays").
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
