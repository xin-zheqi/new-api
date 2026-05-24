package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

const (
	RedemptionTypeQuota        = "quota"
	RedemptionTypeSubscription = "subscription"
)

type Redemption struct {
	Id                 int            `json:"id"`
	UserId             int            `json:"user_id"`
	Key                string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status             int            `json:"status" gorm:"default:1"`
	Name               string         `json:"name" gorm:"index"`
	Quota              int            `json:"quota" gorm:"default:100"`
	RedeemType         string         `json:"redeem_type" gorm:"type:varchar(32);not null;default:'quota'"`
	SubscriptionPlanId int            `json:"subscription_plan_id" gorm:"index;default:0"`
	SubscriptionPlanTitle string      `json:"subscription_plan_title,omitempty" gorm:"-:all"`
	CreatedTime        int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime       int64          `json:"redeemed_time" gorm:"bigint"`
	Count              int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId         int            `json:"used_user_id"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
	ExpiredTime        int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
}

type subscriptionPlanTitleRow struct {
	Id    int    `gorm:"column:id"`
	Title string `gorm:"column:title"`
}

type RedeemSubscriptionInfo struct {
	PlanId       int    `json:"plan_id"`
	Title        string `json:"title"`
	EndTime      int64  `json:"end_time"`
	UpgradeGroup string `json:"upgrade_group,omitempty"`
}

type RedeemResult struct {
	Kind         string                  `json:"kind"`
	Quota        int                     `json:"quota"`
	Subscription *RedeemSubscriptionInfo `json:"subscription,omitempty"`
}

func NormalizeRedemptionType(redeemType string) string {
	switch strings.ToLower(strings.TrimSpace(redeemType)) {
	case "", RedemptionTypeQuota:
		return RedemptionTypeQuota
	case RedemptionTypeSubscription:
		return RedemptionTypeSubscription
	default:
		return ""
	}
}

func IsValidRedemptionType(redeemType string) bool {
	return NormalizeRedemptionType(redeemType) != ""
}

func (redemption *Redemption) normalizeFields() {
	redemption.RedeemType = NormalizeRedemptionType(redemption.RedeemType)
	if redemption.RedeemType == "" {
		redemption.RedeemType = RedemptionTypeQuota
	}
	switch redemption.RedeemType {
	case RedemptionTypeQuota:
		redemption.SubscriptionPlanId = 0
	case RedemptionTypeSubscription:
		redemption.Quota = 0
	}
}

func (redemption *Redemption) AfterFind(tx *gorm.DB) error {
	redemption.normalizeFields()
	return nil
}

func (redemption *Redemption) BeforeSave(tx *gorm.DB) error {
	redemption.normalizeFields()
	return nil
}

func enrichRedemptionSubscriptionPlanTitlesTx(tx *gorm.DB, redemptions []*Redemption) error {
	if len(redemptions) == 0 {
		return nil
	}
	planIds := make([]int, 0)
	seen := make(map[int]struct{})
	for _, redemption := range redemptions {
		if redemption == nil || redemption.RedeemType != RedemptionTypeSubscription || redemption.SubscriptionPlanId <= 0 {
			continue
		}
		if _, ok := seen[redemption.SubscriptionPlanId]; ok {
			continue
		}
		seen[redemption.SubscriptionPlanId] = struct{}{}
		planIds = append(planIds, redemption.SubscriptionPlanId)
	}
	if len(planIds) == 0 {
		return nil
	}
	query := DB
	if tx != nil {
		query = tx
	}
	var rows []subscriptionPlanTitleRow
	if err := query.Model(&SubscriptionPlan{}).Select("id", "title").Where("id IN ?", planIds).Find(&rows).Error; err != nil {
		return err
	}
	titleMap := make(map[int]string, len(rows))
	for _, row := range rows {
		titleMap[row.Id] = row.Title
	}
	for _, redemption := range redemptions {
		if redemption == nil || redemption.SubscriptionPlanId <= 0 {
			continue
		}
		redemption.SubscriptionPlanTitle = titleMap[redemption.SubscriptionPlanId]
	}
	return nil
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = enrichRedemptionSubscriptionPlanTitlesTx(tx, redemptions); err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Build query based on keyword type
	query := tx.Model(&Redemption{})

	// Only try to convert to ID if the string represents a valid integer
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
	} else {
		query = query.Where("name LIKE ?", keyword+"%")
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	if err = enrichRedemptionSubscriptionPlanTitlesTx(tx, redemptions); err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	if err == nil {
		_ = enrichRedemptionSubscriptionPlanTitlesTx(nil, []*Redemption{&redemption})
	}
	return &redemption, err
}

func Redeem(key string, userId int) (result *RedeemResult, err error) {
	if key == "" {
		return nil, ErrRedemptionInvalid
	}
	if userId == 0 {
		return nil, ErrRedeemFailed
	}
	redemption := &Redemption{}
	redeemResult := &RedeemResult{
		Kind:  RedemptionTypeQuota,
		Quota: 0,
	}
	logMessage := ""
	upgradeGroup := ""

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRedemptionInvalid
			}
			return err
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return ErrRedemptionUsed
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return ErrRedemptionExpired
		}
		switch redemption.RedeemType {
		case RedemptionTypeQuota:
			if redemption.Quota <= 0 {
				return ErrRedemptionQuotaInvalid
			}
			err = tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
			if err != nil {
				return err
			}
			redeemResult.Kind = RedemptionTypeQuota
			redeemResult.Quota = redemption.Quota
			logMessage = fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id)
		case RedemptionTypeSubscription:
			if redemption.SubscriptionPlanId <= 0 {
				return ErrRedemptionPlanRequired
			}
			plan, err := getSubscriptionPlanByIdTx(tx, redemption.SubscriptionPlanId)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrRedemptionPlanNotFound
				}
				return err
			}
			subscription, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "redemption")
			if err != nil {
				return err
			}
			upgradeGroup = strings.TrimSpace(plan.UpgradeGroup)
			redeemResult.Kind = RedemptionTypeSubscription
			redeemResult.Quota = 0
			redeemResult.Subscription = &RedeemSubscriptionInfo{
				PlanId:       plan.Id,
				Title:        plan.Title,
				EndTime:      subscription.EndTime,
				UpgradeGroup: upgradeGroup,
			}
			logMessage = fmt.Sprintf("通过兑换码开通订阅套餐 %s，兑换码ID %d", plan.Title, redemption.Id)
		default:
			return ErrRedemptionTypeInvalid
		}
		redemption.RedeemedTime = common.GetTimestamp()
		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId
		err = tx.Save(redemption).Error
		return err
	})
	if err != nil {
		if errors.Is(err, ErrRedemptionInvalid) ||
			errors.Is(err, ErrRedemptionUsed) ||
			errors.Is(err, ErrRedemptionExpired) ||
			errors.Is(err, ErrRedemptionTypeInvalid) ||
			errors.Is(err, ErrRedemptionQuotaInvalid) ||
			errors.Is(err, ErrRedemptionPlanRequired) ||
			errors.Is(err, ErrRedemptionPlanNotFound) ||
			err.Error() == "已达到该套餐购买上限" {
			return nil, err
		}
		common.SysError("redemption failed: " + err.Error())
		return nil, ErrRedeemFailed
	}
	if upgradeGroup != "" {
		_ = UpdateUserGroupCache(userId, upgradeGroup)
	}
	if logMessage != "" {
		RecordLog(userId, LogTypeTopup, logMessage)
	}
	return redeemResult, nil
}

func (redemption *Redemption) Insert() error {
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	var err error
	err = DB.Model(redemption).Select("name", "status", "quota", "redeem_type", "subscription_plan_id", "redeemed_time", "expired_time").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
