package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type SubscriptionBalancePayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestBalancePay(c *gin.Context) {
	if rejectSubscriptionPurchaseWhenMallEnabled(c) || !requirePaymentCompliance(c) {
		return
	}
	if !operation_setting.GetPaymentSetting().BalanceSubscriptionEnabled {
		common.ApiErrorI18n(c, i18n.MsgSubscriptionBalanceDisabled)
		return
	}

	var request SubscriptionBalancePayRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentRequestMaxBytes)
	if err := c.ShouldBindJSON(&request); err != nil || request.PlanId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	err := model.PurchaseSubscriptionWithBalance(c.GetInt("id"), request.PlanId)
	switch {
	case err == nil:
		common.ApiSuccess(c, nil)
	case errors.Is(err, model.ErrSubscriptionBalancePaymentDisabled):
		common.ApiErrorI18n(c, i18n.MsgSubscriptionBalanceDisabled)
	case errors.Is(err, model.ErrSubscriptionInsufficientBalance):
		common.ApiErrorI18n(c, i18n.MsgSubscriptionBalanceLow)
	case errors.Is(err, model.ErrSubscriptionPurchaseLimit):
		common.ApiErrorI18n(c, i18n.MsgSubscriptionPurchaseMax)
	default:
		common.ApiError(c, err)
	}
}
