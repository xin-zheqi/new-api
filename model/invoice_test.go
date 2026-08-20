package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInvoiceTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&InvoiceApplication{}, &InvoiceApplicationItem{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&InvoiceApplicationItem{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&InvoiceApplication{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UserSubscription{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&SubscriptionPlan{}).Error)
}

func TestCreateInvoiceApplicationUsesEligibleSubscriptionsOnce(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{Title: "Invoice plan", DurationValue: 1, DurationUnit: SubscriptionDurationMonth, TotalAmount: 1000}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "invoice-user", Password: "password", Identity: UserIdentityEnterprise, AffCode: "invoice-user"}
	require.NoError(t, DB.Create(&user).Error)

	subscriptions := []UserSubscription{
		{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1200, Status: "active", Source: "redemption", InvoiceEligible: true},
		{UserId: user.Id, PlanId: plan.Id, AmountTotal: 800, Status: "active", Source: "redemption", InvoiceEligible: true},
	}
	require.NoError(t, DB.Create(&subscriptions).Error)

	application, err := CreateInvoiceApplication(user.Id, "Example University", []int{subscriptions[0].Id, subscriptions[1].Id})
	require.NoError(t, err)
	assert.Equal(t, int64(2000), application.TotalAmount)
	assert.Len(t, application.Items, 2)

	_, err = CreateInvoiceApplication(user.Id, "Example University", []int{subscriptions[0].Id})
	require.ErrorContains(t, err, "already submitted this month")
}

func TestCreateInvoiceApplicationRejectsIneligibleSubscription(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{Title: "Non-invoice plan", DurationValue: 1, DurationUnit: SubscriptionDurationMonth, TotalAmount: 1000}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "invoice-ineligible-user", Password: "password", Identity: UserIdentityUniversity, AffCode: "invoice-ineligible-user"}
	require.NoError(t, DB.Create(&user).Error)
	subscription := UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, Status: "active", Source: "redemption"}
	require.NoError(t, DB.Create(&subscription).Error)

	_, err := CreateInvoiceApplication(user.Id, "Example University", []int{subscription.Id})
	require.ErrorContains(t, err, "not eligible")
}
