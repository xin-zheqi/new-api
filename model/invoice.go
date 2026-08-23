package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	InvoiceApplicationStatusPending   = "pending"
	InvoiceApplicationStatusCompleted = "completed"
	InvoiceApplicationStatusRejected  = "rejected"

	InvoiceTitleMaxLength           = 255
	InvoiceTaxpayerIdMaxLength      = 32
	InvoiceBankNameMaxLength        = 255
	InvoiceRemarkMaxLength          = 1000
	InvoiceRejectionReasonMaxLength = 1000
	InvoiceSubscriptionLimit        = 100
)

var (
	ErrInvoiceApplicationNotFound = errors.New("invoice application not found")
	ErrInvoiceApplicationState    = errors.New("invoice application is no longer pending")
	ErrInvoicePDFRequired         = errors.New("upload an invoice PDF before completing the application")
	ErrInvalidInvoiceFilter       = errors.New("invalid invoice application filter")
	ErrInvoiceIdentityRequired    = errors.New("invoice center is only available for university or enterprise users")
)

var invoiceSettingOptionKeys = []string{
	"InvoiceEnabled",
	"InvoiceApplicationDay",
	"InvoiceLookbackDays",
	"InvoiceMonthlyLimit",
	"InvoiceSystemRechargeEnabled",
	"InvoiceRedemptionRechargeEnabled",
	"InvoiceSystemSubscriptionEnabled",
	"InvoiceRedemptionSubscriptionEnabled",
}

type InvoiceRequestError struct {
	message string
}

func (e *InvoiceRequestError) Error() string {
	return e.message
}

func IsInvoiceRequestError(err error) bool {
	var requestError *InvoiceRequestError
	return errors.As(err, &requestError)
}

func invoiceRequestError(message string) error {
	return &InvoiceRequestError{message: message}
}

func invoiceRequestErrorf(format string, arguments ...interface{}) error {
	return &InvoiceRequestError{message: fmt.Sprintf(format, arguments...)}
}

type InvoiceApplication struct {
	Id               int    `json:"id"`
	UserId           int    `json:"user_id" gorm:"index;index:idx_user_invoice_month,priority:1"`
	ApplicationMonth string `json:"application_month" gorm:"type:varchar(7);index:idx_user_invoice_month,priority:2"`
	InvoiceTitle     string `json:"invoice_title" gorm:"type:varchar(255);not null"`
	TaxpayerId       string `json:"taxpayer_id" gorm:"type:varchar(32)"`
	BankName         string `json:"bank_name" gorm:"type:varchar(255)"`
	Remark           string `json:"remark" gorm:"type:text"`
	// LegacyTotalAmount contains the pre-payment-snapshot quota value. It is
	// retained only for schema compatibility and is never exposed as money.
	LegacyTotalAmount int64                    `json:"-" gorm:"column:total_amount;type:bigint;not null;default:0"`
	TotalAmountMicros int64                    `json:"total_amount_micros" gorm:"type:bigint;not null;default:0"`
	Currency          string                   `json:"currency" gorm:"type:varchar(8);not null;default:''"`
	Status            string                   `json:"status" gorm:"type:varchar(32);not null;default:'pending';index;index:idx_invoice_status_created,priority:1"`
	PDFPath           string                   `json:"-" gorm:"type:text"`
	PDFName           string                   `json:"pdf_name,omitempty" gorm:"type:varchar(255)"`
	RejectionReason   string                   `json:"rejection_reason,omitempty" gorm:"type:text"`
	RejectedAt        int64                    `json:"rejected_at" gorm:"bigint"`
	RejectedBy        int                      `json:"rejected_by" gorm:"index"`
	CreatedAt         int64                    `json:"created_at" gorm:"bigint;autoCreateTime;index:idx_invoice_status_created,priority:2"`
	CompletedAt       int64                    `json:"completed_at" gorm:"bigint"`
	UpdatedAt         int64                    `json:"updated_at" gorm:"bigint;autoUpdateTime"`
	User              *User                    `json:"-" gorm:"foreignKey:UserId;-:migration"`
	Items             []InvoiceApplicationItem `json:"items,omitempty" gorm:"foreignKey:InvoiceApplicationId"`
}

