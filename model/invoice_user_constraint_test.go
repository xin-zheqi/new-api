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

func TestDropInvoiceUserConstraintKeepsFinancialRecordAndAllowsHardDelete(t *testing.T) {
	originalDB := DB
	originalMainType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invoice-user-constraint.db")), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = originalDB
		common.SetMainDatabaseType(originalMainType)
		initCol()
	})

	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, DB.Exec(`CREATE TABLE invoice_applications (
		id integer PRIMARY KEY,
		user_id integer NOT NULL,
		invoice_title text NOT NULL,
		pdf_path text,
		CONSTRAINT fk_invoice_applications_user FOREIGN KEY (user_id) REFERENCES users(id)
	)`).Error)
	user := User{Username: "invoice-retention", Password: "password", AffCode: "invoice-retention"}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Exec(
		"INSERT INTO invoice_applications (id, user_id, invoice_title, pdf_path) VALUES (?, ?, ?, ?)",
		1, user.Id, "Retained invoice", "invoice-pdfs/invoice-1.pdf",
	).Error)

	require.True(t, DB.Migrator().HasConstraint(&InvoiceApplication{}, "fk_invoice_applications_user"))
	require.NoError(t, dropInvoiceApplicationUserConstraint())
	assert.False(t, DB.Migrator().HasConstraint(&InvoiceApplication{}, "fk_invoice_applications_user"))
	require.NoError(t, DB.Unscoped().Delete(&User{}, user.Id).Error)

	var applicationCount int64
	require.NoError(t, DB.Table("invoice_applications").Where("id = ? AND user_id = ?", 1, user.Id).Count(&applicationCount).Error)
	assert.Equal(t, int64(1), applicationCount)
}
