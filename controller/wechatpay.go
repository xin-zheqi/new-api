package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	wechatNativeMaxAmountMinor = int64(1_000_000_000)
	wechatNotifyURLMaxBytes    = 255
)

type WechatPayRequest struct {
	Amount int64 `json:"amount"`
}

type WechatSubscriptionPayRequest struct {
	PlanID int `json:"plan_id"`
}

type wechatOrderResult struct {
	TradeNo        string `json:"trade_no"`
	Status         string `json:"status"`
	TradeState     string `json:"trade_state,omitempty"`
	TradeStateDesc string `json:"trade_state_desc,omitempty"`
	CodeURL        string `json:"code_url,omitempty"`
	Amount         string `json:"amount,omitempty"`
	Currency       string `json:"currency,omitempty"`
	ExpiresAt      int64  `json:"expires_at,omitempty"`
}

type wechatRuntimeSettings struct {
	Enabled            bool
	NotifyURL          string
	OrderExpireMinutes int
	MinTopUp           int
	UnitPrice          float64
	Config             service.WechatPayConfig
}

var wechatClientCache struct {
	sync.Mutex
	initialized bool
	config      service.WechatPayConfig
	client      *service.WechatPayClient
	err         error
}

func currentWechatRuntimeSettings() wechatRuntimeSettings {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return wechatRuntimeSettings{
		Enabled:            setting.WechatEnabled,
		NotifyURL:          setting.WechatNotifyURL,
		OrderExpireMinutes: setting.WechatOrderExpireMinutes,
		MinTopUp:           setting.EffectiveWechatMinTopUp(operation_setting.MinTopUp),
		UnitPrice:          setting.EffectiveWechatUnitPrice(operation_setting.Price),
		Config: service.WechatPayConfig{
			MchID:                   setting.WechatMchID,
			AppID:                   setting.WechatAppID,
			ApiV3Key:                setting.WechatApiV3Key,
			CertificateSerialNumber: setting.WechatCertificateSerialNumber,
			PrivateKey:              setting.WechatPrivateKey,
			MerchantCertificate:     setting.WechatMerchantCertificate,
			PayPublicKeyID:          setting.WechatPayPublicKeyID,
			PayPublicKey:            setting.WechatPayPublicKey,
			PlatformCertificate:     setting.WechatPlatformCertificate,
		},
	}
}

func wechatConfig() service.WechatPayConfig {
	return currentWechatRuntimeSettings().Config
}

func newWechatClient(config service.WechatPayConfig) (*service.WechatPayClient, error) {
	wechatClientCache.Lock()
	defer wechatClientCache.Unlock()
	if wechatClientCache.initialized && wechatClientCache.config == config {
		return wechatClientCache.client, wechatClientCache.err
	}
	client, err := service.NewWechatPayClient(config)
	wechatClientCache.initialized = true
	wechatClientCache.config = config
	wechatClientCache.client = client
	wechatClientCache.err = err
	return client, err
}

func newConfiguredWechatClient(settings wechatRuntimeSettings) (*service.WechatPayClient, error) {
	if !isPaymentComplianceConfirmed() || !settings.Enabled ||
		!wechatCredentialsPresent(settings.Config) || !isWechatNotifyURLConfigured(settings) {
		return nil, errors.New("wechat pay is not configured")
	}
	return newWechatClient(settings.Config)
}

