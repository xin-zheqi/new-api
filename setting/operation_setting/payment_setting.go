package operation_setting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type PaymentSetting struct {
	AmountOptions  []int           `json:"amount_options"`
	AmountDiscount map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠
	MallEnabled    bool            `json:"mall_enabled"`

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:  []int{10, 20, 50, 100, 200, 500},
	AmountDiscount: map[int]float64{},
	MallEnabled:    false,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func ValidatePaymentAmountOptions(value string) error {
	if common.GetJsonType(json.RawMessage(value)) != "array" {
		return errors.New("payment amount options must be a JSON array")
	}
	var amounts []int
	if err := common.UnmarshalJsonStr(value, &amounts); err != nil {
		return fmt.Errorf("invalid payment amount options: %w", err)
	}
	for _, amount := range amounts {
		if amount <= 0 {
			return errors.New("payment amount options must contain only positive integers")
		}
	}
	return nil
}

func ValidatePaymentAmountDiscount(value string) error {
	if common.GetJsonType(json.RawMessage(value)) != "object" {
		return errors.New("payment amount discount must be a JSON object")
	}
	// Decode into the same key type used by PaymentSetting so validation and
	// runtime publication cannot disagree on inputs such as whitespace keys.
	var discounts map[int]float64
	if err := common.UnmarshalJsonStr(value, &discounts); err != nil {
		return fmt.Errorf("invalid payment amount discount: %w", err)
	}
	for amount, discount := range discounts {
		if amount <= 0 {
			return fmt.Errorf("payment amount discount key %d must be a positive integer", amount)
		}
		if discount <= 0 || math.IsNaN(discount) || math.IsInf(discount, 0) {
			return fmt.Errorf("payment amount discount for %d must be a finite positive number", amount)
		}
	}
	return nil
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