type InvoiceApplicationItem struct {
	Id                   int    `json:"id"`
	InvoiceApplicationId int    `json:"invoice_application_id" gorm:"index"`
	UserSubscriptionId   int    `json:"user_subscription_id"`
	TopUpId              int    `json:"top_up_id,omitempty" gorm:"index"`
	RedemptionId         int    `json:"redemption_id,omitempty" gorm:"index"`
	ItemType             string `json:"item_type" gorm:"type:varchar(16);not null;default:'subscription'"`
	ActiveSlot           *int   `json:"-"`
	PlanTitle            string `json:"plan_title" gorm:"type:varchar(255)"`
	LegacyAmountTotal    int64  `json:"-" gorm:"column:amount_total;type:bigint;not null;default:0"`
	PaidAmountMicros     int64  `json:"paid_amount_micros" gorm:"type:bigint;not null;default:0"`
	Currency             string `json:"currency" gorm:"type:varchar(8);not null;default:''"`
	StartTime            int64  `json:"start_time" gorm:"bigint"`
	EndTime              int64  `json:"end_time" gorm:"bigint"`
}

// invoiceApplicationItemIndex is used only after legacy invoice rows have
// been normalized. Keeping the unique index off the runtime model prevents
// AutoMigrate from attempting to build it while duplicate historical rows
// still exist.
type invoiceApplicationItemIndex struct {
	UserSubscriptionId int  `gorm:"column:user_subscription_id;uniqueIndex:idx_invoice_subscription_active,priority:1"`
	ActiveSlot         *int `gorm:"column:active_slot;uniqueIndex:idx_invoice_subscription_active,priority:2"`
}

func (invoiceApplicationItemIndex) TableName() string {
	return "invoice_application_items"
}

type InvoiceEligibleSubscription struct {
	UserSubscription
	PlanTitle    string `json:"plan_title"`
	Source       string `json:"source,omitempty"`
	TopUpId      int    `json:"top_up_id,omitempty"`
	RedemptionId int    `json:"redemption_id,omitempty"`
	ItemType     string `json:"item_type,omitempty"`
}

type InvoiceApplicationInput struct {
	InvoiceTitle string
	TaxpayerId   string
	BankName     string
	Remark       string
}

type InvoiceAdminFilter struct {
	Status  string
	Keyword string
	UserId  int
}

type InvoiceSettings struct {
	Enabled                       bool
	ApplicationDay                int
	LookbackDays                  int
	MonthlyLimit                  int
	SystemRechargeEnabled         bool
	RedemptionRechargeEnabled     bool
	SystemSubscriptionEnabled     bool
	RedemptionSubscriptionEnabled bool
}

func parseInvoiceSettingInt(values map[string]string, key string, minimum, maximum, fallback int, rejectInvalid bool) (int, error) {
	value, exists := values[key]
	if !exists {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil && parsed >= minimum && parsed <= maximum {
		return parsed, nil
	}
	if rejectInvalid {
		return 0, fmt.Errorf("invalid %s setting", key)
	}
	return fallback, nil
}

func invoiceSettingsFromValues(values map[string]string, rejectInvalid bool) (InvoiceSettings, error) {
	settings := InvoiceSettings{Enabled: true, ApplicationDay: 25, LookbackDays: 90, MonthlyLimit: 1,
		SystemRechargeEnabled: true, RedemptionRechargeEnabled: true,
		SystemSubscriptionEnabled: true, RedemptionSubscriptionEnabled: true}
	if value, exists := values["InvoiceEnabled"]; exists {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true":
			settings.Enabled = true
		case "false":
			settings.Enabled = false
		default:
			if rejectInvalid {
				return InvoiceSettings{}, errors.New("invalid InvoiceEnabled setting")
			}
		}
	}
	var err error
	settings.ApplicationDay, err = parseInvoiceSettingInt(values, "InvoiceApplicationDay", 1, 28, settings.ApplicationDay, rejectInvalid)
	if err != nil {
		return InvoiceSettings{}, err
	}
	settings.LookbackDays, err = parseInvoiceSettingInt(values, "InvoiceLookbackDays", 1, 3650, settings.LookbackDays, rejectInvalid)
	if err != nil {
		return InvoiceSettings{}, err
	}
	settings.MonthlyLimit, err = parseInvoiceSettingInt(values, "InvoiceMonthlyLimit", 1, 31, settings.MonthlyLimit, rejectInvalid)
	if err != nil {
		return InvoiceSettings{}, err
	}
	boolSettings := []struct {
		key string
		out *bool
	}{
		{"InvoiceSystemRechargeEnabled", &settings.SystemRechargeEnabled},
		{"InvoiceRedemptionRechargeEnabled", &settings.RedemptionRechargeEnabled},
		{"InvoiceSystemSubscriptionEnabled", &settings.SystemSubscriptionEnabled},
		{"InvoiceRedemptionSubscriptionEnabled", &settings.RedemptionSubscriptionEnabled},
	}
	for _, item := range boolSettings {
		if value, exists := values[item.key]; exists {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "true":
				*item.out = true
			case "false":
				*item.out = false
			default:
				if rejectInvalid {
					return InvoiceSettings{}, fmt.Errorf("invalid %s setting", item.key)
				}
			}
		}
	}
	return settings, nil
}

