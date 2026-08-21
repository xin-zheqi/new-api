package model

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInvoicePaymentSnapshotMigrationIsConservativeAndPreservesPendingCheckout(t *testing.T) {
	originalDB := DB
	dsn := filepath.Join(t.TempDir(), "invoice-payment-migration.db") + "?_busy_timeout=30000&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	DB = db
	t.Cleanup(func() { DB = originalDB })
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	require.NoError(t, DB.AutoMigrate(
		&Option{}, &User{}, &Redemption{}, &TopUp{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{},
		&InvoiceApplication{}, &InvoiceApplicationItem{},
	))

	user := User{
		Username: "legacy-invoice-user", Password: "password", AffCode: "legacy-invoice-user",
		Identity: UserIdentityUniversity, IdentityRequested: UserIdentityEnterprise, IdentityReviewStatus: "pending",
	}
	require.NoError(t, DB.Create(&user).Error)
	plan := SubscriptionPlan{Title: "Legacy plan", Currency: "EUR", DurationUnit: SubscriptionDurationMonth, DurationValue: 1}
	require.NoError(t, DB.Create(&plan).Error)
	legacySubscription := UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: 50_000, Status: "active", Source: "order",
		InvoiceEligible: true, PaidAmountMicros: 50_000, PaidCurrency: "USD",
	}
	require.NoError(t, DB.Create(&legacySubscription).Error)
	orders := []SubscriptionOrder{
		{UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "legacy-stripe-pending", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending},
		{UserId: user.Id, PlanId: plan.Id, Money: 12.34, TradeNo: "legacy-creem-pending", PaymentProvider: PaymentProviderCreem, Status: common.TopUpStatusPending},
		{UserId: user.Id, PlanId: plan.Id, Money: 1.005, TradeNo: "legacy-epay-pending", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending},
		{UserId: user.Id, PlanId: plan.Id, Money: 20, TradeNo: "legacy-stripe-paid", PaymentProvider: PaymentProviderStripe, PaidCurrency: "USD", Status: common.TopUpStatusSuccess},
	}
	require.NoError(t, DB.Create(&orders).Error)
	legacyTopUps := []TopUp{
		{UserId: user.Id, TradeNo: orders[0].TradeNo, Status: common.TopUpStatusSuccess},
		{UserId: user.Id, TradeNo: "legacy-recharge", Status: common.TopUpStatusSuccess},
		{UserId: user.Id, TradeNo: orders[3].TradeNo, Status: common.TopUpStatusSuccess},
	}
	require.NoError(t, DB.Create(&legacyTopUps).Error)
	pendingApplication := InvoiceApplication{
		UserId: user.Id, ApplicationMonth: "2026-08", InvoiceTitle: "Legacy pending invoice",
		LegacyTotalAmount: 50_000, TotalAmountMicros: 50_000, Currency: "USD", Status: InvoiceApplicationStatusPending,
	}
	require.NoError(t, DB.Create(&pendingApplication).Error)
	rejectedApplication := InvoiceApplication{
		UserId: user.Id, ApplicationMonth: "2026-07", InvoiceTitle: "Legacy rejected invoice",
		Status: InvoiceApplicationStatusRejected,
	}
	require.NoError(t, DB.Create(&rejectedApplication).Error)
	activeSlot := 1
	items := []InvoiceApplicationItem{
		{InvoiceApplicationId: pendingApplication.Id, UserSubscriptionId: legacySubscription.Id, ActiveSlot: nil, LegacyAmountTotal: 50_000, PaidAmountMicros: 50_000, Currency: "USD"},
		{InvoiceApplicationId: rejectedApplication.Id, UserSubscriptionId: legacySubscription.Id + 1, ActiveSlot: &activeSlot},
	}
	require.NoError(t, DB.Create(&items).Error)

	require.NoError(t, normalizeInvoiceApplicationItemIndex())
	var normalizedItems []InvoiceApplicationItem
	require.NoError(t, DB.Order("id").Find(&normalizedItems).Error)
	require.Len(t, normalizedItems, 2)
	require.NotNil(t, normalizedItems[0].ActiveSlot)
	assert.Equal(t, 1, *normalizedItems[0].ActiveSlot)
	assert.Nil(t, normalizedItems[1].ActiveSlot)

	start := make(chan struct{})
	migrationErrors := make(chan error, 8)
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			migrationErrors <- migrateInvoicePaymentSnapshots()
		}()
	}
	close(start)
	wait.Wait()
	close(migrationErrors)
	for migrationErr := range migrationErrors {
		require.NoError(t, migrationErr)
	}
	var migration Option
	require.NoError(t, DB.Where("key = ?", invoicePaymentSnapshotMigrationKey).First(&migration).Error)
	assert.Equal(t, "completed", migration.Value)
	var migratedUser User
	require.NoError(t, DB.First(&migratedUser, user.Id).Error)
	assert.Equal(t, UserIdentityUniversity, migratedUser.Identity)
	assert.Equal(t, UserIdentityEnterprise, migratedUser.IdentityRequested)
	assert.Equal(t, "pending", migratedUser.IdentityReviewStatus)
	var migratedSubscription UserSubscription
	require.NoError(t, DB.First(&migratedSubscription, legacySubscription.Id).Error)
	assert.False(t, migratedSubscription.InvoiceEligible)
	assert.Zero(t, migratedSubscription.PaidAmountMicros)
	assert.Empty(t, migratedSubscription.PaidCurrency)
	var migratedOrders []SubscriptionOrder
	require.NoError(t, DB.Order("id").Find(&migratedOrders).Error)
	require.Len(t, migratedOrders, 4)
	assert.Equal(t, int64(9_990_000), migratedOrders[0].ExpectedAmountMicros)
	assert.Equal(t, "EUR", migratedOrders[0].ExpectedCurrency)
	assert.Equal(t, int64(12_340_000), migratedOrders[1].ExpectedAmountMicros)
	assert.Empty(t, migratedOrders[1].ExpectedCurrency)
	assert.Equal(t, int64(1_000_000), migratedOrders[2].ExpectedAmountMicros)
	assert.Equal(t, "CNY", migratedOrders[2].ExpectedCurrency)
	var migratedApplication InvoiceApplication
	require.NoError(t, DB.First(&migratedApplication, pendingApplication.Id).Error)
	assert.Equal(t, InvoiceApplicationStatusRejected, migratedApplication.Status)
	assert.Contains(t, migratedApplication.RejectionReason, "no verifiable payment amount")
	assert.Zero(t, migratedApplication.TotalAmountMicros)
	assert.Empty(t, migratedApplication.Currency)
	var migratedItem InvoiceApplicationItem
	require.NoError(t, DB.First(&migratedItem, items[0].Id).Error)
	assert.Nil(t, migratedItem.ActiveSlot)
	assert.Zero(t, migratedItem.PaidAmountMicros)
	assert.Empty(t, migratedItem.Currency)

	currentSubscription := UserSubscription{
		UserId: user.Id, PlanId: plan.Id, Status: "active", Source: "order", InvoiceEligible: true,
		PaidAmountMicros: 20_000_000, PaidCurrency: "USD",
	}
	require.NoError(t, DB.Create(&currentSubscription).Error)
	require.NoError(t, migrateInvoicePaymentSnapshots())
	require.NoError(t, DB.First(&currentSubscription, currentSubscription.Id).Error)
	assert.True(t, currentSubscription.InvoiceEligible)
	assert.Equal(t, int64(20_000_000), currentSubscription.PaidAmountMicros)

	require.NoError(t, migrateTopUpSources())
	var migratedTopUps []TopUp
	require.NoError(t, DB.Order("id").Find(&migratedTopUps).Error)
	require.Len(t, migratedTopUps, 3)
	assert.Equal(t, TopUpSourceSubscription, migratedTopUps[0].Source)
	assert.Equal(t, TopUpSourceRecharge, migratedTopUps[1].Source)
	assert.Equal(t, TopUpSourceSubscription, migratedTopUps[2].Source)
	assert.Equal(t, "USD", migratedTopUps[2].Currency)
}

