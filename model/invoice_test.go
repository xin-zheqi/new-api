package model

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInvoiceTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Option{}, &SubscriptionOrder{}, &InvoiceApplication{}, &InvoiceApplicationItem{}))
	require.NoError(t, normalizeInvoiceApplicationItemIndex())
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&InvoiceApplicationItem{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&InvoiceApplication{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UserSubscription{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&SubscriptionOrder{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&SubscriptionPlan{}).Error)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	setInvoiceOptionForTest(t, "InvoiceMonthlyLimit", "1")
	setInvoiceOptionForTest(t, "InvoiceLookbackDays", "90")
}

func setInvoiceOptionForTest(t *testing.T, key, value string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previous, existed := common.OptionMap[key]
	common.OptionMap[key] = value
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		if existed {
			common.OptionMap[key] = previous
		} else {
			delete(common.OptionMap, key)
		}
		common.OptionMapRWMutex.Unlock()
	})
}

func invoiceTestInput(title string) InvoiceApplicationInput {
	return InvoiceApplicationInput{
		InvoiceTitle: title,
		TaxpayerId:   "91310000MA1K123456",
		BankName:     "中国银行上海分行",
		Remark:       "增值税普通发票",
	}
}

func createInvoiceTestSubscription(t *testing.T, username string) (User, UserSubscription) {
	t.Helper()
	plan := SubscriptionPlan{Title: "Invoice plan", DurationValue: 1, DurationUnit: SubscriptionDurationMonth, TotalAmount: 1000}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	user := User{Username: username, Password: "password", Identity: UserIdentityUniversity, AffCode: username}
	require.NoError(t, DB.Create(&user).Error)
	subscription := UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: 1200,
		Status: "active", Source: "order", InvoiceEligible: true,
		PaidAmountMicros: 12_000_000, PaidCurrency: "CNY",
	}
	require.NoError(t, DB.Create(&subscription).Error)
	return user, subscription
}

func TestCreateInvoiceApplicationRejectsUnsafeInputBounds(t *testing.T) {
	setupInvoiceTest(t)
	tests := []struct {
		name            string
		input           InvoiceApplicationInput
		subscriptionIds []int
		errorContains   string
	}{
		{name: "long title", input: invoiceTestInput(strings.Repeat("a", InvoiceTitleMaxLength+1)), subscriptionIds: []int{1}, errorContains: "must not exceed"},
		{name: "markup title", input: invoiceTestInput("<script>alert(1)</script>"), subscriptionIds: []int{1}, errorContains: "unsupported"},
		{name: "invalid taxpayer id", input: InvoiceApplicationInput{InvoiceTitle: "个人", TaxpayerId: "9131 0000"}, subscriptionIds: []int{1}, errorContains: "letters and digits"},
		{name: "control in bank", input: InvoiceApplicationInput{InvoiceTitle: "个人", BankName: "bank\x00name"}, subscriptionIds: []int{1}, errorContains: "unsupported"},
		{name: "markup in remark", input: InvoiceApplicationInput{InvoiceTitle: "个人", Remark: "<img src=x>"}, subscriptionIds: []int{1}, errorContains: "unsupported"},
		{name: "too many subscriptions", input: invoiceTestInput("Example University"), subscriptionIds: make([]int, InvoiceSubscriptionLimit+1), errorContains: "too many subscriptions"},
		{name: "duplicate subscription", input: invoiceTestInput("Example University"), subscriptionIds: []int{1, 1}, errorContains: "duplicate subscription"},
		{name: "invalid subscription", input: invoiceTestInput("Example University"), subscriptionIds: []int{0}, errorContains: "invalid subscription"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CreateInvoiceApplication(1, GetInvoiceSettings(), test.input, test.subscriptionIds)
			require.ErrorContains(t, err, test.errorContains)
		})
	}
}