// GetInvoiceSettings reads the local option snapshot. Request handlers must use
// LoadInvoiceSettings so a stale node cannot keep accepting applications.
func GetInvoiceSettings() InvoiceSettings {
	values := make(map[string]string, len(invoiceSettingOptionKeys))
	common.OptionMapRWMutex.RLock()
	for _, key := range invoiceSettingOptionKeys {
		if value, exists := common.OptionMap[key]; exists {
			values[key] = value
		}
	}
	common.OptionMapRWMutex.RUnlock()
	settings, _ := invoiceSettingsFromValues(values, false)
	return settings
}

// LoadInvoiceSettings reads the shared database on every invoice request. This
// prevents the periodic in-memory option sync from creating a cross-node window
// where a disabled or tightened invoice policy is still accepted.
func LoadInvoiceSettings() (InvoiceSettings, error) {
	if DB == nil {
		return InvoiceSettings{}, errors.New("invoice settings database is unavailable")
	}
	var options []Option
	if err := DB.Select("key", "value").Where("key IN ?", invoiceSettingOptionKeys).Find(&options).Error; err != nil {
		return InvoiceSettings{}, fmt.Errorf("load invoice settings: %w", err)
	}
	values := make(map[string]string, len(options))
	for _, option := range options {
		values[option.Key] = option.Value
	}
	settings, err := invoiceSettingsFromValues(values, true)
	if err != nil {
		return InvoiceSettings{}, fmt.Errorf("load invoice settings: %w", err)
	}
	return settings, nil
}

func normalizeInvoiceText(value, field string, maxLength int, multiline, required bool) (string, error) {
	if !utf8.ValidString(value) {
		return "", invoiceRequestErrorf("%s is not valid UTF-8", field)
	}
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n"))
	if value == "" {
		if required {
			return "", invoiceRequestErrorf("%s is required", field)
		}
		return "", nil
	}
	if utf8.RuneCountInString(value) > maxLength {
		return "", invoiceRequestErrorf("%s must not exceed %d characters", field, maxLength)
	}
	for _, r := range value {
		if r == '<' || r == '>' || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return "", invoiceRequestErrorf("%s contains unsupported characters", field)
		}
		if unicode.IsControl(r) && !(multiline && r == '\n') {
			return "", invoiceRequestErrorf("%s contains unsupported characters", field)
		}
		if !multiline && r == '\n' {
			return "", invoiceRequestErrorf("%s must be a single line", field)
		}
	}
	return value, nil
}

func normalizeInvoiceInput(input InvoiceApplicationInput) (InvoiceApplicationInput, error) {
	var err error
	input.InvoiceTitle, err = normalizeInvoiceText(input.InvoiceTitle, "invoice title", InvoiceTitleMaxLength, false, true)
	if err != nil {
		return InvoiceApplicationInput{}, err
	}
	// Keep model-level compatibility for historical/internal callers; the HTTP
	// application endpoint enforces taxpayer ID as a required field.
	input.TaxpayerId, err = normalizeInvoiceText(input.TaxpayerId, "taxpayer id", InvoiceTaxpayerIdMaxLength, false, false)
	if err != nil {
		return InvoiceApplicationInput{}, err
	}
	input.TaxpayerId = strings.ToUpper(input.TaxpayerId)
	for _, r := range input.TaxpayerId {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return InvoiceApplicationInput{}, invoiceRequestError("taxpayer id must contain only letters and digits")
		}
	}
	input.BankName, err = normalizeInvoiceText(input.BankName, "bank name", InvoiceBankNameMaxLength, false, false)
	if err != nil {
		return InvoiceApplicationInput{}, err
	}
	input.Remark, err = normalizeInvoiceText(input.Remark, "invoice remark", InvoiceRemarkMaxLength, true, false)
	if err != nil {
		return InvoiceApplicationInput{}, err
	}
	return input, nil
}