func TestInvoiceApplicationItemIndexMigrationRepairsDuplicateLegacyRows(t *testing.T) {
	originalDB := DB
	originalMainType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invoice-item-index.db")), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = originalDB
		common.SetMainDatabaseType(originalMainType)
		initCol()
	})

	require.NoError(t, DB.AutoMigrate(&Option{}, &InvoiceApplication{}, &InvoiceApplicationItem{}))
	// The runtime model deliberately has no unique index. This is the state
	// needed to upgrade a legacy database before its duplicate rows are fixed.
	assert.False(t, DB.Migrator().HasIndex(&InvoiceApplicationItem{}, "idx_invoice_subscription_active"))

	activeSlot := 1
	applications := []InvoiceApplication{
		{UserId: 1, ApplicationMonth: "2026-08", InvoiceTitle: "Older", Status: InvoiceApplicationStatusCompleted, CreatedAt: 100},
		{UserId: 1, ApplicationMonth: "2026-08", InvoiceTitle: "Newer", Status: InvoiceApplicationStatusPending, CreatedAt: 200},
		{UserId: 1, ApplicationMonth: "2026-07", InvoiceTitle: "Rejected", Status: InvoiceApplicationStatusRejected, CreatedAt: 300},
	}
	require.NoError(t, DB.Create(&applications).Error)
	items := []InvoiceApplicationItem{
		{InvoiceApplicationId: applications[0].Id, UserSubscriptionId: 77, ActiveSlot: &activeSlot, PlanTitle: "Older"},
		{InvoiceApplicationId: applications[1].Id, UserSubscriptionId: 77, ActiveSlot: &activeSlot, PlanTitle: "Newer"},
		{InvoiceApplicationId: applications[2].Id, UserSubscriptionId: 77, ActiveSlot: &activeSlot, PlanTitle: "Rejected"},
	}
	require.NoError(t, DB.Create(&items).Error)

	require.NoError(t, normalizeInvoiceApplicationItemIndex())
	assert.True(t, DB.Migrator().HasIndex(&invoiceApplicationItemIndex{}, "idx_invoice_subscription_active"))
	var migrated []InvoiceApplicationItem
	require.NoError(t, DB.Order("id ASC").Find(&migrated).Error)
	require.Len(t, migrated, 3)
	assert.Nil(t, migrated[0].ActiveSlot)
	require.NotNil(t, migrated[1].ActiveSlot)
	assert.Equal(t, 1, *migrated[1].ActiveSlot)
	assert.Nil(t, migrated[2].ActiveSlot)

	duplicate := InvoiceApplicationItem{InvoiceApplicationId: applications[0].Id, UserSubscriptionId: 77, ActiveSlot: &activeSlot}
	assert.Error(t, DB.Create(&duplicate).Error)
	beforeRestart := append([]InvoiceApplicationItem(nil), migrated...)
	require.NoError(t, normalizeInvoiceApplicationItemIndex())
	var afterRestart []InvoiceApplicationItem
	require.NoError(t, DB.Order("id ASC").Find(&afterRestart).Error)
	assert.Equal(t, beforeRestart, afterRestart)
}
