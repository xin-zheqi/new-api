package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestManagedUserQueriesEnforceRoleScopeAndOmitSettings(t *testing.T) {
	originalDB := DB
	originalDatabaseType := common.MainDatabaseType()
	t.Cleanup(func() {
		DB = originalDB
		common.SetMainDatabaseType(originalDatabaseType)
		initCol()
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()

	users := []User{
		{Username: "root", Password: "root-password", Role: common.RoleRootUser, AffCode: "root-aff", Setting: `{"webhook_secret":"root-secret"}`, StripeCustomer: "cus_root"},
		{Username: "admin", Password: "admin-password", Role: common.RoleAdminUser, AffCode: "admin-aff", Setting: `{"gotify_token":"admin-secret"}`, StripeCustomer: "cus_admin"},
		{Username: "member", Password: "member-password", Role: common.RoleCommonUser, AffCode: "member-aff", Setting: `{"webhook_secret":"member-secret"}`, StripeCustomer: "cus_member"},
	}
	require.NoError(t, db.Create(&users).Error)

	page := &common.PageInfo{Page: 1, PageSize: 20}
	adminResults, total, err := GetAllUsers(page, common.RoleAdminUser)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, adminResults, 1)
	assert.Equal(t, "member", adminResults[0].Username)
	assert.Empty(t, adminResults[0].Password)
	assert.Empty(t, adminResults[0].Setting)
	assert.Empty(t, adminResults[0].StripeCustomer)

	searchResults, total, err := SearchUsers("root", "", nil, nil, 0, 20, common.RoleAdminUser)
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, searchResults)

	rootResults, total, err := GetAllUsers(page, common.RoleRootUser)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Len(t, rootResults, 3)
	for _, user := range rootResults {
		assert.Empty(t, user.Password)
		assert.Empty(t, user.Setting)
		assert.Empty(t, user.StripeCustomer)
	}
}