func GetInvoiceEligibleSubscriptions(userId int, settings InvoiceSettings) ([]InvoiceEligibleSubscription, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	var user User
	if err := DB.Select("id", "identity").Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}
	if !IsInvoiceEligibleIdentity(user.Identity) {
		return nil, ErrInvoiceIdentityRequired
	}
	cutoff := common.GetTimestamp() - int64(settings.LookbackDays)*24*60*60
	var subscriptions []InvoiceEligibleSubscription
	query := DB.Table("user_subscriptions AS us").
		Select("us.*, COALESCE(NULLIF(us.plan_title_snapshot, ''), sp.title, '') AS plan_title").
		Joins("LEFT JOIN subscription_plans AS sp ON sp.id = us.plan_id").
		Where("us.user_id = ? AND us.paid_amount_micros > 0 AND us.paid_currency <> ? AND us.created_at >= ?", userId, "", cutoff).
		Where("NOT EXISTS (SELECT 1 FROM invoice_application_items iai WHERE iai.user_subscription_id = us.id AND iai.active_slot = ?)", 1)
	if settings.SystemSubscriptionEnabled && settings.RedemptionSubscriptionEnabled {
		query = query.Where("us.source IN ?", []string{"order", "balance", "redemption"})
	} else if settings.SystemSubscriptionEnabled {
		query = query.Where("us.source IN ?", []string{"order", "balance"})
	} else if settings.RedemptionSubscriptionEnabled {
		query = query.Where("us.source = ?", "redemption")
	} else {
		query = query.Where("1 = 0")
	}
	err := query.Order("us.created_at DESC, us.id DESC").Limit(InvoiceSubscriptionLimit).Find(&subscriptions).Error
	if err != nil {
		return nil, err
	}
	for i := range subscriptions {
		subscriptions[i].ItemType = "subscription"
	}
	var topUps []TopUp
	if settings.SystemRechargeEnabled {
		if err := DB.Table("top_ups").Where("user_id = ? AND status = ? AND source = ? AND complete_time >= ?", userId, common.TopUpStatusSuccess, TopUpSourceRecharge, cutoff).
			Where("expected_amount_micros > 0 AND expected_currency <> ''").
			Where("NOT EXISTS (SELECT 1 FROM invoice_application_items iai JOIN invoice_applications ia ON ia.id = iai.invoice_application_id WHERE iai.top_up_id = top_ups.id AND ia.status IN ?)", []string{InvoiceApplicationStatusPending, InvoiceApplicationStatusCompleted}).
			Order("complete_time DESC, id DESC").Limit(InvoiceSubscriptionLimit).Find(&topUps).Error; err != nil {
			return nil, err
		}
		for _, topUp := range topUps {
			amount, currency, valid := invoiceTopUpPaymentSnapshot(topUp)
			if !valid {
				continue
			}
			topUpsub := InvoiceEligibleSubscription{
				UserSubscription: UserSubscription{Id: -topUp.Id, UserId: userId, PaidAmountMicros: amount, PaidCurrency: currency, CreatedAt: topUp.CompleteTime},
				PlanTitle:        "Balance recharge", Source: TopUpSourceRecharge, TopUpId: topUp.Id, ItemType: "top_up",
			}
			subscriptions = append(subscriptions, topUpsub)
		}
	}
	if settings.RedemptionRechargeEnabled {
		var redemptions []Redemption
		if err := DB.Table("redemptions").Where("used_user_id = ? AND status = ? AND redeem_type = ? AND redeemed_time >= ? AND quota > ? AND invoice_amount_micros > 0 AND invoice_currency <> ''", userId, common.RedemptionCodeStatusUsed, RedemptionTypeQuota, cutoff, 0).
			Where("NOT EXISTS (SELECT 1 FROM invoice_application_items iai JOIN invoice_applications ia ON ia.id = iai.invoice_application_id WHERE iai.redemption_id = redemptions.id AND ia.status IN ?)", []string{InvoiceApplicationStatusPending, InvoiceApplicationStatusCompleted}).
			Order("redeemed_time DESC, id DESC").Limit(InvoiceSubscriptionLimit).Find(&redemptions).Error; err != nil {
			return nil, err
		}
		for _, redemption := range redemptions {
			if redemption.InvoiceAmountMicros <= 0 || strings.TrimSpace(redemption.InvoiceCurrency) == "" {
				continue
			}
			subscriptions = append(subscriptions, InvoiceEligibleSubscription{
				UserSubscription: UserSubscription{Id: -1000000000 - redemption.Id, UserId: userId, PaidAmountMicros: redemption.InvoiceAmountMicros, PaidCurrency: redemption.InvoiceCurrency, CreatedAt: redemption.RedeemedTime},
				PlanTitle:        "Redemption code balance recharge", Source: "redemption_recharge", RedemptionId: redemption.Id, ItemType: "redemption_recharge",
			})
		}
	}
	sort.SliceStable(subscriptions, func(i, j int) bool {
		if subscriptions[i].CreatedAt != subscriptions[j].CreatedAt {
			return subscriptions[i].CreatedAt > subscriptions[j].CreatedAt
		}
		return subscriptions[i].Id > subscriptions[j].Id
	})
	if len(subscriptions) > InvoiceSubscriptionLimit {
		subscriptions = subscriptions[:InvoiceSubscriptionLimit]
	}
	return subscriptions, nil
}