func TestInvoiceEligibilityRequiresOrganizationIdentityRegardlessOfRole(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{Title: "Identity plan", DurationValue: 1, DurationUnit: SubscriptionDurationMonth, TotalAmount: 1000}
	require.NoError(t, DB.Create(&plan).Error)
	tests := []struct {
		name                 string
		identity             string
		identityRequested    string
		identityReviewStatus string
		role                 int
		allowed              bool
	}{
		{name: "personal user", identity: UserIdentityPersonal, role: common.RoleCommonUser},
		{name: "student administrator", identity: UserIdentityStudent, role: common.RoleAdminUser},
		{
			name: "pending university review", identity: UserIdentityPersonal,
			identityRequested: UserIdentityUniversity, identityReviewStatus: "pending", role: common.RoleCommonUser,
		},
		{
			name: "pending enterprise review", identity: UserIdentityPersonal,
			identityRequested: UserIdentityEnterprise, identityReviewStatus: "pending", role: common.RoleCommonUser,
		},
		{name: "university user", identity: UserIdentityUniversity, role: common.RoleCommonUser, allowed: true},
		{name: "enterprise administrator", identity: UserIdentityEnterprise, role: common.RoleAdminUser, allowed: true},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			username := fmt.Sprintf("invoice-identity-%d", index)
			user := User{
				Username: username, Password: "password", Identity: test.identity,
				IdentityRequested: test.identityRequested, IdentityReviewStatus: test.identityReviewStatus,
				Role: test.role, AffCode: username,
			}
			require.NoError(t, DB.Create(&user).Error)
			subscription := UserSubscription{
				UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, Status: "active", Source: "order",
				InvoiceEligible: true, PaidAmountMicros: 10_000_000, PaidCurrency: "CNY",
			}
			require.NoError(t, DB.Create(&subscription).Error)

			eligible, err := GetInvoiceEligibleSubscriptions(user.Id, GetInvoiceSettings())
			if !test.allowed {
				require.ErrorIs(t, err, ErrInvoiceIdentityRequired)
				assert.Empty(t, eligible)
				_, err = CreateInvoiceApplication(user.Id, GetInvoiceSettings(), invoiceTestInput("Identity invoice"), []int{subscription.Id})
				require.ErrorIs(t, err, ErrInvoiceIdentityRequired)
				return
			}

			require.NoError(t, err)
			require.Len(t, eligible, 1)
			application, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), invoiceTestInput("Identity invoice"), []int{subscription.Id})
			require.NoError(t, err)
			assert.Equal(t, user.Id, application.UserId)
		})
	}
}

func TestCreateInvoiceApplicationStoresMainlandInvoiceDetailsForEligibleUser(t *testing.T) {
	setupInvoiceTest(t)
	user, first := createInvoiceTestSubscription(t, "invoice-personal-user")
	plan := SubscriptionPlan{Title: "Second plan", DurationValue: 1, DurationUnit: SubscriptionDurationMonth, TotalAmount: 800}
	require.NoError(t, DB.Create(&plan).Error)
	second := UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: 800, Status: "active", Source: "order",
		InvoiceEligible: true, PaidAmountMicros: 8_000_000, PaidCurrency: "CNY",
	}
	require.NoError(t, DB.Create(&second).Error)

	input := invoiceTestInput("  上海示例科技有限公司  ")
	input.TaxpayerId = strings.ToLower(input.TaxpayerId)
	application, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), input, []int{first.Id, second.Id})
	require.NoError(t, err)
	assert.Equal(t, "上海示例科技有限公司", application.InvoiceTitle)
	assert.Equal(t, "91310000MA1K123456", application.TaxpayerId)
	assert.Equal(t, input.BankName, application.BankName)
	assert.Equal(t, input.Remark, application.Remark)
	assert.Equal(t, int64(20_000_000), application.TotalAmountMicros)
	assert.Equal(t, "CNY", application.Currency)
	assert.Len(t, application.Items, 2)
	itemTitles := make([]string, 0, len(application.Items))
	for _, item := range application.Items {
		require.NotNil(t, item.ActiveSlot)
		assert.Equal(t, 1, *item.ActiveSlot)
		assert.Equal(t, "CNY", item.Currency)
		assert.Positive(t, item.PaidAmountMicros)
		itemTitles = append(itemTitles, item.PlanTitle)
	}
	assert.ElementsMatch(t, []string{"Invoice plan", "Second plan"}, itemTitles)

	_, err = CreateInvoiceApplication(user.Id, GetInvoiceSettings(), input, []int{first.Id})
	require.ErrorContains(t, err, "already submitted this month")
}