func resolveWechatNotifyURL(configuredURL string) (string, error) {
	notifyURL := strings.TrimSpace(configuredURL)
	if notifyURL == "" {
		notifyURL = strings.TrimRight(strings.TrimSpace(service.GetCallbackAddress()), "/") + "/api/wechat/notify"
	}
	if len([]byte(notifyURL)) > wechatNotifyURLMaxBytes {
		return "", errors.New("wechat notify URL is too long")
	}
	parsed, err := url.Parse(notifyURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || !strings.EqualFold(parsed.Scheme, "https") {
		return "", errors.New("wechat notify URL is invalid")
	}
	return strings.TrimRight(notifyURL, "/"), nil
}

// wechatPaymentSnapshot converts a server-calculated CNY amount to the exact
// fixed-point representation shared by all payment providers. WeChat Native
// accepts CNY minor units (fen), so no client-supplied money value is trusted.
func wechatPaymentSnapshot(money float64) (model.SubscriptionPaymentSnapshot, int64, error) {
	if math.IsNaN(money) || math.IsInf(money, 0) || money <= 0 {
		return model.SubscriptionPaymentSnapshot{}, 0, errors.New("invalid payment amount")
	}
	return wechatPaymentSnapshotDecimal(decimal.NewFromFloat(money))
}

func wechatTopUpPaymentSnapshot(amount int64, group string, unitPrice float64) (model.SubscriptionPaymentSnapshot, int64, error) {
	return wechatPaymentSnapshot(getPayMoneyWithUnitPrice(amount, group, unitPrice))
}

func wechatPaymentSnapshotDecimal(amount decimal.Decimal) (model.SubscriptionPaymentSnapshot, int64, error) {
	if !amount.IsPositive() {
		return model.SubscriptionPaymentSnapshot{}, 0, errors.New("invalid payment amount")
	}
	// Native orders are denominated in fen. Round once at this boundary and use
	// the resulting integer for both the provider request and local snapshot.
	rounded := amount.Round(2)
	minorUnitsDecimal := rounded.Mul(decimal.NewFromInt(100))
	if !minorUnitsDecimal.IsInteger() ||
		minorUnitsDecimal.GreaterThan(decimal.NewFromInt(wechatNativeMaxAmountMinor)) {
		return model.SubscriptionPaymentSnapshot{}, 0, errors.New("payment amount is outside WeChat limits")
	}
	minorUnits := minorUnitsDecimal.IntPart()
	if minorUnits <= 0 {
		return model.SubscriptionPaymentSnapshot{}, 0, errors.New("invalid payment amount")
	}
	snapshot, err := model.NewSubscriptionPaymentFromMajorUnits(rounded.StringFixed(2), "CNY")
	if err != nil {
		return model.SubscriptionPaymentSnapshot{}, 0, err
	}
	return snapshot, minorUnits, nil
}

func wechatSubscriptionPaymentSnapshot(plan *model.SubscriptionPlan, unitPrice float64) (model.SubscriptionPaymentSnapshot, int64, error) {
	if plan == nil || math.IsNaN(plan.PriceAmount) || math.IsInf(plan.PriceAmount, 0) || plan.PriceAmount <= 0 {
		return model.SubscriptionPaymentSnapshot{}, 0, errors.New("invalid subscription payment amount")
	}
	currency, err := model.NormalizePaymentCurrency(plan.Currency)
	if err != nil {
		return model.SubscriptionPaymentSnapshot{}, 0, errors.New("unsupported subscription payment currency")
	}
	var amount decimal.Decimal
	switch currency {
	case "USD":
		if !isFinitePaymentValue(unitPrice) || unitPrice <= 0 {
			return model.SubscriptionPaymentSnapshot{}, 0, errors.New("invalid WeChat exchange rate")
		}
		amount = decimal.NewFromFloat(plan.PriceAmount).Mul(decimal.NewFromFloat(unitPrice))
	case "CNY":
		amount = decimal.NewFromFloat(plan.PriceAmount)
	default:
		return model.SubscriptionPaymentSnapshot{}, 0, errors.New("WeChat Pay supports USD or CNY subscription plans only")
	}
	return wechatPaymentSnapshotDecimal(amount)
}

func wechatOrderExpiry(minutes int) time.Time {
	// Native code_url values are valid for at most two hours, while WeChat
	// requires time_expire to be at least one minute after order creation.
	if minutes < 1 || minutes > 120 {
		minutes = 120
	}
	return time.Now().Add(time.Duration(minutes) * time.Minute)
}

func updateWechatOrderTerminalStatus(tradeNo, status string) error {
	if tradeNo == "" {
		return model.ErrPaymentMethodMismatch
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	switch tradeNo[0] {
	case 'W':
		return model.UpdatePendingWechatTopUpStatus(tradeNo, status)
	case 'S':
		return model.UpdatePendingWechatSubscriptionOrderStatus(tradeNo, status)
	default:
		return model.ErrPaymentMethodMismatch
	}
}

func reconcileWechatPendingOrder(
	ctx context.Context,
	client *service.WechatPayClient,
	config service.WechatPayConfig,
	tradeNo string,
	closeUnpaid bool,
	missingIsFailed bool,
	source string,
	callerIP string,
) (string, error) {
	order, err := client.QueryOrderByOutTradeNo(ctx, tradeNo)
	if err != nil {
		if !missingIsFailed || !service.IsWechatOrderNotExist(err) {
			return common.TopUpStatusPending, err
		}
		if err := updateWechatOrderTerminalStatus(tradeNo, common.TopUpStatusFailed); err != nil {
			return common.TopUpStatusPending, err
		}
		return common.TopUpStatusFailed, nil
	}

	status, err := validateWechatQueriedOrder(order, tradeNo, config)
	if err != nil {
		return common.TopUpStatusPending, err
	}
	if status == common.TopUpStatusSuccess {
		if err := settleWechatOrder(tradeNo, order.Amount.Total, order.Amount.Currency, source, callerIP); err != nil {
			return common.TopUpStatusPending, err
		}
		return common.TopUpStatusSuccess, nil
	}
	if status == common.TopUpStatusPending {
		if !closeUnpaid || order.TradeState != "NOTPAY" {
			return common.TopUpStatusPending, nil
		}
		if err := client.CloseOrder(ctx, tradeNo); err != nil {
			return common.TopUpStatusPending, err
		}
		if err := updateWechatOrderTerminalStatus(tradeNo, common.TopUpStatusExpired); err != nil {
			return common.TopUpStatusPending, err
		}
		return common.TopUpStatusExpired, nil
	}
	if err := updateWechatOrderTerminalStatus(tradeNo, status); err != nil {
		return common.TopUpStatusPending, err
	}
	return status, nil
}

// reconcileWechatCreateFailure resolves the indeterminate result of a failed
// Native prepay call. A transport error or signed 5xx response does not prove
// that WeChat rejected the order, so only an authenticated query (and, for an
// unpaid order, an authenticated close response) may move it out of pending.
func reconcileWechatCreateFailure(c *gin.Context, client *service.WechatPayClient, config service.WechatPayConfig, tradeNo string) (string, error) {
	return reconcileWechatPendingOrder(
		c.Request.Context(), client, config, tradeNo, true, true,
		"create_reconcile", common.GetClientIP(c),
	)
}

func newWechatTradeNo(prefix byte) string {
	// Snowflake IDs are positive, globally unique, and keep the order number
	// within WeChat's 6-32 ASCII character constraint.
	return fmt.Sprintf("%c%d", prefix, common.NextSnowflakeID())
}

func RequestWechatPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	if !requirePaymentMethodEnabled(c, model.PaymentMethodWechat) {
		return
	}
	wechatSettings := currentWechatRuntimeSettings()
	client, err := newConfiguredWechatClient(wechatSettings)
	if err != nil {
		common.ApiErrorMsg(c, "微信支付暂未配置完整")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentRequestMaxBytes)
	var req WechatPayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if valid, message := validateTopUpAmount(req.Amount, topUpDisplayLimit(wechatSettings.MinTopUp)); !valid {
		common.ApiErrorMsg(c, message)
		return
	}

	userID := c.GetInt("id")
	creditedQuota, rejected := validateTopUpQuotaForUser(c, userID, req.Amount)
	if rejected {
		return
	}
	group, err := model.GetUserGroup(userID, true)
	if err != nil {
		common.ApiErrorMsg(c, "获取用户分组失败")
		return
	}
	snapshot, amountMinor, err := wechatTopUpPaymentSnapshot(req.Amount, group, wechatSettings.UnitPrice)
	if err != nil {
		common.ApiErrorMsg(c, "充值金额配置错误")
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = decimal.NewFromInt(req.Amount).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	expiresAt := wechatOrderExpiry(wechatSettings.OrderExpireMinutes)
	notifyURL, err := resolveWechatNotifyURL(wechatSettings.NotifyURL)
	if err != nil {
		common.ApiErrorMsg(c, "微信支付回调地址配置错误")
		return
	}
	tradeNo := newWechatTradeNo('W')
	order := &model.TopUp{
		UserId:               userID,
		Amount:               amount,
		Money:                decimal.NewFromInt(snapshot.AmountMicros).Shift(-6).InexactFloat64(),
		CreditedQuota:        int64(creditedQuota),
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodWechat,
		PaymentProvider:      model.PaymentProviderWechat,
		Currency:             "CNY",
		ExpectedAmountMicros: snapshot.AmountMicros,
		ExpectedCurrency:     snapshot.Currency,
		CreateTime:           common.GetTimestamp(),
		PaymentExpireTime:    expiresAt.Unix(),
		Status:               common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付创建充值订单失败 user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	prepay, err := client.CreateNativeOrder(c.Request.Context(), service.WechatNativeOrder{
		AppID:       wechatSettings.Config.AppID,
		MchID:       wechatSettings.Config.MchID,
		Description: "账户余额充值",
		OutTradeNo:  tradeNo,
		NotifyURL:   notifyURL,
		AmountTotal: amountMinor,
		TimeExpire:  &expiresAt,
	})
	if err != nil {
		reconciledStatus, reconcileErr := reconcileWechatCreateFailure(c, client, wechatSettings.Config, tradeNo)
		if reconcileErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付充值订单下单失败后状态确认异常 trade_no=%s status=%s error=%q", tradeNo, reconciledStatus, reconcileErr.Error()))
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 Native 下单失败 user_id=%d trade_no=%s reconciled_status=%s error=%q", userID, tradeNo, reconciledStatus, err.Error()))
		common.ApiErrorMsg(c, "拉起微信支付失败")
		return
	}
	common.ApiSuccess(c, wechatOrderResult{
		TradeNo:   tradeNo,
		Status:    common.TopUpStatusPending,
		CodeURL:   prepay.CodeURL,
		Amount:    decimal.NewFromInt(snapshot.AmountMicros).Shift(-6).StringFixed(2),
		Currency:  snapshot.Currency,
		ExpiresAt: expiresAt.Unix(),
	})
}

// RequestWechatAmount returns the exact CNY amount that a WeChat Native order
// would use. Keeping this calculation server-side prevents the confirmation UI
// from drifting from the amount signed into the eventual payment order.
func RequestWechatAmount(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	if !requirePaymentMethodEnabled(c, model.PaymentMethodWechat) {
		return
	}
	settings := currentWechatRuntimeSettings()
	if _, err := newConfiguredWechatClient(settings); err != nil {
		common.ApiErrorMsg(c, "微信支付暂未配置完整")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentRequestMaxBytes)
	var req WechatPayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if valid, message := validateTopUpAmount(req.Amount, topUpDisplayLimit(settings.MinTopUp)); !valid {
		common.ApiErrorMsg(c, message)
		return
	}
	userID := c.GetInt("id")
	if _, rejected := validateTopUpQuotaForUser(c, userID, req.Amount); rejected {
		return
	}
	group, err := model.GetUserGroup(userID, true)
	if err != nil {
		common.ApiErrorMsg(c, "获取用户分组失败")
		return
	}
	snapshot, _, err := wechatTopUpPaymentSnapshot(req.Amount, group, settings.UnitPrice)
	if err != nil {
		common.ApiErrorMsg(c, "充值金额配置错误")
		return
	}
	common.ApiSuccess(c, decimal.NewFromInt(snapshot.AmountMicros).Shift(-6).StringFixed(2))
}

func RequestWechatSubscriptionPay(c *gin.Context) {
	if rejectSubscriptionPurchaseWhenMallEnabled(c) || !requirePaymentCompliance(c) {
		return
	}
	if !requirePaymentMethodEnabled(c, model.PaymentMethodWechat) {
		return
	}
	wechatSettings := currentWechatRuntimeSettings()
	client, err := newConfiguredWechatClient(wechatSettings)
	if err != nil {
		common.ApiErrorMsg(c, "微信支付暂未配置完整")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentRequestMaxBytes)
	var req WechatSubscriptionPayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.PlanID <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	plan, err := model.GetSubscriptionPlanById(req.PlanID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	snapshot, amountMinor, err := wechatSubscriptionPaymentSnapshot(plan, wechatSettings.UnitPrice)
	if err != nil {
		common.ApiErrorMsg(c, "套餐金额配置错误")
		return
	}
	expiresAt := wechatOrderExpiry(wechatSettings.OrderExpireMinutes)
	notifyURL, err := resolveWechatNotifyURL(wechatSettings.NotifyURL)
	if err != nil {
		common.ApiErrorMsg(c, "微信支付回调地址配置错误")
		return
	}
	userID := c.GetInt("id")
	tradeNo := newWechatTradeNo('S')
	order := &model.SubscriptionOrder{
		UserId:               userID,
		PlanId:               plan.Id,
		Money:                decimal.NewFromInt(snapshot.AmountMicros).Shift(-6).InexactFloat64(),
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodWechat,
		PaymentProvider:      model.PaymentProviderWechat,
		ExpectedAmountMicros: snapshot.AmountMicros,
		ExpectedCurrency:     snapshot.Currency,
		CreateTime:           common.GetTimestamp(),
		PaymentExpireTime:    expiresAt.Unix(),
		Status:               common.TopUpStatusPending,
	}
	if err := model.CreatePendingSubscriptionOrder(order, plan); err != nil {
		if errors.Is(err, model.ErrSubscriptionPurchaseLimit) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	prepay, err := client.CreateNativeOrder(c.Request.Context(), service.WechatNativeOrder{
		AppID:       wechatSettings.Config.AppID,
		MchID:       wechatSettings.Config.MchID,
		Description: "账户余额充值（订阅）",
		OutTradeNo:  tradeNo,
		NotifyURL:   notifyURL,
		AmountTotal: amountMinor,
		TimeExpire:  &expiresAt,
	})
	if err != nil {
		reconciledStatus, reconcileErr := reconcileWechatCreateFailure(c, client, wechatSettings.Config, tradeNo)
		if reconcileErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付订阅订单下单失败后状态确认异常 trade_no=%s status=%s error=%q", tradeNo, reconciledStatus, reconcileErr.Error()))
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 Native 订阅下单失败 user_id=%d plan_id=%d trade_no=%s reconciled_status=%s error=%q", userID, plan.Id, tradeNo, reconciledStatus, err.Error()))
		common.ApiErrorMsg(c, "拉起微信支付失败")
		return
	}
	common.ApiSuccess(c, wechatOrderResult{
		TradeNo:   tradeNo,
		Status:    common.TopUpStatusPending,
		CodeURL:   prepay.CodeURL,
		Amount:    decimal.NewFromInt(snapshot.AmountMicros).Shift(-6).StringFixed(2),
		Currency:  snapshot.Currency,
		ExpiresAt: expiresAt.Unix(),
	})
}

func QueryWechatOrder(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	if len(tradeNo) < 6 || len(tradeNo) > 32 || !isWechatTradeNoValue(tradeNo) {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}
	userID := c.GetInt("id")
	isTopUpOrder := tradeNo[0] == 'W'
	paymentExpireTime := int64(0)
	if isTopUpOrder {
		topUp, err := model.FindTopUpByTradeNo(tradeNo)
		if err != nil {
			if errors.Is(err, model.ErrTopUpNotFound) {
				common.ApiErrorMsg(c, "订单不存在")
			} else {
				common.ApiErrorMsg(c, "查询订单失败")
			}
			return
		}
		if topUp.UserId != userID || topUp.PaymentProvider != model.PaymentProviderWechat {
			common.ApiErrorMsg(c, "订单不存在")
			return
		}
		if topUp.Status == common.TopUpStatusSuccess {
			common.ApiSuccess(c, wechatOrderResult{TradeNo: tradeNo, Status: common.TopUpStatusSuccess, Amount: fmt.Sprintf("%.2f", topUp.Money), Currency: "CNY"})
			return
		}
		if topUp.Status == common.TopUpStatusFailed || topUp.Status == common.TopUpStatusExpired {
			common.ApiSuccess(c, wechatOrderResult{TradeNo: tradeNo, Status: topUp.Status})
			return
		}
		if topUp.Status != common.TopUpStatusPending {
			common.ApiErrorMsg(c, "订单状态异常")
			return
		}
		paymentExpireTime = topUp.PaymentExpireTime
	} else if tradeNo[0] == 'S' {
		subscription, err := model.FindSubscriptionOrderByTradeNo(tradeNo)
		if err != nil {
			if errors.Is(err, model.ErrSubscriptionOrderNotFound) {
				common.ApiErrorMsg(c, "订单不存在")
			} else {
				common.ApiErrorMsg(c, "查询订单失败")
			}
			return
		}
		if subscription.UserId != userID || subscription.PaymentProvider != model.PaymentProviderWechat {
			common.ApiErrorMsg(c, "订单不存在")
			return
		}
		if subscription.Status == common.TopUpStatusSuccess {
			common.ApiSuccess(c, wechatOrderResult{TradeNo: tradeNo, Status: common.TopUpStatusSuccess, Amount: fmt.Sprintf("%.2f", subscription.Money), Currency: "CNY"})
			return
		}
		if subscription.Status == common.TopUpStatusFailed || subscription.Status == common.TopUpStatusExpired {
			common.ApiSuccess(c, wechatOrderResult{TradeNo: tradeNo, Status: subscription.Status})
			return
		}
		if subscription.Status != common.TopUpStatusPending {
			common.ApiErrorMsg(c, "订单状态异常")
			return
		}
		paymentExpireTime = subscription.PaymentExpireTime
	} else {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}
	// Existing orders remain queryable after the administrator disables new
	// checkouts. This mirrors the webhook path and prevents paid orders from
	// becoming stranded during a configuration change.
	wechatSettings := currentWechatRuntimeSettings()
	client, err := newWechatClient(wechatSettings.Config)
	if err != nil {
		common.ApiErrorMsg(c, "微信支付配置无效")
		return
	}
	wechatOrder, err := client.QueryOrderByOutTradeNo(c.Request.Context(), tradeNo)
	if err != nil {
		common.ApiErrorMsg(c, "查询微信支付状态失败")
		return
	}
	localStatus, err := validateWechatQueriedOrder(wechatOrder, tradeNo, wechatSettings.Config)
	if err != nil {
		common.ApiErrorMsg(c, "微信支付订单校验失败")
		return
	}
	if localStatus == common.TopUpStatusPending && wechatOrder.TradeState == "NOTPAY" &&
		paymentExpireTime > 0 && common.GetTimestamp() >= paymentExpireTime {
		if err := client.CloseOrder(c.Request.Context(), tradeNo); err != nil {
			common.ApiErrorMsg(c, "微信支付订单关单失败，请稍后重试")
			return
		}
		localStatus = common.TopUpStatusExpired
		wechatOrder.TradeState = "CLOSED"
		wechatOrder.TradeStateDesc = "CLOSED"
	}
	if localStatus == common.TopUpStatusSuccess {
		if err := settleWechatOrder(wechatOrder.OutTradeNo, wechatOrder.Amount.Total, wechatOrder.Amount.Currency, "query", common.GetClientIP(c)); err != nil {
			if errors.Is(err, model.ErrSubscriptionPurchaseLimit) {
				common.ApiErrorMsg(c, "支付已成功，订阅正在等待管理员处理")
				return
			}
			common.ApiErrorMsg(c, "支付成功但订单处理失败，请联系管理员")
			return
		}
		common.ApiSuccess(c, wechatOrderResult{TradeNo: tradeNo, Status: common.TopUpStatusSuccess, TradeState: wechatOrder.TradeState, TradeStateDesc: wechatOrder.TradeStateDesc, Amount: decimal.NewFromInt(wechatOrder.Amount.Total).Shift(-2).StringFixed(2), Currency: wechatOrder.Amount.Currency})
		return
	}
	if localStatus != common.TopUpStatusPending {
		transitionErr := updateWechatOrderTerminalStatus(tradeNo, localStatus)
		if transitionErr != nil {
			common.ApiErrorMsg(c, "更新微信支付订单状态失败")
			return
		}
	}
	common.ApiSuccess(c, wechatOrderResult{TradeNo: tradeNo, Status: localStatus, TradeState: wechatOrder.TradeState, TradeStateDesc: wechatOrder.TradeStateDesc})
}

func isWechatTradeNoValue(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validateWechatQueriedOrder(order *service.WechatOrder, tradeNo string, config service.WechatPayConfig) (string, error) {
	if order == nil || order.OutTradeNo != tradeNo || order.MchID != config.MchID || order.AppID != config.AppID {
		return "", errors.New("wechat order identity mismatch")
	}
	var localStatus string
	switch order.TradeState {
	case "SUCCESS":
		localStatus = common.TopUpStatusSuccess
	case "NOTPAY", "USERPAYING":
		localStatus = common.TopUpStatusPending
	case "CLOSED", "REVOKED":
		localStatus = common.TopUpStatusExpired
	case "PAYERROR", "REFUND":
		localStatus = common.TopUpStatusFailed
	default:
		return "", errors.New("unsupported wechat trade state")
	}
	if localStatus == common.TopUpStatusSuccess &&
		(order.TransactionID == "" || len(order.TransactionID) > 32 || order.Amount.Total <= 0 ||
			order.Amount.Total > wechatNativeMaxAmountMinor || order.Amount.Currency != "CNY") {
		return "", errors.New("wechat order amount mismatch")
	}
	return localStatus, nil
}

func settleWechatOrder(tradeNo string, amountMinor int64, currency, source, callerIP string) error {
	if len(tradeNo) < 6 || len(tradeNo) > 32 || !isWechatTradeNoValue(tradeNo) ||
		amountMinor <= 0 || amountMinor > wechatNativeMaxAmountMinor || currency != "CNY" {
		return model.ErrTopUpPaymentMismatch
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	switch tradeNo[0] {
	case 'W':
		return model.RechargeWechatPay(tradeNo, amountMinor, currency, callerIP)
	case 'S':
		payment, err := model.NewSubscriptionPaymentFromMinorUnits(amountMinor, currency)
		if err != nil {
			return model.ErrSubscriptionPaymentMismatch
		}
		payload := fmt.Sprintf("wechat:%s:%s", source, tradeNo)
		return model.CompleteSubscriptionOrder(tradeNo, payload, model.PaymentProviderWechat, model.PaymentMethodWechat, payment)
	default:
		return model.ErrPaymentMethodMismatch
	}
}

func WechatPayNotify(c *gin.Context) {
	wechatSettings := currentWechatRuntimeSettings()
	if !wechatCredentialsPresent(wechatSettings.Config) {
		c.JSON(http.StatusForbidden, gin.H{"code": "FAIL", "message": "微信支付未启用"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentWebhookMaxBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "请求无效"})
		return
	}
	client, err := newWechatClient(wechatSettings.Config)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付回调客户端初始化失败 error=%q", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "服务暂不可用"})
		return
	}
	order, err := client.DecryptNotification(c.Request.Header, body)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付回调验签或解密失败 client_ip=%s error=%q", common.GetClientIP(c), err.Error()))
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "回调校验失败"})
		return
	}
	if err := settleWechatOrder(order.OutTradeNo, order.Amount.Total, order.Amount.Currency, "notify", common.GetClientIP(c)); err != nil {
		if errors.Is(err, model.ErrTopUpNotFound) || errors.Is(err, model.ErrSubscriptionOrderNotFound) || errors.Is(err, model.ErrPaymentMethodMismatch) || errors.Is(err, model.ErrTopUpPaymentMismatch) || errors.Is(err, model.ErrSubscriptionPaymentMismatch) || errors.Is(err, model.ErrTopUpStatusInvalid) || errors.Is(err, model.ErrSubscriptionOrderStatusInvalid) {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付回调订单校验失败 trade_no=%s error=%q", order.OutTradeNo, err.Error()))
			c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "订单校验失败"})
			return
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付回调结算失败 trade_no=%s error=%q", order.OutTradeNo, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "订单处理失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
}