func invoiceTopUpPaymentSnapshot(topUp TopUp) (int64, string, bool) {
	if topUp.ExpectedAmountMicros <= 0 || strings.TrimSpace(topUp.ExpectedCurrency) == "" {
		return 0, "", false
	}
	currency, err := normalizeSubscriptionPaymentCurrency(topUp.ExpectedCurrency)
	if err != nil || currency != topUp.ExpectedCurrency {
		return 0, "", false
	}
	return topUp.ExpectedAmountMicros, currency, true
}

func CreateInvoiceApplication(userId int, settings InvoiceSettings, input InvoiceApplicationInput, subscriptionIds []int, redemptionIds ...[]int) (*InvoiceApplication, error) {
	requestedRedemptionCount := 0
	if len(redemptionIds) > 0 {
		requestedRedemptionCount = len(redemptionIds[0])
	}
	if userId <= 0 || len(subscriptionIds)+requestedRedemptionCount == 0 {
		return nil, invoiceRequestError("invoice title and subscriptions are required")
	}
	input, err := normalizeInvoiceInput(input)
	if err != nil {
		return nil, err
	}
	if len(subscriptionIds)+requestedRedemptionCount > InvoiceSubscriptionLimit {
		return nil, invoiceRequestError("too many subscriptions in one invoice application")
	}
	seenSubscriptionIds := make(map[int]struct{}, len(subscriptionIds))
	for _, subscriptionId := range subscriptionIds {
		if subscriptionId == 0 {
			return nil, invoiceRequestError("invalid subscription id")
		}
		if _, exists := seenSubscriptionIds[subscriptionId]; exists {
			return nil, invoiceRequestError("duplicate subscription id")
		}
		seenSubscriptionIds[subscriptionId] = struct{}{}
	}
	seenRedemptionIds := make(map[int]struct{}, requestedRedemptionCount)
	if len(redemptionIds) > 0 {
		for _, redemptionId := range redemptionIds[0] {
			if redemptionId <= 0 {
				return nil, invoiceRequestError("invalid redemption id")
			}
			if _, exists := seenRedemptionIds[redemptionId]; exists {
				return nil, invoiceRequestError("duplicate redemption id")
			}
			seenRedemptionIds[redemptionId] = struct{}{}
		}
	}

	var application *InvoiceApplication
	err = DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var user User
		if err := lockForUpdate(tx).Select("id", "identity").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		if !IsInvoiceEligibleIdentity(user.Identity) {
			return ErrInvoiceIdentityRequired
		}
		var monthlyCount int64
		if err := tx.Model(&InvoiceApplication{}).
			Where("user_id = ? AND application_month = ? AND status IN ?", userId, now.Format("2006-01"), []string{InvoiceApplicationStatusPending, InvoiceApplicationStatusCompleted}).
			Count(&monthlyCount).Error; err != nil {
			return err
		}
		if monthlyCount >= int64(settings.MonthlyLimit) {
			return invoiceRequestError("invoice application already submitted this month")
		}

		var subscriptions []InvoiceEligibleSubscription
		var subscriptionIDs, topUpIDs, redemptionIDsInput []int
		for _, itemID := range subscriptionIds {
			if itemID > 0 {
				subscriptionIDs = append(subscriptionIDs, itemID)
			} else {
				topUpIDs = append(topUpIDs, -itemID)
			}
		}
		if len(redemptionIds) > 0 {
			redemptionIDsInput = append(redemptionIDsInput, redemptionIds[0]...)
		}
		if len(subscriptionIDs) > 0 {
			subscriptionQuery := tx.Table("user_subscriptions AS us").Select("us.*, COALESCE(NULLIF(us.plan_title_snapshot, ''), sp.title, '') AS plan_title").
				Joins("LEFT JOIN subscription_plans AS sp ON sp.id = us.plan_id").
				Where("us.user_id = ? AND us.id IN ? AND us.paid_amount_micros > 0 AND us.paid_currency <> ? AND us.created_at >= ?", userId, subscriptionIDs, "", common.GetTimestamp()-int64(settings.LookbackDays)*24*60*60).
				Where("NOT EXISTS (SELECT 1 FROM invoice_application_items iai WHERE iai.user_subscription_id = us.id AND iai.active_slot = ?)", 1)
			if settings.SystemSubscriptionEnabled && settings.RedemptionSubscriptionEnabled {
				subscriptionQuery = subscriptionQuery.Where("us.source IN ?", []string{"order", "balance", "redemption"})
			} else if settings.SystemSubscriptionEnabled {
				subscriptionQuery = subscriptionQuery.Where("us.source IN ?", []string{"order", "balance"})
			} else if settings.RedemptionSubscriptionEnabled {
				subscriptionQuery = subscriptionQuery.Where("us.source = ?", "redemption")
			} else {
				subscriptionQuery = subscriptionQuery.Where("1 = 0")
			}
			if err := subscriptionQuery.Find(&subscriptions).Error; err != nil {
				return err
			}
		}
		for i := range subscriptions {
			subscriptions[i].ItemType = "subscription"
		}
		if len(topUpIDs) > 0 {
			if !settings.SystemRechargeEnabled {
				return invoiceRequestError("one or more invoice items are not eligible for invoicing")
			}
			var topUps []TopUp
			if err := tx.Table("top_ups").Where("user_id = ? AND id IN ? AND status = ? AND source = ? AND complete_time >= ?", userId, topUpIDs, common.TopUpStatusSuccess, TopUpSourceRecharge, common.GetTimestamp()-int64(settings.LookbackDays)*24*60*60).
				Where("expected_amount_micros > 0 AND expected_currency <> ''").
				Where("NOT EXISTS (SELECT 1 FROM invoice_application_items iai JOIN invoice_applications ia ON ia.id = iai.invoice_application_id WHERE iai.top_up_id = top_ups.id AND ia.status IN ?)", []string{InvoiceApplicationStatusPending, InvoiceApplicationStatusCompleted}).Find(&topUps).Error; err != nil {
				return err
			}
			for _, topUp := range topUps {
				amount, currency, valid := invoiceTopUpPaymentSnapshot(topUp)
				if !valid {
					continue
				}
				subscriptions = append(subscriptions, InvoiceEligibleSubscription{UserSubscription: UserSubscription{Id: -topUp.Id, UserId: userId, PaidAmountMicros: amount, PaidCurrency: currency, CreatedAt: topUp.CompleteTime}, PlanTitle: "Balance recharge", Source: TopUpSourceRecharge, TopUpId: topUp.Id, ItemType: "top_up"})
			}
		}
		if len(redemptionIDsInput) > 0 {
			if !settings.RedemptionRechargeEnabled {
				return invoiceRequestError("one or more invoice items are not eligible for invoicing")
			}
			var redemptions []Redemption
			if err := tx.Table("redemptions").Where("used_user_id = ? AND id IN ? AND status = ? AND redeem_type = ? AND redeemed_time >= ? AND quota > ? AND invoice_amount_micros > 0 AND invoice_currency <> ''", userId, redemptionIDsInput, common.RedemptionCodeStatusUsed, RedemptionTypeQuota, common.GetTimestamp()-int64(settings.LookbackDays)*24*60*60, 0).
				Where("NOT EXISTS (SELECT 1 FROM invoice_application_items iai JOIN invoice_applications ia ON ia.id = iai.invoice_application_id WHERE iai.redemption_id = redemptions.id AND ia.status IN ?)", []string{InvoiceApplicationStatusPending, InvoiceApplicationStatusCompleted}).Find(&redemptions).Error; err != nil {
				return err
			}
			for _, redemption := range redemptions {
				if redemption.InvoiceAmountMicros <= 0 || strings.TrimSpace(redemption.InvoiceCurrency) == "" {
					continue
				}
				subscriptions = append(subscriptions, InvoiceEligibleSubscription{UserSubscription: UserSubscription{Id: -1000000000 - redemption.Id, UserId: userId, PaidAmountMicros: redemption.InvoiceAmountMicros, PaidCurrency: redemption.InvoiceCurrency, CreatedAt: redemption.RedeemedTime}, PlanTitle: "Redemption code balance recharge", Source: "redemption_recharge", RedemptionId: redemption.Id, ItemType: "redemption_recharge"})
			}
		}
		if len(subscriptions) != len(subscriptionIds)+len(redemptionIDsInput) {
			return invoiceRequestError("one or more invoice items are not eligible for invoicing")
		}

		application = &InvoiceApplication{
			UserId: userId, ApplicationMonth: now.Format("2006-01"),
			InvoiceTitle: input.InvoiceTitle, TaxpayerId: input.TaxpayerId,
			BankName: input.BankName, Remark: input.Remark,
			Status: InvoiceApplicationStatusPending,
		}
		for _, subscription := range subscriptions {
			if subscription.PaidAmountMicros <= 0 {
				return invoiceRequestError("subscription amount is invalid")
			}
			currency, err := normalizeSubscriptionPaymentCurrency(subscription.PaidCurrency)
			if err != nil || currency != subscription.PaidCurrency {
				return invoiceRequestError("subscription currency is invalid")
			}
			if application.Currency == "" {
				application.Currency = currency
			} else if application.Currency != currency {
				return invoiceRequestError("subscriptions in one invoice application must use the same currency")
			}
			if subscription.PaidAmountMicros > math.MaxInt64-application.TotalAmountMicros {
				return invoiceRequestError("subscription amount is invalid")
			}
			activeSlot := 1
			userSubscriptionID := subscription.Id
			var activeSlotPointer = &activeSlot
			if subscription.ItemType == "top_up" || subscription.ItemType == "redemption_recharge" {
				userSubscriptionID = 0
				activeSlotPointer = nil
			}
			application.TotalAmountMicros += subscription.PaidAmountMicros
			application.Items = append(application.Items, InvoiceApplicationItem{
				UserSubscriptionId: userSubscriptionID, TopUpId: subscription.TopUpId, RedemptionId: subscription.RedemptionId,
				ItemType: subscription.ItemType, ActiveSlot: activeSlotPointer,
				PlanTitle: subscription.PlanTitle, PaidAmountMicros: subscription.PaidAmountMicros,
				Currency:  currency,
				StartTime: subscription.StartTime, EndTime: subscription.EndTime,
			})
		}
		return tx.Create(application).Error
	})
	return application, err
}

