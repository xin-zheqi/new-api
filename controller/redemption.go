package controller

import (
	"errors"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetAllRedemptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.GetAllRedemptions(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchRedemptions(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.SearchRedemptions(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetRedemption(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemption,
	})
	return
}

func AddRedemption(c *gin.Context) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return
	}

	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if utf8.RuneCountInString(redemption.Name) == 0 || utf8.RuneCountInString(redemption.Name) > 20 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionNameLength)
		return
	}
	if redemption.Count <= 0 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountPositive)
		return
	}
	if redemption.Count > 100 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountMax)
		return
	}
	if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}
	if err = validateRedemptionConfig(c, &redemption); err != nil {
		handleRedemptionValidationError(c, err)
		return
	}
	var keys []string
	for i := 0; i < redemption.Count; i++ {
		key := common.GetUUID()
		cleanRedemption := model.Redemption{
			UserId:             c.GetInt("id"),
			Name:               redemption.Name,
			Key:                key,
			CreatedTime:        common.GetTimestamp(),
			Quota:              redemption.Quota,
			RedeemType:         redemption.RedeemType,
			SubscriptionPlanId: redemption.SubscriptionPlanId,
			ExpiredTime:        redemption.ExpiredTime,
		}
		err = cleanRedemption.Insert()
		if err != nil {
			common.SysError("failed to insert redemption: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": i18n.T(c, i18n.MsgRedemptionCreateFailed),
				"data":    keys,
			})
			return
		}
		keys = append(keys, key)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
	})
	return
}

func DeleteRedemption(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateRedemption(c *gin.Context) {
	statusOnly := c.Query("status_only")
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cleanRedemption, err := model.GetRedemptionById(redemption.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if cleanRedemption.Status == common.RedemptionCodeStatusUsed {
		common.ApiErrorI18n(c, i18n.MsgRedemptionUsedImmutable)
		return
	}
	if statusOnly == "" {
		if utf8.RuneCountInString(redemption.Name) == 0 || utf8.RuneCountInString(redemption.Name) > 20 {
			common.ApiErrorI18n(c, i18n.MsgRedemptionNameLength)
			return
		}
		if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		// If you add more fields, please also update redemption.Update()
		cleanRedemption.Name = redemption.Name
		cleanRedemption.Quota = redemption.Quota
		cleanRedemption.RedeemType = redemption.RedeemType
		cleanRedemption.SubscriptionPlanId = redemption.SubscriptionPlanId
		cleanRedemption.ExpiredTime = redemption.ExpiredTime
		if err = validateRedemptionConfig(c, cleanRedemption); err != nil {
			handleRedemptionValidationError(c, err)
			return
		}
	}
	if statusOnly != "" {
		cleanRedemption.Status = redemption.Status
	}
	err = cleanRedemption.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanRedemption,
	})
	return
}

func DeleteInvalidRedemption(c *gin.Context) {
	rows, err := model.DeleteInvalidRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

func validateExpiredTime(c *gin.Context, expired int64) (bool, string) {
	if expired != 0 && expired < common.GetTimestamp() {
		return false, i18n.T(c, i18n.MsgRedemptionExpireTimeInvalid)
	}
	return true, ""
}

func validateRedemptionConfig(c *gin.Context, redemption *model.Redemption) error {
	if redemption == nil {
		return model.ErrRedemptionTypeInvalid
	}
	redeemType := model.NormalizeRedemptionType(redemption.RedeemType)
	if redeemType == "" {
		return model.ErrRedemptionTypeInvalid
	}
	redemption.RedeemType = redeemType
	switch redeemType {
	case model.RedemptionTypeQuota:
		if redemption.Quota <= 0 {
			return model.ErrRedemptionQuotaInvalid
		}
		redemption.SubscriptionPlanId = 0
	case model.RedemptionTypeSubscription:
		redemption.Quota = 0
		if redemption.SubscriptionPlanId <= 0 {
			return model.ErrRedemptionPlanRequired
		}
		_, err := model.GetSubscriptionPlanById(redemption.SubscriptionPlanId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrRedemptionPlanNotFound
			}
			return err
		}
	default:
		return model.ErrRedemptionTypeInvalid
	}
	return nil
}

func handleRedemptionValidationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrRedemptionTypeInvalid):
		common.ApiErrorI18n(c, i18n.MsgRedemptionTypeInvalid)
	case errors.Is(err, model.ErrRedemptionQuotaInvalid):
		common.ApiErrorI18n(c, i18n.MsgRedemptionQuotaPositive)
	case errors.Is(err, model.ErrRedemptionPlanRequired):
		common.ApiErrorI18n(c, i18n.MsgRedemptionSubscriptionPlanRequired)
	case errors.Is(err, model.ErrRedemptionPlanNotFound):
		common.ApiErrorI18n(c, i18n.MsgRedemptionSubscriptionPlanNotFound)
	case errors.Is(err, model.ErrRedemptionUsedImmutable):
		common.ApiErrorI18n(c, i18n.MsgRedemptionUsedImmutable)
	default:
		common.ApiError(c, err)
	}
}
