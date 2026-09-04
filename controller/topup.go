package controller

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"golang.org/x/net/idna"
)

const (
	maxMallURLLength             = 2048
	paymentRequestMaxBytes int64 = 64 * 1024
	paymentWebhookMaxBytes int64 = 1024 * 1024
	maxTopUpDisplayAmount  int64 = 10000
)

func isFinitePaymentValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hasValidQuotaPerUnit() bool {
	return common.QuotaPerUnit > 0 && isFinitePaymentValue(common.QuotaPerUnit)
}

// topupPaymentGroupRatio preserves the historical zero-ratio fallback while
// rejecting invalid values before they reach decimal or float arithmetic.
func topupPaymentGroupRatio(group string) (float64, bool) {
	ratio := common.GetTopupGroupRatio(group)
	if ratio == 0 {
		ratio = 1
	}
	if ratio < 0 || !isFinitePaymentValue(ratio) {
		return 0, false
	}
	return ratio, true
}

func topupPaymentDiscount(amount int64) float64 {
	discount := 1.0
	if configured, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok &&
		configured > 0 && isFinitePaymentValue(configured) {
		discount = configured
	}
	return discount
}

func parseTopUpQueryFilter(c *gin.Context) model.TopUpQueryFilter {
	parseTime := func(keys ...string) int64 {
		for _, key := range keys {
			value := strings.TrimSpace(c.Query(key))
			if value == "" {
				continue
			}
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err == nil && parsed > 0 {
				return parsed
			}
		}
		return 0
	}

	return model.TopUpQueryFilter{
		StartTime: parseTime("start_time", "start_timestamp"),
		EndTime:   parseTime("end_time", "end_timestamp"),
		Status:    strings.TrimSpace(c.Query("status")),
	}
}

func GetTopUpInfo(c *gin.Context) {
	complianceConfirmed := operation_setting.IsPaymentComplianceConfirmed()
	mallEnabled, mallURL, err := mallTopUpInfo(c.Request.Host)
	if err != nil {
		common.SysError("failed to load current mall settings: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "payment settings are temporarily unavailable"})
		return
	}

	// 获取支付方式
	payMethods := append([]map[string]string(nil), operation_setting.PayMethods...)
	if !complianceConfirmed {
		payMethods = []map[string]string{}
	}

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if complianceConfirmed && isStripeTopUpEnabled() {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.FormatInt(getStripeMinTopup(), 10),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	// Waffo Pancake is displayed above the standard Waffo gateway.
	enableWaffoPancake := isWaffoPancakeTopUpEnabled()
	if complianceConfirmed && enableWaffoPancake {
		hasWaffoPancake := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffoPancake {
				hasWaffoPancake = true
				break
			}
		}

		if !hasWaffoPancake {
			payMethods = append(payMethods, map[string]string{
				"name":      "Waffo Pancake",
				"type":      model.PaymentMethodWaffoPancake,
				"color":     "rgba(var(--semi-orange-5), 1)",
				"min_topup": strconv.FormatInt(getWaffoPancakeMinTopup(), 10),
			})
		}
	}

	// Official WeChat Pay Native is exposed as a normal payment method so the
	// existing wallet flow can render it alongside Epay methods. The request
	// handler still enforces the provider server-side; this list is only UI
	// capability metadata.
	enableWechat := isWechatTopUpEnabled()
	if complianceConfirmed && enableWechat {
		hasWechat := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWechat {
				hasWechat = true
				break
			}
		}
		if !hasWechat {
			payMethods = append(payMethods, map[string]string{
				"name":      "WeChat Pay",
				"type":      model.PaymentMethodWechat,
				"color":     "#07C160",
				"min_topup": strconv.FormatInt(getMinTopup(), 10),
			})
		}
	}

	// 如果启用了 Waffo 支付，添加到支付方法列表
	enableWaffo := isWaffoTopUpEnabled()
	if complianceConfirmed && enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffo {
				hasWaffo = true
				break
			}
		}

		if !hasWaffo {
			waffoMethod := map[string]string{
				"name":      "Waffo (Global Payment)",
				"type":      model.PaymentMethodWaffo,
				"color":     "rgba(var(--semi-blue-5), 1)",
				"min_topup": strconv.FormatInt(getWaffoMinTopup(), 10),
			}
			payMethods = append(payMethods, waffoMethod)
		}
	}

	data := gin.H{
		"enable_online_topup":              complianceConfirmed && isEpayTopUpEnabled(),
		"enable_stripe_topup":              complianceConfirmed && isStripeTopUpEnabled(),
		"enable_creem_topup":               complianceConfirmed && isCreemTopUpEnabled(),
		"enable_waffo_topup":               complianceConfirmed && enableWaffo,
		"enable_waffo_pancake_topup":       complianceConfirmed && enableWaffoPancake,
		"enable_wechat_topup":              complianceConfirmed && enableWechat,
		"enable_redemption":                complianceConfirmed,
		"payment_compliance_confirmed":     complianceConfirmed,
		"payment_compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"waffo_pay_methods": func() interface{} {
			if complianceConfirmed && enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products":          setting.CreemProducts,
		"pay_methods":             payMethods,
		"min_topup":               getMinTopup(),
		"stripe_min_topup":        getStripeMinTopup(),
		"waffo_min_topup":         getWaffoMinTopup(),
		"waffo_pancake_min_topup": getWaffoPancakeMinTopup(),
		"amount_options":          operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                operation_setting.GetPaymentSetting().AmountDiscount,
		"topup_link":              common.TopUpLink,
		"mall_enabled":            mallEnabled,
		"mall_url":                mallURL,
	}
	common.ApiSuccess(c, data)
}