func ListUserInvoiceApplications(userId, offset, limit int) ([]InvoiceApplication, int64, error) {
	if userId <= 0 || offset < 0 || limit < 1 || limit > 100 {
		return nil, 0, ErrInvalidInvoiceFilter
	}
	query := DB.Model(&InvoiceApplication{}).Where("user_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var applications []InvoiceApplication
	err := query.Preload("Items").Order("created_at DESC, id DESC").
		Offset(offset).Limit(limit).Find(&applications).Error
	return applications, total, err
}

func CountActiveUserInvoiceApplicationsInMonth(userId int, month string) (int64, error) {
	if userId <= 0 || len(month) != 7 {
		return 0, ErrInvalidInvoiceFilter
	}
	var count int64
	err := DB.Model(&InvoiceApplication{}).
		Where("user_id = ? AND application_month = ? AND status IN ?", userId, month, []string{InvoiceApplicationStatusPending, InvoiceApplicationStatusCompleted}).
		Count(&count).Error
	return count, err
}

func escapeInvoiceLike(value string) string {
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	value = strings.ReplaceAll(value, "_", "!_")
	return "%" + strings.ToLower(value) + "%"
}

func invoiceApplicationQuery(db *gorm.DB) *gorm.DB {
	return db.Preload("User", func(query *gorm.DB) *gorm.DB {
		return query.Unscoped().Select("id", "username", "display_name", "email", "identity")
	}).Preload("Items")
}

func ListInvoiceApplications(filter InvoiceAdminFilter, offset, limit int) ([]InvoiceApplication, int64, error) {
	if offset < 0 || limit < 1 || limit > 100 || filter.UserId < 0 {
		return nil, 0, ErrInvalidInvoiceFilter
	}
	if filter.Status != "" && filter.Status != InvoiceApplicationStatusPending && filter.Status != InvoiceApplicationStatusCompleted && filter.Status != InvoiceApplicationStatusRejected {
		return nil, 0, ErrInvalidInvoiceFilter
	}
	keyword, err := normalizeInvoiceText(filter.Keyword, "invoice search", 100, false, false)
	if err != nil {
		return nil, 0, ErrInvalidInvoiceFilter
	}

	query := DB.Model(&InvoiceApplication{}).
		Joins("LEFT JOIN users u ON u.id = invoice_applications.user_id")
	if filter.Status != "" {
		query = query.Where("invoice_applications.status = ?", filter.Status)
	}
	if filter.UserId > 0 {
		query = query.Where("invoice_applications.user_id = ?", filter.UserId)
	}
	if keyword != "" {
		pattern := escapeInvoiceLike(keyword)
		condition := "(LOWER(invoice_applications.invoice_title) LIKE ? ESCAPE '!' OR LOWER(invoice_applications.taxpayer_id) LIKE ? ESCAPE '!' OR LOWER(invoice_applications.bank_name) LIKE ? ESCAPE '!' OR LOWER(u.username) LIKE ? ESCAPE '!' OR LOWER(u.display_name) LIKE ? ESCAPE '!' OR LOWER(u.email) LIKE ? ESCAPE '!')"
		arguments := []interface{}{pattern, pattern, pattern, pattern, pattern, pattern}
		if id, parseErr := strconv.Atoi(keyword); parseErr == nil && id > 0 {
			condition = "(invoice_applications.id = ? OR " + strings.TrimPrefix(condition, "(")
			arguments = append([]interface{}{id}, arguments...)
		}
		query = query.Where(condition, arguments...)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var applications []InvoiceApplication
	if err := invoiceApplicationQuery(query.Select("invoice_applications.*")).
		Order("invoice_applications.created_at DESC, invoice_applications.id DESC").
		Offset(offset).Limit(limit).Find(&applications).Error; err != nil {
		return nil, 0, err
	}
	return applications, total, nil
}

func GetInvoiceApplication(id int) (*InvoiceApplication, error) {
	if id <= 0 {
		return nil, ErrInvoiceApplicationNotFound
	}
	var application InvoiceApplication
	if err := invoiceApplicationQuery(DB).First(&application, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceApplicationNotFound
		}
		return nil, err
	}
	return &application, nil
}

func GetInvoicePDF(id, userId int) (*InvoiceApplication, error) {
	if id <= 0 || userId < 0 {
		return nil, ErrInvoiceApplicationNotFound
	}
	var application InvoiceApplication
	query := DB.Select("id", "user_id", "status", "pdf_path", "pdf_name").Where("id = ?", id)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if err := query.First(&application).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceApplicationNotFound
		}
		return nil, err
	}
	return &application, nil
}

