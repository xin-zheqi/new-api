package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	textcurrency "golang.org/x/text/currency"
	"gorm.io/gorm"
)

type TopUp struct {
	Id                   int     `json:"id"`
	UserId               int     `json:"user_id" gorm:"index"`
	Amount               int64   `json:"amount"`
	Money                float64 `json:"money"`
	TradeNo              string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod        string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider      string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	Source               string  `json:"source" gorm:"type:varchar(32);not null;default:'';index"`
	Currency             string  `json:"currency" gorm:"type:varchar(8);not null;default:''"`
	ExpectedAmountMicros int64   `json:"expected_amount_micros" gorm:"type:bigint;not null;default:0"`
	ExpectedCurrency     string  `json:"expected_currency" gorm:"type:varchar(8);not null;default:''"`
	CreateTime           int64   `json:"create_time" gorm:"index"`
	CompleteTime         int64   `json:"complete_time"`
	Status               string  `json:"status"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
	PaymentMethodManual       = "manual"
)

const (
	TopUpSourceRecharge     = "recharge"
	TopUpSourceSubscription = "subscription"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
	PaymentProviderManual       = "manual"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpPaymentMismatch  = errors.New("topup payment amount or currency mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
	ErrTopUpQuotaOverflow    = errors.New("topup would exceed maximum user quota")
)

type ManualTopUpParams struct {
	UserId        int
	Amount        int64
	Money         float64
	PaymentMethod string
	CreateTime    int64
	CallerIp      string
	CreditBalance bool
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) BeforeSave(tx *gorm.DB) error {
	if topUp.Source == "" {
		topUp.Source = TopUpSourceRecharge
	}
	if topUp.Currency != "" {
		currency, err := NormalizePaymentCurrency(topUp.Currency)
		if err != nil {
			return err
		}
		topUp.Currency = currency
	}
	if topUp.ExpectedAmountMicros < 0 {
		return errors.New("invalid expected topup amount")
	}
	if topUp.ExpectedCurrency != "" {
		currency, err := NormalizePaymentCurrency(topUp.ExpectedCurrency)
		if err != nil {
			return err
		}
		topUp.ExpectedCurrency = currency
	}
	if topUp.ExpectedAmountMicros > 0 && topUp.ExpectedCurrency == "" {
		return errors.New("expected topup currency is required")
	}
	return nil
}

func NormalizePaymentCurrency(currencyCode string) (string, error) {
	currencyCode = strings.ToUpper(strings.TrimSpace(currencyCode))
	unit, err := textcurrency.ParseISO(currencyCode)
	if err != nil || unit.String() != currencyCode {
		return "", errors.New("invalid payment currency")
	}
	// ISO 4217 reserves XXX for transactions without a currency and XTS for
	// testing. Neither can represent money verified by a payment provider.
	if currencyCode == "XXX" || currencyCode == "XTS" {
		return "", errors.New("invalid payment currency")
	}
	return currencyCode, nil
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	topUp, err := FindTopUpByTradeNo(tradeNo)
	if err != nil {
		return nil
	}
	return topUp
}

func FindTopUpByTradeNo(tradeNo string) (*TopUp, error) {
	var topUp TopUp
	if err := DB.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopUpNotFound
		}
		return nil, err
	}
	return &topUp, nil
}

func topUpQuotaFromDecimal(value decimal.Decimal) (int, error) {
	quota, clamp := common.QuotaFromDecimalChecked(value.Truncate(0))
	if clamp != nil {
		return 0, fmt.Errorf("%w: %v", ErrTopUpQuotaOverflow, clamp)
	}
	if quota <= 0 {
		return 0, errors.New("无效的充值额度")
	}
	return quota, nil
}

// Manual administrator credits historically saturated oversized values. Keep
// that compatibility at the input boundary, while creditTopUpQuotaTx still
// enforces the user's actual int32 headroom atomically.
func topUpQuotaFromDecimalSaturating(value decimal.Decimal) (int, error) {
	quota, clamp := common.QuotaFromDecimalChecked(value.Truncate(0))
	if clamp != nil && clamp.Kind != common.QuotaClampOverflow {
		return 0, fmt.Errorf("%w: %v", ErrTopUpQuotaOverflow, clamp)
	}
	if quota <= 0 {
		return 0, errors.New("无效的充值额度")
	}
	return quota, nil
}

// creditTopUpQuotaTx uses an atomic headroom predicate in addition to the
// surrounding row locks. The predicate is required for SQLite, where
// lockForUpdate intentionally emits no unsupported FOR UPDATE clause.
func creditTopUpQuotaTx(tx *gorm.DB, userId int, quota int, updates map[string]interface{}) error {
	if quota <= 0 || quota > common.MaxQuota {
		return ErrTopUpQuotaOverflow
	}
	fields := make(map[string]interface{}, len(updates)+1)
	for key, value := range updates {
		fields[key] = value
	}
	fields["quota"] = gorm.Expr("quota + ?", quota)
	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", userId, common.MaxQuota-quota).
		Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var user User
	if err := lockForUpdate(tx).Select("id", "quota").Where("id = ?", userId).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}
	if user.Quota > common.MaxQuota-quota {
		return ErrTopUpQuotaOverflow
	}
	return errors.New("failed to credit topup quota")
}

func expectedTopUpPayment(topUp *TopUp, paid SubscriptionPaymentSnapshot) (SubscriptionPaymentSnapshot, error) {
	if topUp == nil || paid.AmountMicros <= 0 {
		return SubscriptionPaymentSnapshot{}, ErrTopUpPaymentMismatch
	}
	paidCurrency, err := NormalizePaymentCurrency(paid.Currency)
	if err != nil {
		return SubscriptionPaymentSnapshot{}, ErrTopUpPaymentMismatch
	}
	paid.Currency = paidCurrency

	if topUp.ExpectedAmountMicros > 0 || topUp.ExpectedCurrency != "" {
		expectedCurrency, err := NormalizePaymentCurrency(topUp.ExpectedCurrency)
		if err != nil || topUp.ExpectedAmountMicros <= 0 {
			return SubscriptionPaymentSnapshot{}, ErrTopUpPaymentMismatch
		}
		expected := SubscriptionPaymentSnapshot{AmountMicros: topUp.ExpectedAmountMicros, Currency: expectedCurrency}
		if expected.AmountMicros != paid.AmountMicros || expected.Currency != paid.Currency {
			return SubscriptionPaymentSnapshot{}, ErrTopUpPaymentMismatch
		}
		return expected, nil
	}

	if topUp.Currency != "" {
		legacyCurrency, err := NormalizePaymentCurrency(topUp.Currency)
		if err != nil || legacyCurrency != paid.Currency {
			return SubscriptionPaymentSnapshot{}, ErrTopUpPaymentMismatch
		}
	}
	// Stripe's legacy Money represented the quota credit (and did not include
	// Stripe's configured Price unit), so its checkout total cannot be derived
	// reliably. The callback is still authenticated by Stripe; pin its exact
	// amount and currency as the snapshot when this old order is completed.
	if topUp.PaymentProvider == PaymentProviderStripe {
		return paid, nil
	}
	legacyCurrency := topUp.Currency
	if legacyCurrency == "" {
		switch topUp.PaymentProvider {
		case PaymentProviderEpay:
			legacyCurrency = "CNY"
		case PaymentProviderWaffoPancake:
			legacyCurrency = "USD"
		default:
			legacyCurrency = paid.Currency
		}
	}
	// Money was persisted as a float before the fixed-point snapshot columns
	// existed. Recreate the exact two-decimal checkout amount for old rows;
	// newly-created rows always take the fixed-point branch above.
	expected, err := NewSubscriptionPaymentFromMajorUnits(strconv.FormatFloat(topUp.Money, 'f', 2, 64), legacyCurrency)
	if err != nil || expected.AmountMicros != paid.AmountMicros || expected.Currency != paid.Currency {
		return SubscriptionPaymentSnapshot{}, ErrTopUpPaymentMismatch
	}
	return expected, nil
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == targetStatus {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

// Recharge preserves the historical call shape for integrations that only
// have a currency. New webhook handlers must use RechargeWithPayment so the
// provider-reported amount is checked as well.
func Recharge(referenceId string, customerId string, callerIp string, paidCurrency string) error {
	topUp, err := FindTopUpByTradeNo(referenceId)
	if err != nil {
		return err
	}
	amount, parseErr := NewSubscriptionPaymentFromMajorUnits(strconv.FormatFloat(topUp.Money, 'f', 2, 64), paidCurrency)
	if parseErr != nil {
		return parseErr
	}
	return RechargeWithPayment(referenceId, customerId, callerIp, amount)
}

func RechargeWithPayment(referenceId string, customerId string, callerIp string, paidPayment SubscriptionPaymentSnapshot) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	var credited bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		expectedPayment, err := expectedTopUpPayment(topUp, paidPayment)
		if err != nil {
			return err
		}

		quota, err = topUpQuotaFromDecimal(decimal.NewFromFloat(topUp.Money).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if err != nil {
			return err
		}
		topUp.ExpectedAmountMicros = expectedPayment.AmountMicros
		topUp.ExpectedCurrency = expectedPayment.Currency
		topUp.Currency = paidPayment.Currency
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err = tx.Save(topUp).Error; err != nil {
			return err
		}
		if err := creditTopUpQuotaTx(tx, topUp.UserId, quota, map[string]interface{}{"stripe_customer": customerId}); err != nil {
			return err
		}
		credited = true

		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return fmt.Errorf("stripe topup failed: %w", err)
	}

	if credited {
		if err := cacheIncrUserQuota(topUp.UserId, int64(quota)); err != nil {
			common.SysLog("failed to increase user quota cache after Stripe topup: " + err.Error())
		}
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quota), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)
	}

	return nil
}

// RechargeEpay atomically completes a verified Epay order and credits quota.
// The callback amount is compared with the exact two-decimal amount sent when
// the checkout was created, so a valid signature cannot settle the wrong order
// value because of provider or configuration drift.
func RechargeEpay(tradeNo, actualPaymentMethod, paidAmount, callerIp string) error {
	if tradeNo == "" || actualPaymentMethod == "" {
		return errors.New("未提供支付单号或支付方式")
	}
	paidPayment, err := NewSubscriptionPaymentFromMajorUnits(paidAmount, "CNY")
	if err != nil {
		return ErrTopUpPaymentMismatch
	}

	var topUp TopUp
	var quotaToAdd int
	credited := false
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(&topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if topUp.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		expectedPayment, err := expectedTopUpPayment(&topUp, paidPayment)
		if err != nil {
			return err
		}

		quotaToAdd, err = topUpQuotaFromDecimal(decimal.NewFromInt(topUp.Amount).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if err != nil {
			return err
		}
		topUp.PaymentMethod = actualPaymentMethod
		topUp.ExpectedAmountMicros = expectedPayment.AmountMicros
		topUp.ExpectedCurrency = expectedPayment.Currency
		topUp.Currency = paidPayment.Currency
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(&topUp).Error; err != nil {
			return err
		}
		if err := creditTopUpQuotaTx(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}
		credited = true
		return nil
	})
	if err != nil {
		return err
	}
	if !credited {
		return nil
	}
	if err := cacheIncrUserQuota(topUp.UserId, int64(quotaToAdd)); err != nil {
		common.SysLog("failed to increase user quota cache after Epay topup: " + err.Error())
	}
	RecordTopupLog(topUp.UserId,
		fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%.2f", logger.FormatQuota(quotaToAdd), topUp.Money),
		callerIp, topUp.PaymentMethod, PaymentProviderEpay)
	return nil
}

type TopUpQueryFilter struct {
	StartTime int64
	EndTime   int64
	Status    string
}

func applyTopUpQueryFilter(query *gorm.DB, filter TopUpQueryFilter) *gorm.DB {
	if filter.StartTime > 0 {
		query = query.Where("create_time >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("create_time <= ?", filter.EndTime)
	}
	if strings.TrimSpace(filter.Status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(filter.Status))
	}
	return query
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo, filter TopUpQueryFilter) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	query := applyTopUpQueryFilter(tx.Model(&TopUp{}).Where("user_id = ?", userId), filter)
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = query.Order("create_time desc, id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用）
func GetAllTopUps(pageInfo *common.PageInfo, filter TopUpQueryFilter) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := applyTopUpQueryFilter(tx.Model(&TopUp{}), filter)
	if err = query.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = query.Order("create_time desc, id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo, filter TopUpQueryFilter) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := applyTopUpQueryFilter(tx.Model(&TopUp{}).Where("user_id = ?", userId), filter)
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("create_time desc, id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo, filter TopUpQueryFilter) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := applyTopUpQueryFilter(tx.Model(&TopUp{}), filter)
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("create_time desc, id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string
	credited := false

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		if topUp.PaymentProvider == PaymentProviderStripe {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			var quotaErr error
			quotaToAdd, quotaErr = topUpQuotaFromDecimal(decimal.NewFromFloat(topUp.Money).Mul(dQuotaPerUnit))
			if quotaErr != nil {
				return quotaErr
			}
		} else {
			dAmount := decimal.NewFromInt(topUp.Amount)
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			var quotaErr error
			quotaToAdd, quotaErr = topUpQuotaFromDecimal(dAmount.Mul(dQuotaPerUnit))
			if quotaErr != nil {
				return quotaErr
			}
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := creditTopUpQuotaTx(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		credited = true
		return nil
	})

	if err != nil {
		return err
	}

	if credited {
		if err := cacheIncrUserQuota(userId, int64(quotaToAdd)); err != nil {
			common.SysLog("failed to increase user quota cache after manual completion: " + err.Error())
		}
		RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	}
	return nil
}

// CreateManualTopUp creates a completed recharge record, optionally crediting the user.
func CreateManualTopUp(params ManualTopUpParams) (*TopUp, error) {
	if params.UserId <= 0 {
		return nil, errors.New("用户不存在")
	}
	if params.Amount <= 0 {
		return nil, errors.New("充值额度必须大于 0")
	}
	if params.Money < 0 {
		return nil, errors.New("支付金额不能为负数")
	}
	if params.PaymentMethod == "" {
		return nil, errors.New("支付方式不能为空")
	}
	if params.CreateTime <= 0 {
		return nil, errors.New("创建时间无效")
	}

	tradeNo := strconv.FormatInt(common.NextSnowflakeID(), 10)
	quotaToAdd := 0
	if params.CreditBalance {
		var quotaErr error
		quotaToAdd, quotaErr = topUpQuotaFromDecimalSaturating(decimal.NewFromInt(params.Amount).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if quotaErr != nil {
			return nil, quotaErr
		}
	}

	topUp := &TopUp{
		UserId:          params.UserId,
		Amount:          params.Amount,
		Money:           params.Money,
		TradeNo:         tradeNo,
		PaymentMethod:   params.PaymentMethod,
		PaymentProvider: PaymentProviderManual,
		CreateTime:      params.CreateTime,
		CompleteTime:    params.CreateTime,
		Status:          common.TopUpStatusSuccess,
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Select("id").Where("id = ?", params.UserId).First(&user).Error; err != nil {
			return errors.New("用户不存在")
		}
		if err := tx.Create(topUp).Error; err != nil {
			return err
		}
		if params.CreditBalance {
			if err := creditTopUpQuotaTx(tx, params.UserId, quotaToAdd, nil); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if params.CreditBalance {
		if err := cacheIncrUserQuota(params.UserId, int64(quotaToAdd)); err != nil {
			common.SysLog("failed to increase user quota cache: " + err.Error())
		}
		RecordTopupLog(params.UserId, fmt.Sprintf("超级管理员创建充值记录成功，充值金额: %v，支付金额：%.2f", logger.FormatQuota(quotaToAdd), params.Money), params.CallerIp, params.PaymentMethod, PaymentProviderManual)
	} else {
		RecordLog(params.UserId, LogTypeManage, fmt.Sprintf("超级管理员补录充值记录，未增加余额，充值额度: %d，支付金额：%.2f", params.Amount, params.Money))
	}
	return topUp, nil
}

func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string, paidCurrency string) error {
	topUp, err := FindTopUpByTradeNo(referenceId)
	if err != nil {
		return err
	}
	paidPayment, parseErr := NewSubscriptionPaymentFromMajorUnits(strconv.FormatFloat(topUp.Money, 'f', 2, 64), paidCurrency)
	if parseErr != nil {
		return parseErr
	}
	return RechargeCreemWithPayment(referenceId, customerEmail, customerName, callerIp, paidPayment)
}

func RechargeCreemWithPayment(referenceId string, customerEmail string, customerName string, callerIp string, paidPayment SubscriptionPaymentSnapshot) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	var credited bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		expectedPayment, err := expectedTopUpPayment(topUp, paidPayment)
		if err != nil {
			return err
		}

		quota, err = topUpQuotaFromDecimal(decimal.NewFromInt(topUp.Amount))
		if err != nil {
			return err
		}
		topUp.ExpectedAmountMicros = expectedPayment.AmountMicros
		topUp.ExpectedCurrency = expectedPayment.Currency
		topUp.Currency = expectedPayment.Currency
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		if err := creditTopUpQuotaTx(tx, topUp.UserId, quota, updateFields); err != nil {
			return err
		}
		credited = true

		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return fmt.Errorf("creem topup failed: %w", err)
	}

	if credited {
		if err := cacheIncrUserQuota(topUp.UserId, int64(quota)); err != nil {
			common.SysLog("failed to increase user quota cache after Creem topup: " + err.Error())
		}
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)
	}

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	topUp, lookupErr := FindTopUpByTradeNo(tradeNo)
	if lookupErr != nil {
		return lookupErr
	}
	currency := topUp.Currency
	if currency == "" {
		currency = "USD"
	}
	paidPayment, parseErr := NewSubscriptionPaymentFromMajorUnits(strconv.FormatFloat(topUp.Money, 'f', 2, 64), currency)
	if parseErr != nil {
		return parseErr
	}
	return RechargeWaffoWithPayment(tradeNo, callerIp, paidPayment)
}

func RechargeWaffoWithPayment(tradeNo string, callerIp string, paidPayment SubscriptionPaymentSnapshot) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	credited := false
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		expectedPayment, err := expectedTopUpPayment(topUp, paidPayment)
		if err != nil {
			return err
		}

		quotaToAdd, err = topUpQuotaFromDecimal(decimal.NewFromInt(topUp.Amount).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if err != nil {
			return err
		}
		topUp.ExpectedAmountMicros = expectedPayment.AmountMicros
		topUp.ExpectedCurrency = expectedPayment.Currency
		topUp.Currency = expectedPayment.Currency

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := creditTopUpQuotaTx(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}
		credited = true

		return nil
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return fmt.Errorf("waffo topup failed: %w", err)
	}

	if credited {
		if err := cacheIncrUserQuota(topUp.UserId, int64(quotaToAdd)); err != nil {
			common.SysLog("failed to increase user quota cache after Waffo topup: " + err.Error())
		}
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string, paidCurrency string) error {
	topUp, lookupErr := FindTopUpByTradeNo(tradeNo)
	if lookupErr != nil {
		return lookupErr
	}
	paidPayment, parseErr := NewSubscriptionPaymentFromMajorUnits(strconv.FormatFloat(topUp.Money, 'f', 2, 64), paidCurrency)
	if parseErr != nil {
		return parseErr
	}
	return RechargeWaffoPancakeWithPayment(tradeNo, paidPayment)
}

func RechargeWaffoPancakeWithPayment(tradeNo string, paidPayment SubscriptionPaymentSnapshot) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	credited := false
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		expectedPayment, err := expectedTopUpPayment(topUp, paidPayment)
		if err != nil {
			return err
		}

		quotaToAdd, err = topUpQuotaFromDecimal(decimal.NewFromInt(topUp.Amount).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if err != nil {
			return err
		}
		topUp.ExpectedAmountMicros = expectedPayment.AmountMicros
		topUp.ExpectedCurrency = expectedPayment.Currency
		topUp.Currency = expectedPayment.Currency

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := creditTopUpQuotaTx(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}
		credited = true

		return nil
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return fmt.Errorf("waffo pancake topup failed: %w", err)
	}

	if credited {
		if err := cacheIncrUserQuota(topUp.UserId, int64(quotaToAdd)); err != nil {
			common.SysLog("failed to increase user quota cache after Waffo Pancake topup: " + err.Error())
		}
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
	}

	return nil
}