func canonicalMallHostname(hostname string) string {
	hostname = strings.TrimRight(strings.TrimSpace(hostname), ".")
	if hostname == "" {
		return ""
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return strings.ToLower(ip.String())
	}
	asciiHostname, err := idna.Lookup.ToASCII(hostname)
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.ToLower(asciiHostname), ".")
}

func safeMallURL(value string, applicationHost string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxMallURLLength {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return ""
	}
	parsed.Scheme = "https"
	mallHostname := canonicalMallHostname(parsed.Hostname())
	if mallHostname == "" {
		return ""
	}
	applicationURL, err := url.Parse("//" + strings.TrimSpace(applicationHost))
	if err != nil || applicationURL == nil {
		return ""
	}
	applicationHostname := canonicalMallHostname(applicationURL.Hostname())
	if applicationHostname == "" || mallHostname == applicationHostname {
		return ""
	}
	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort(mallHostname, port)
	} else if strings.Contains(mallHostname, ":") {
		parsed.Host = "[" + mallHostname + "]"
	} else {
		parsed.Host = mallHostname
	}
	return parsed.String()
}

func loadCurrentMallSettings() (bool, string, error) {
	values, err := model.GetOptionValues([]string{"payment_setting.mall_enabled", "MallURL"})
	if err != nil {
		return false, "", fmt.Errorf("load mall settings: %w", err)
	}
	enabledValue, exists := values["payment_setting.mall_enabled"]
	if !exists {
		return false, "", nil
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(enabledValue))
	if err != nil {
		return false, "", fmt.Errorf("invalid mall mode setting: %w", err)
	}
	return enabled, values["MallURL"], nil
}

func mallTopUpInfo(applicationHost string) (bool, string, error) {
	enabled, configuredURL, err := loadCurrentMallSettings()
	if err != nil || !enabled {
		return false, "", err
	}
	mallURL := safeMallURL(configuredURL, applicationHost)
	if mallURL == "" {
		return false, "", fmt.Errorf("mall mode is enabled but MallURL is invalid")
	}
	return true, mallURL, nil
}

func rejectSubscriptionPurchaseWhenMallEnabled(c *gin.Context) bool {
	enabled, _, err := mallTopUpInfo(c.Request.Host)
	if err != nil {
		common.SysError("failed to load mall mode setting: " + err.Error())
		common.ApiErrorMsg(c, "unable to load payment settings")
		return true
	}
	if !enabled {
		return false
	}
	common.ApiErrorI18n(c, i18n.MsgSubscriptionMallModeEnabled)
	return true
}

type EpayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type AmountRequest struct {
	Amount int64 `json:"amount"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func getPayMoneyWithUnitPrice(amount int64, group string, unitPrice float64) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		if !hasValidQuotaPerUnit() {
			return 0
		}
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio, ok := topupPaymentGroupRatio(group)
	if !ok || !isFinitePaymentValue(unitPrice) || unitPrice < 0 {
		return 0
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(unitPrice)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := topupPaymentDiscount(amount)
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)
	result := payMoney.InexactFloat64()
	if result < 0 || !isFinitePaymentValue(result) {
		return 0
	}
	return result
}

func getPayMoney(amount int64, group string) float64 {
	return getPayMoneyWithUnitPrice(amount, group, operation_setting.Price)
}

func getMinTopup() int64 {
	return topUpDisplayLimit(operation_setting.MinTopUp)
}

// topUpDisplayLimit converts a configured monetary limit to the value accepted
// by the API. In token mode the client submits raw tokens, so limits must be
// scaled by QuotaPerUnit before comparing the int64 request value.
func topUpDisplayLimit(value int) int64 {
	if value <= 0 || operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return int64(value)
	}
	if !hasValidQuotaPerUnit() {
		return 0
	}
	converted := decimal.NewFromInt(int64(value)).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	maxInt64 := decimal.NewFromInt(math.MaxInt64)
	if converted.GreaterThan(maxInt64) {
		return math.MaxInt64
	}
	return converted.Ceil().IntPart()
}

func validateTopUpAmount(amount, minimum int64) (bool, string) {
	if amount <= 0 {
		return false, "充值数量必须大于 0"
	}
	if amount < minimum {
		return false, fmt.Sprintf("充值数量不能小于 %d", minimum)
	}
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		if !hasValidQuotaPerUnit() {
			return false, "额度单位配置无效"
		}
		normalized := decimal.NewFromInt(amount).Div(decimal.NewFromFloat(common.QuotaPerUnit))
		if !normalized.Equal(normalized.Truncate(0)) {
			return false, "充值数量必须为完整额度单位"
		}
		if normalized.GreaterThan(decimal.NewFromInt(maxTopUpDisplayAmount)) {
			return false, fmt.Sprintf("充值金额折算后不能大于 %d", maxTopUpDisplayAmount)
		}
		return true, ""
	}
	if amount > maxTopUpDisplayAmount {
		return false, fmt.Sprintf("充值数量不能大于 %d", maxTopUpDisplayAmount)
	}
	return true, ""
}

func getTopUpQuota(amount int64) (int, error) {
	if amount <= 0 || !hasValidQuotaPerUnit() {
		return 0, errors.New("充值数量无效")
	}
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	quota := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		units := quota.Div(quotaPerUnit)
		if !units.Equal(units.Truncate(0)) {
			return 0, errors.New("充值数量必须为完整额度单位")
		}
		if units.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
			return 0, errors.New("充值数量超出系统可表示范围")
		}
		quota = decimal.NewFromInt(units.IntPart()).Mul(quotaPerUnit)
	} else {
		quota = quota.Mul(quotaPerUnit)
	}
	return common.WalletQuotaFromDecimalStrict(quota.Truncate(0))
}

func getMaxTopUpAmount() int64 {
	if !hasValidQuotaPerUnit() {
		return 0
	}
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	maxInt64 := decimal.NewFromInt(math.MaxInt64)
	maxStoredAmount := decimal.NewFromInt(common.MaxWalletQuota).
		Div(quotaPerUnit).
		Floor()
	if maxStoredAmount.LessThanOrEqual(decimal.Zero) {
		return 0
	}
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		candidate := maxStoredAmount.Mul(quotaPerUnit).Floor()
		if candidate.GreaterThan(maxInt64) {
			return math.MaxInt64
		}
		return candidate.IntPart()
	}
	if maxStoredAmount.GreaterThan(maxInt64) {
		return math.MaxInt64
	}
	return maxStoredAmount.IntPart()
}

func validateCreditedQuota(quota decimal.Decimal) (int, error) {
	value, err := common.WalletQuotaFromDecimalStrict(quota.Truncate(0))
	if err != nil {
		return 0, errors.New("充值额度超出系统可表示范围")
	}
	if value <= 0 {
		return 0, errors.New("充值额度必须大于 0")
	}
	return value, nil
}

func validateTopUpQuota(amount int64) (int, error) {
	quota, err := getTopUpQuota(amount)
	if err == nil && quota > 0 {
		return quota, nil
	}
	maxAmount := getMaxTopUpAmount()
	if maxAmount > 0 && amount > maxAmount {
		return 0, fmt.Errorf("单笔充值数量不能大于 %d", maxAmount)
	}
	return 0, errors.New("充值数量无效")
}

func validateCreditedQuotaForUser(c *gin.Context, userId int, quota decimal.Decimal) (int, bool) {
	creditedQuota, err := validateCreditedQuota(quota)
	if err == nil {
		err = model.ValidateTopUpQuotaCapacity(userId, creditedQuota)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return 0, true
	}
	return creditedQuota, false
}

func validateTopUpQuotaForUser(c *gin.Context, userId int, amount int64) (int, bool) {
	creditedQuota, err := validateTopUpQuota(amount)
	if err == nil {
		err = model.ValidateTopUpQuotaCapacity(userId, creditedQuota)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return 0, true
	}
	return creditedQuota, false
}

func RequestEpay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req EpayRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentRequestMaxBytes)
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if valid, message := validateTopUpAmount(req.Amount, getMinTopup()); !valid {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": message})
		return
	}
	id := c.GetInt("id")
	creditedQuota, rejected := validateTopUpQuotaForUser(c, id, req.Amount)
	if rejected {
		return
	}

	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(paymentReturnPath("/usage-logs"))
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 拉起支付失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		CreditedQuota:   int64(creditedQuota),
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderEpay,
		Currency:        "CNY",
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	expectedPayment, paymentErr := model.NewSubscriptionPaymentFromMajorUnits(
		strconv.FormatFloat(payMoney, 'f', 2, 64), topUp.Currency)
	if paymentErr != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 订单金额快照无效 user_id=%d trade_no=%s error=%q", id, tradeNo, paymentErr.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付金额配置错误"})
		return
	}
	topUp.ExpectedAmountMicros = expectedPayment.AmountMicros
	topUp.ExpectedCurrency = expectedPayment.Currency
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 创建充值订单失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值订单创建成功 user_id=%d trade_no=%s payment_method=%s amount=%d money=%.2f", id, tradeNo, req.PaymentMethod, req.Amount, payMoney))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

func EpayNotify(c *gin.Context) {
	if !isEpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, common.GetClientIP(c)))
		_ = writeEpayWebhookResponse(c, false)
		return
	}

	var params map[string]string

	if c.Request.Method == "POST" {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentRequestMaxBytes)
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, common.GetClientIP(c), err.Error()))
			_ = writeEpayWebhookResponse(c, false)
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s parameter_count=%d", c.Request.RequestURI, common.GetClientIP(c), c.Request.Method, len(params)))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, common.GetClientIP(c)))
		_ = writeEpayWebhookResponse(c, false)
		return
	}
	client := GetEpayClient()
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 path=%q client_ip=%s", c.Request.RequestURI, common.GetClientIP(c)))
		err := writeEpayWebhookResponse(c, false)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, common.GetClientIP(c), err.Error()))
		}
		return
	}
	verifyInfo, verifyErr := client.Verify(params)
	if verifyErr != nil || !verifyInfo.VerifyStatus {
		if writeErr := writeEpayWebhookResponse(c, false); writeErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, common.GetClientIP(c), writeErr.Error()))
		}
		if verifyErr != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.RequestURI, common.GetClientIP(c), verifyErr.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.RequestURI, common.GetClientIP(c)))
		}
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签成功 trade_no=%s callback_type=%s trade_status=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, common.GetClientIP(c)))

	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		// 进程内锁只是优化；重复/并发回调的正确性由 RechargeEpay 的
		// 数据库行锁 + 事务内状态校验保证（多实例部署下同样安全）。
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		if err := model.RechargeEpayWithPayment(verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.Money, common.GetClientIP(c)); err != nil {
			switch {
			case errors.Is(err, model.ErrTopUpNotFound):
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调订单不存在 trade_no=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, common.GetClientIP(c)))
			case errors.Is(err, model.ErrPaymentMethodMismatch):
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 订单支付网关不匹配 trade_no=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, common.GetClientIP(c)))
			case errors.Is(err, model.ErrTopUpStatusInvalid):
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 订单状态非法 trade_no=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, common.GetClientIP(c)))
			default:
				logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 充值处理失败 trade_no=%s callback_type=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, common.GetClientIP(c), err.Error()))
			}
			if writeErr := writeEpayWebhookResponse(c, false); writeErr != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, common.GetClientIP(c), writeErr.Error()))
			}
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值成功 trade_no=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, common.GetClientIP(c)))
	} else {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, common.GetClientIP(c)))
	}
	if writeErr := writeEpayWebhookResponse(c, true); writeErr != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, common.GetClientIP(c), writeErr.Error()))
	}
}

func writeEpayWebhookResponse(c *gin.Context, success bool) error {
	if !success {
		c.Status(http.StatusInternalServerError)
	}
	response := "success"
	if !success {
		response = "fail"
	}
	_, err := c.Writer.Write([]byte(response))
	return err
}

func RequestAmount(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req AmountRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, paymentRequestMaxBytes)
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if valid, message := validateTopUpAmount(req.Amount, getMinTopup()); !valid {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": message})
		return
	}
	id := c.GetInt("id")
	if _, rejected := validateTopUpQuotaForUser(c, id, req.Amount); rejected {
		return
	}
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")
	filter := parseTopUpQueryFilter(c)

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo, filter)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo, filter)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")
	filter := parseTopUpQueryFilter(c)

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo, filter)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo, filter)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

type AdminCreateManualTopUpRequest struct {
	UserId          int     `json:"user_id"`
	PaymentMethod   string  `json:"payment_method"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	Currency        string  `json:"currency"`
	CreateTime      int64   `json:"create_time"`
	CreditBalance   bool    `json:"credit_balance"`
	InvoiceAmount   float64 `json:"invoice_amount"`
	InvoiceCurrency string  `json:"invoice_currency"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo, common.GetClientIP(c)); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// AdminCreateManualTopUp 超级管理员手动创建成功充值记录
func AdminCreateManualTopUp(c *gin.Context) {
	var req AdminCreateManualTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	req.PaymentMethod = strings.TrimSpace(req.PaymentMethod)
	if req.UserId <= 0 || req.PaymentMethod == "" || req.Amount <= 0 ||
		!isFinitePaymentValue(req.Money) || req.Money < 0 || req.CreateTime <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if len(req.PaymentMethod) > 50 {
		common.ApiErrorMsg(c, "支付方式不能超过 50 个字符")
		return
	}
	if strings.TrimSpace(req.Currency) == "" {
		req.Currency = "CNY"
	}
	paymentCurrency, normalizeErr := model.NormalizePaymentCurrency(req.Currency)
	if normalizeErr != nil {
		common.ApiErrorMsg(c, "支付币种无效")
		return
	}
	if req.Money > 0 {
		if _, paymentErr := model.NewSubscriptionPaymentFromMajorUnits(
			strconv.FormatFloat(req.Money, 'f', -1, 64), paymentCurrency,
		); paymentErr != nil {
			common.ApiErrorMsg(c, "支付金额无效")
			return
		}
	}
	if math.IsNaN(req.InvoiceAmount) || math.IsInf(req.InvoiceAmount, 0) || req.InvoiceAmount < 0 {
		common.ApiErrorMsg(c, "开票金额无效")
		return
	}
	invoiceAmountMicros := int64(0)
	invoiceCurrency := ""
	if req.InvoiceAmount > 0 {
		invoicePayment, paymentErr := model.NewSubscriptionPaymentFromMajorUnits(
			strconv.FormatFloat(req.InvoiceAmount, 'f', -1, 64), req.InvoiceCurrency,
		)
		if paymentErr != nil {
			common.ApiErrorMsg(c, "开票金额或币种无效")
			return
		}
		invoiceAmountMicros = invoicePayment.AmountMicros
		invoiceCurrency = invoicePayment.Currency
	}

	topUp, err := model.CreateManualTopUp(model.ManualTopUpParams{
		UserId:              req.UserId,
		Amount:              req.Amount,
		Money:               req.Money,
		Currency:            paymentCurrency,
		PaymentMethod:       req.PaymentMethod,
		CreateTime:          req.CreateTime,
		CallerIp:            common.GetClientIP(c),
		CreditBalance:       req.CreditBalance,
		InvoiceAmountMicros: invoiceAmountMicros,
		InvoiceCurrency:     invoiceCurrency,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, topUp)
}