func TestCreateInvoiceApplicationRejectsTotalAmountOverflow(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{Title: "Overflow plan", DurationValue: 1, DurationUnit: SubscriptionDurationMonth, TotalAmount: 1}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "invoice-overflow-user", Password: "password", Identity: UserIdentityUniversity, AffCode: "invoice-overflow-user"}
	require.NoError(t, DB.Create(&user).Error)
	subscriptions := []UserSubscription{
		{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1, Status: "active", Source: "order", InvoiceEligible: true, PaidAmountMicros: math.MaxInt64, PaidCurrency: "USD"},
		{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1, Status: "active", Source: "order", InvoiceEligible: true, PaidAmountMicros: 1, PaidCurrency: "USD"},
	}
	require.NoError(t, DB.Create(&subscriptions).Error)

	_, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), invoiceTestInput("Overflow invoice"), []int{subscriptions[0].Id, subscriptions[1].Id})
	require.ErrorContains(t, err, "subscription amount is invalid")
	var count int64
	require.NoError(t, DB.Model(&InvoiceApplication{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateInvoiceApplicationAllowsPaidSubscriptionWithoutPlanFlag(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{Title: "Non-invoice plan", DurationValue: 1, DurationUnit: SubscriptionDurationMonth, TotalAmount: 1000}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "invoice-ineligible-user", Password: "password", Identity: UserIdentityUniversity, AffCode: "invoice-ineligible-user"}
	require.NoError(t, DB.Create(&user).Error)
	subscription := UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, Status: "active", Source: "order",
		PaidAmountMicros: 10_000_000, PaidCurrency: "USD",
	}
	require.NoError(t, DB.Create(&subscription).Error)

	application, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), invoiceTestInput("Example University"), []int{subscription.Id})
	require.NoError(t, err)
	require.NotNil(t, application)
	require.Len(t, application.Items, 1)
	assert.Equal(t, int64(10_000_000), application.Items[0].PaidAmountMicros)
	assert.Equal(t, "USD", application.Items[0].Currency)
}

func TestCreateInvoiceApplicationRejectsMixedCurrencies(t *testing.T) {
	setupInvoiceTest(t)
	user, first := createInvoiceTestSubscription(t, "invoice-mixed-currency-user")
	second := UserSubscription{
		UserId: user.Id, PlanId: first.PlanId, AmountTotal: 1000, Status: "active", Source: "order",
		InvoiceEligible: true, PaidAmountMicros: 5_000_000, PaidCurrency: "USD",
	}
	require.NoError(t, DB.Create(&second).Error)

	_, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), invoiceTestInput("Mixed currency invoice"), []int{first.Id, second.Id})
	require.ErrorContains(t, err, "same currency")
	var count int64
	require.NoError(t, DB.Model(&InvoiceApplication{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestGetInvoiceEligibleSubscriptionsLimitsResponseAndRequiresPaymentSnapshot(t *testing.T) {
	setupInvoiceTest(t)
	plan := SubscriptionPlan{Title: "Eligible limit plan", DurationValue: 1, DurationUnit: SubscriptionDurationMonth, TotalAmount: 1000}
	require.NoError(t, DB.Create(&plan).Error)
	user := User{Username: "invoice-eligible-limit-user", Password: "password", Identity: UserIdentityUniversity, AffCode: "invoice-eligible-limit-user"}
	require.NoError(t, DB.Create(&user).Error)
	subscriptions := make([]UserSubscription, 0, InvoiceSubscriptionLimit+6)
	for i := 0; i < InvoiceSubscriptionLimit+5; i++ {
		subscriptions = append(subscriptions, UserSubscription{
			UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, Status: "active", Source: "order",
			InvoiceEligible: true, PaidAmountMicros: 1_000_000, PaidCurrency: "CNY",
		})
	}
	// A legacy-looking row with only quota must not be exposed.
	subscriptions = append(subscriptions, UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: 999_999, Status: "active", InvoiceEligible: true,
	})
	require.NoError(t, DB.Create(&subscriptions).Error)

	eligible, err := GetInvoiceEligibleSubscriptions(user.Id, GetInvoiceSettings())
	require.NoError(t, err)
	require.Len(t, eligible, InvoiceSubscriptionLimit)
	for _, subscription := range eligible {
		assert.Positive(t, subscription.PaidAmountMicros)
		assert.NotEmpty(t, subscription.PaidCurrency)
	}
}

func TestListUserInvoiceApplicationsIsPaginated(t *testing.T) {
	setupInvoiceTest(t)
	user := User{Username: "invoice-history-page-user", Password: "password", AffCode: "invoice-history-page-user"}
	require.NoError(t, DB.Create(&user).Error)
	applications := make([]InvoiceApplication, 25)
	for i := range applications {
		applications[i] = InvoiceApplication{
			UserId: user.Id, ApplicationMonth: "2026-08", InvoiceTitle: "Paged invoice",
			Status: InvoiceApplicationStatusCompleted, TotalAmountMicros: int64(i+1) * 1_000_000, Currency: "CNY",
			CreatedAt: int64(i + 1),
		}
	}
	require.NoError(t, DB.Create(&applications).Error)

	page, total, err := ListUserInvoiceApplications(user.Id, 20, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(25), total)
	require.Len(t, page, 5)
	assert.Equal(t, int64(5), page[0].CreatedAt)
	assert.Equal(t, int64(1), page[4].CreatedAt)
}

func TestRejectInvoiceApplicationReleasesSubscriptionsForCorrectedRequest(t *testing.T) {
	setupInvoiceTest(t)
	user, subscription := createInvoiceTestSubscription(t, "invoice-reject-user")
	admin := User{Username: "invoice-reject-admin", Password: "password", Role: common.RoleAdminUser, AffCode: "invoice-reject-admin"}
	require.NoError(t, DB.Create(&admin).Error)
	application, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), invoiceTestInput("Original title"), []int{subscription.Id})
	require.NoError(t, err)
	require.NoError(t, DB.Model(&InvoiceApplication{}).Where("id = ?", application.Id).
		Updates(map[string]interface{}{"pdf_path": "invoices/original.pdf", "pdf_name": "original.pdf"}).Error)

	oldPath, err := RejectInvoiceApplication(application.Id, admin.Id, "  税号不完整，请修正后重新申请。  ")
	require.NoError(t, err)
	assert.Equal(t, "invoices/original.pdf", oldPath)

	var rejected InvoiceApplication
	require.NoError(t, DB.Preload("Items").First(&rejected, application.Id).Error)
	assert.Equal(t, InvoiceApplicationStatusRejected, rejected.Status)
	assert.Equal(t, "税号不完整，请修正后重新申请。", rejected.RejectionReason)
	assert.Equal(t, admin.Id, rejected.RejectedBy)
	assert.NotZero(t, rejected.RejectedAt)
	assert.Empty(t, rejected.PDFPath)
	require.Len(t, rejected.Items, 1)
	assert.Nil(t, rejected.Items[0].ActiveSlot)

	eligible, err := GetInvoiceEligibleSubscriptions(user.Id, GetInvoiceSettings())
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, subscription.Id, eligible[0].Id)
	corrected, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), invoiceTestInput("Corrected title"), []int{subscription.Id})
	require.NoError(t, err)
	assert.Equal(t, InvoiceApplicationStatusPending, corrected.Status)

	_, err = RejectInvoiceApplication(application.Id, admin.Id, "repeat")
	require.ErrorIs(t, err, ErrInvoiceApplicationState)
}