func ReplaceInvoicePDF(id int, path, name string) (string, error) {
	if id <= 0 || path == "" || name == "" {
		return "", ErrInvoiceApplicationNotFound
	}
	var oldPath string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var application InvoiceApplication
		if err := lockForUpdate(tx).Select("id", "status", "pdf_path").Where("id = ?", id).First(&application).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceApplicationNotFound
			}
			return err
		}
		if application.Status != InvoiceApplicationStatusPending {
			return ErrInvoiceApplicationState
		}
		oldPath = application.PDFPath
		result := tx.Model(&InvoiceApplication{}).Where("id = ? AND status = ?", id, InvoiceApplicationStatusPending).
			Updates(map[string]interface{}{"pdf_path": path, "pdf_name": name, "updated_at": common.GetTimestamp()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceApplicationState
		}
		return nil
	})
	return oldPath, err
}

func ClearInvoicePDF(id int) (string, error) {
	var oldPath string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var application InvoiceApplication
		if err := lockForUpdate(tx).Select("id", "status", "pdf_path").Where("id = ?", id).First(&application).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceApplicationNotFound
			}
			return err
		}
		if application.Status != InvoiceApplicationStatusPending {
			return ErrInvoiceApplicationState
		}
		oldPath = application.PDFPath
		if oldPath == "" {
			return nil
		}
		result := tx.Model(&InvoiceApplication{}).Where("id = ? AND status = ?", id, InvoiceApplicationStatusPending).
			Updates(map[string]interface{}{"pdf_path": "", "pdf_name": "", "updated_at": common.GetTimestamp()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceApplicationState
		}
		return nil
	})
	return oldPath, err
}

