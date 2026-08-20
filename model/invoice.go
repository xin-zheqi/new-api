package model

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func invoiceOptionInt(key string, fallback int) int {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

const (
	InvoiceApplicationStatusPending   = "pending"
	InvoiceApplicationStatusCompleted = "completed"
	InvoiceTitleMaxLength             = 255
	InvoiceSubscriptionLimit          = 100
)

type InvoiceApplication struct {
	Id               int                      `json:"id"`
	UserId           int                      `json:"user_id" gorm:"index;index:idx_user_invoice_month,priority:1"`
	ApplicationMonth string                   `json:"application_month" gorm:"type:varchar(7);index:idx_user_invoice_month,priority:2"`
	InvoiceTitle     string                   `json:"invoice_title" gorm:"type:varchar(255);not null"`
	TaxpayerId       string                   `json:"taxpayer_id" gorm:"type:varchar(32);not null;default:''"`
	TotalAmount      int64                    `json:"total_amount" gorm:"type:bigint;not null;default:0"`
	Status           string                   `json:"status" gorm:"type:varchar(32);not null;default:'pending';index"`
	PDFPath          string                   `json:"-" gorm:"type:text"`
	PDFName          string                   `json:"pdf_name,omitempty" gorm:"type:varchar(255)"`
	CreatedAt        int64                    `json:"created_at" gorm:"bigint;autoCreateTime"`
	CompletedAt      int64                    `json:"completed_at" gorm:"bigint;default:0"`
	UpdatedAt        int64                    `json:"updated_at" gorm:"bigint;autoUpdateTime"`
	User             *User                    `json:"-" gorm:"foreignKey:UserId"`
	Items            []InvoiceApplicationItem `json:"items,omitempty" gorm:"foreignKey:InvoiceApplicationId"`
}

type InvoiceApplicationItem struct {
	Id                   int    `json:"id"`
	InvoiceApplicationId int    `json:"invoice_application_id" gorm:"index"`
	UserSubscriptionId   int    `json:"user_subscription_id" gorm:"uniqueIndex:idx_invoice_subscription"`
	PlanTitle            string `json:"plan_title" gorm:"type:varchar(255)"`
	AmountTotal          int64  `json:"amount_total" gorm:"type:bigint;not null;default:0"`
	StartTime            int64  `json:"start_time" gorm:"bigint"`
	EndTime              int64  `json:"end_time" gorm:"bigint"`
}

type InvoiceEligibleSubscription struct {
	UserSubscription
	PlanTitle string `json:"plan_title"`
}

func GetInvoiceEligibleSubscriptions(userId int) ([]InvoiceEligibleSubscription, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	cutoff := common.GetTimestamp() - int64(invoiceOptionInt("InvoiceLookbackDays", 90))*24*60*60
	var subscriptions []InvoiceEligibleSubscription
	err := DB.Table("user_subscriptions AS us").
		Select("us.*, sp.title AS plan_title").
		Joins("LEFT JOIN subscription_plans AS sp ON sp.id = us.plan_id").
		Where("us.user_id = ? AND us.invoice_eligible = ? AND us.created_at >= ?", userId, true, cutoff).
		Where("NOT EXISTS (SELECT 1 FROM invoice_application_items iai WHERE iai.user_subscription_id = us.id)").
		Order("us.created_at DESC, us.id DESC").Find(&subscriptions).Error
	return subscriptions, err
}

func CreateInvoiceApplication(userId int, title string, subscriptionIds []int) (*InvoiceApplication, error) {
	title = strings.TrimSpace(title)
	if userId <= 0 || title == "" || len(subscriptionIds) == 0 {
		return nil, errors.New("invoice title and subscriptions are required")
	}
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > InvoiceTitleMaxLength {
		return nil, errors.New("invoice title must not exceed 255 characters")
	}
	if len(subscriptionIds) > InvoiceSubscriptionLimit {
		return nil, errors.New("too many subscriptions in one invoice application")
	}
	seenSubscriptionIds := make(map[int]struct{}, len(subscriptionIds))
	for _, subscriptionId := range subscriptionIds {
		if subscriptionId <= 0 {
			return nil, errors.New("invalid subscription id")
		}
		if _, exists := seenSubscriptionIds[subscriptionId]; exists {
			return nil, errors.New("duplicate subscription id")
		}
		seenSubscriptionIds[subscriptionId] = struct{}{}
	}
	var application *InvoiceApplication
	err := DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var user User
		if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		var monthlyCount int64
		if err := tx.Model(&InvoiceApplication{}).Where("user_id = ? AND application_month = ?", userId, now.Format("2006-01")).Count(&monthlyCount).Error; err != nil {
			return err
		}
		monthlyLimit := invoiceOptionInt("InvoiceMonthlyLimit", 1)
		if monthlyCount >= int64(monthlyLimit) {
			return errors.New("invoice application already submitted this month")
		}
		var subscriptions []InvoiceEligibleSubscription
		if err := tx.Table("user_subscriptions AS us").Select("us.*, sp.title AS plan_title").
			Joins("LEFT JOIN subscription_plans AS sp ON sp.id = us.plan_id").
			Where("us.user_id = ? AND us.id IN ? AND us.invoice_eligible = ? AND us.created_at >= ?", userId, subscriptionIds, true, common.GetTimestamp()-int64(invoiceOptionInt("InvoiceLookbackDays", 90))*24*60*60).
			Where("NOT EXISTS (SELECT 1 FROM invoice_application_items iai WHERE iai.user_subscription_id = us.id)").Find(&subscriptions).Error; err != nil {
			return err
		}
		if len(subscriptions) != len(subscriptionIds) {
			return errors.New("one or more subscriptions are not eligible for invoicing")
		}
		application = &InvoiceApplication{UserId: userId, ApplicationMonth: now.Format("2006-01"), InvoiceTitle: title, Status: InvoiceApplicationStatusPending}
		for _, subscription := range subscriptions {
			if subscription.AmountTotal <= 0 {
				return errors.New("subscription quota is invalid")
			}
			application.TotalAmount += subscription.AmountTotal
			application.Items = append(application.Items, InvoiceApplicationItem{
				UserSubscriptionId: subscription.Id, PlanTitle: subscription.PlanTitle,
				AmountTotal: subscription.AmountTotal, StartTime: subscription.StartTime, EndTime: subscription.EndTime,
			})
		}
		return tx.Create(application).Error
	})
	return application, err
}