func TestCompleteInvoiceApplicationRequiresPDFAndPendingState(t *testing.T) {
	setupInvoiceTest(t)
	user, subscription := createInvoiceTestSubscription(t, "invoice-complete-user")
	application, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), invoiceTestInput("Complete invoice"), []int{subscription.Id})
	require.NoError(t, err)

	err = CompleteInvoiceApplication(application.Id)
	require.ErrorIs(t, err, ErrInvoicePDFRequired)
	_, err = ReplaceInvoicePDF(application.Id, "invoices/completed.pdf", "completed.pdf")
	require.NoError(t, err)
	require.NoError(t, CompleteInvoiceApplication(application.Id))

	var completed InvoiceApplication
	require.NoError(t, DB.First(&completed, application.Id).Error)
	assert.Equal(t, InvoiceApplicationStatusCompleted, completed.Status)
	assert.NotZero(t, completed.CompletedAt)
	_, err = ClearInvoicePDF(application.Id)
	require.ErrorIs(t, err, ErrInvoiceApplicationState)
	err = CompleteInvoiceApplication(application.Id)
	require.ErrorIs(t, err, ErrInvoiceApplicationState)
}

func TestInvoiceAdminListFiltersAndEscapesLiteralWildcards(t *testing.T) {
	setupInvoiceTest(t)
	user, subscription := createInvoiceTestSubscription(t, "invoice-search-user")
	application, err := CreateInvoiceApplication(user.Id, GetInvoiceSettings(), InvoiceApplicationInput{InvoiceTitle: "Usage is 100% lower_case"}, []int{subscription.Id})
	require.NoError(t, err)

	for _, keyword := range []string{"%", "_", "invoice-search"} {
		items, total, err := ListInvoiceApplications(InvoiceAdminFilter{Keyword: keyword}, 0, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, items, 1)
		assert.Equal(t, application.Id, items[0].Id)
		require.NotNil(t, items[0].User)
		assert.Empty(t, items[0].User.Password)
	}
	_, _, err = ListInvoiceApplications(InvoiceAdminFilter{Status: "invalid"}, 0, 20)
	require.ErrorIs(t, err, ErrInvalidInvoiceFilter)
	_, _, err = ListInvoiceApplications(InvoiceAdminFilter{Keyword: "bad\u202Equery"}, 0, 20)
	require.ErrorIs(t, err, ErrInvalidInvoiceFilter)
}