func CompleteInvoiceApplication(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var application InvoiceApplication
		if err := lockForUpdate(tx).Select("id", "status", "pdf_path").Where("id = ?", id).First(&application).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceApplicationNotFound
			}
			return err
		}
		if application.Status != InvoiceApplicationStatusPending {
			return ErrInvoiceApplicationState
		}
		if application.PDFPath == "" {
			return ErrInvoicePDFRequired
		}
		now := common.GetTimestamp()
		result := tx.Model(&InvoiceApplication{}).Where("id = ? AND status = ?", id, InvoiceApplicationStatusPending).
			Updates(map[string]interface{}{"status": InvoiceApplicationStatusCompleted, "completed_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceApplicationState
		}
		return nil
	})
}

func RejectInvoiceApplication(id, adminId int, reason string) (string, error) {
	if id <= 0 || adminId <= 0 {
		return "", ErrInvoiceApplicationNotFound
	}
	reason, err := normalizeInvoiceText(reason, "rejection reason", InvoiceRejectionReasonMaxLength, true, true)
	if err != nil {
		return "", err
	}
	var oldPath string
	err = DB.Transaction(func(tx *gorm.DB) error {
		var owner InvoiceApplication
		if err := tx.Select("user_id").Where("id = ?", id).First(&owner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceApplicationNotFound
			}
			return err
		}
		var user User
		if err := lockForUpdate(tx.Unscoped()).Select("id").Where("id = ?", owner.UserId).First(&user).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var application InvoiceApplication
		if err := lockForUpdate(tx).Select("id", "status", "pdf_path").Where("id = ?", id).First(&application).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceApplicationNotFound
			}
			return err
		}
		if application.Status != InvoiceApplicationStatusPending {
			return ErrInvoiceApplicationState
		}
		oldPath = application.PDFPath
		now := common.GetTimestamp()
		result := tx.Model(&InvoiceApplication{}).Where("id = ? AND status = ?", id, InvoiceApplicationStatusPending).
			Updates(map[string]interface{}{
				"status": InvoiceApplicationStatusRejected, "rejection_reason": reason,
				"rejected_at": now, "rejected_by": adminId,
				"pdf_path": "", "pdf_name": "", "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceApplicationState
		}
		return tx.Model(&InvoiceApplicationItem{}).Where("invoice_application_id = ? AND active_slot = ?", id, 1).
			Update("active_slot", nil).Error
	})
	return oldPath, err
}