func ListUserInvoiceApplications(userId int) ([]InvoiceApplication, error) {
	var applications []InvoiceApplication
	err := DB.Preload("Items").Where("user_id = ?", userId).Order("created_at DESC, id DESC").Find(&applications).Error
	return applications, err
}

func ListInvoiceApplications(offset, limit int) ([]InvoiceApplication, int64, error) {
	var applications []InvoiceApplication
	var total int64
	query := DB.Model(&InvoiceApplication{}).Preload("User").Preload("Items").Order("created_at DESC, id DESC")
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Offset(offset).Limit(limit).Find(&applications).Error; err != nil {
		return nil, 0, err
	}
	return applications, total, nil
}

func GetInvoiceApplication(id int) (*InvoiceApplication, error) {
	var application InvoiceApplication
	if err := DB.Preload("User").Preload("Items").First(&application, id).Error; err != nil {
		return nil, err
	}
	return &application, nil
}

func SaveInvoicePDF(id int, path, name string) error {
	return DB.Model(&InvoiceApplication{}).Where("id = ?", id).Updates(map[string]interface{}{"pdf_path": path, "pdf_name": name, "updated_at": common.GetTimestamp()}).Error
}

func ClearInvoicePDF(id int) error {
	return DB.Model(&InvoiceApplication{}).Where("id = ?", id).Updates(map[string]interface{}{"pdf_path": "", "pdf_name": "", "updated_at": common.GetTimestamp()}).Error
}

func CompleteInvoiceApplication(id int) error {
	return DB.Model(&InvoiceApplication{}).Where("id = ? AND status = ?", id, InvoiceApplicationStatusPending).Updates(map[string]interface{}{"status": InvoiceApplicationStatusCompleted, "completed_at": common.GetTimestamp(), "updated_at": common.GetTimestamp()}).Error
}
