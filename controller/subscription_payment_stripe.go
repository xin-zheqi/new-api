package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/thanhpk/randstr"
)

type SubscriptionStripePayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestStripePay(c *gin.Context) {
	if rejectSubscriptionPurchaseWhenMallEnabled(c) {
		return
	}
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionStripePayRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentRequestMaxBytes)
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.StripePriceId == "" {
		common.ApiErrorMsg(c, "该套餐未配置 StripePriceId")
		return
	}
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		common.ApiErrorMsg(c, "Stripe 未配置或密钥无效")
		return
	}
	if setting.StripeWebhookSecret == "" {
		common.ApiErrorMsg(c, "Stripe Webhook 未配置")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	reference := fmt.Sprintf("sub-stripe-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "sub_ref_" + common.Sha1([]byte(reference))
	expectedPayment, err := subscriptionPlanPaymentSnapshot(plan)
	if err != nil {
		common.ApiErrorMsg(c, "套餐金额配置错误")
		return
	}

	order := &model.SubscriptionOrder{
		UserId:               userId,
		PlanId:               plan.Id,
		Money:                plan.PriceAmount,
		TradeNo:              referenceId,
		PaymentMethod:        model.PaymentMethodStripe,
		PaymentProvider:      model.PaymentProviderStripe,
		ExpectedAmountMicros: expectedPayment.AmountMicros,
		ExpectedCurrency:     expectedPayment.Currency,
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusPending,
	}
	if err := model.CreatePendingSubscriptionOrder(order, plan); err != nil {
		if errors.Is(err, model.ErrSubscriptionPurchaseLimit) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	payLink, checkoutPayment, err := genStripeSubscriptionLink(referenceId, user.StripeCustomer, user.Email, plan.StripePriceId)
	if err != nil {
		_ = model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅支付链接创建失败 trade_no=%s plan_id=%d error=%q", referenceId, plan.Id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if checkoutPayment != expectedPayment {
		_ = model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅 Checkout 金额或币种不匹配 trade_no=%s plan_id=%d expected_amount_micros=%d actual_amount_micros=%d expected_currency=%s actual_currency=%s", referenceId, plan.Id, expectedPayment.AmountMicros, checkoutPayment.AmountMicros, expectedPayment.Currency, checkoutPayment.Currency))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "套餐支付配置错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func subscriptionPlanPaymentSnapshot(plan *model.SubscriptionPlan) (model.SubscriptionPaymentSnapshot, error) {
	if plan == nil {
		return model.SubscriptionPaymentSnapshot{}, errors.New("invalid subscription plan")
	}
	// SubscriptionPlan is persisted as decimal(10,6). Preserve all stored
	// precision so three-decimal currencies (for example BHD/KWD) are not
	// silently rounded to cents before Stripe's minor-unit snapshot is checked.
	amount := strconv.FormatFloat(plan.PriceAmount, 'f', 6, 64)
	return model.NewSubscriptionPaymentFromMajorUnits(amount, plan.Currency)
}

func genStripeSubscriptionLink(referenceId string, customerId string, email string, priceId string) (string, model.SubscriptionPaymentSnapshot, error) {
	stripe.Key = setting.StripeApiSecret

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(paymentReturnPath("/console/topup")),
		CancelURL:         stripe.String(paymentReturnPath("/console/topup")),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceId),
				Quantity: stripe.Int64(1),
			},
		},
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
	}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}
		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	result, err := session.New(params)
	if err != nil {
		return "", model.SubscriptionPaymentSnapshot{}, err
	}
	payment, err := model.NewSubscriptionPaymentFromMinorUnits(result.AmountTotal, string(result.Currency))
	if err != nil {
		return "", model.SubscriptionPaymentSnapshot{}, fmt.Errorf("Stripe Checkout Session 未返回有效金额或币种: %w", err)
	}
	return result.URL, payment, nil
}
