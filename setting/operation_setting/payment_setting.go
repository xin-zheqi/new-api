package operation_setting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type PaymentSetting struct {
	AmountOptions              []int           `json:"amount_options"`
	AmountDiscount             map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠
	MallEnabled                bool            `json:"mall_enabled"`
	BalanceSubscriptionEnabled bool            `json:"balance_subscription_enabled"`
	// MethodSettings is an optional overlay for all payment gateways. The
	// legacy PayMethods option remains the source of Epay-specific entries.
	MethodSettings []PaymentMethodSetting `json:"method_settings"`

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

// PaymentMethodSetting controls the user-facing availability and label of a
// payment gateway. It intentionally lives separately from PayMethods so old
// installations keep their Epay JSON unchanged.
type PaymentMethodSetting struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled"`
}

const (
	PaymentMethodSettingsOptionKey = "payment_setting.method_settings"
	maxPaymentMethodSettings       = 64
	maxPaymentMethodSettingTypeLen = 64
	maxPaymentMethodSettingNameLen = 128
	maxPaymentMethodSettingsBytes  = 64 * 1024
)

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:              []int{10, 20, 50, 100, 200, 500},
	AmountDiscount:             map[int]float64{},
	MallEnabled:                false,
	BalanceSubscriptionEnabled: false,
	MethodSettings:             []PaymentMethodSetting{},
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

// GetPaymentMethodSetting returns a copy of the explicit override for type.
// Missing entries intentionally mean enabled with the gateway's default name.
func GetPaymentMethodSetting(paymentType string) (PaymentMethodSetting, bool) {
	paymentType = normalizePaymentMethodType(paymentType)
	if paymentType == "" {
		return PaymentMethodSetting{}, false
	}
	for _, setting := range paymentSetting.MethodSettings {
		if normalizePaymentMethodType(setting.Type) == paymentType {
			return setting, true
		}
	}
	return PaymentMethodSetting{}, false
}

func normalizePaymentMethodType(paymentType string) string {
	paymentType = strings.TrimSpace(paymentType)
	if strings.HasPrefix(paymentType, "waffo:") {
		return "waffo"
	}
	return paymentType
}

// ListPaymentMethodSettings returns a defensive, normalized copy suitable for
// API responses and frontend editors. Missing enabled values are represented as
// true to preserve the historical behavior of hand-written entries.
func ListPaymentMethodSettings() []PaymentMethodSetting {
	result := make([]PaymentMethodSetting, 0, len(paymentSetting.MethodSettings))
	seen := make(map[string]struct{}, len(paymentSetting.MethodSettings))
	for _, item := range paymentSetting.MethodSettings {
		paymentType := normalizePaymentMethodType(item.Type)
		if paymentType == "" {
			continue
		}
		if _, exists := seen[paymentType]; exists {
			continue
		}
		seen[paymentType] = struct{}{}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		result = append(result, PaymentMethodSetting{
			Type:    paymentType,
			Name:    strings.TrimSpace(item.Name),
			Enabled: common.GetPointer(enabled),
		})
	}
	return result
}

// ResolvePaymentMethod returns the configured label and enabled state for a
// gateway. An absent override preserves the existing behavior.
func ResolvePaymentMethod(paymentType, fallbackName string) (string, bool) {
	setting, ok := GetPaymentMethodSetting(paymentType)
	if !ok {
		return fallbackName, true
	}
	name := strings.TrimSpace(setting.Name)
	if name == "" {
		name = fallbackName
	}
	enabled := true
	if setting.Enabled != nil {
		enabled = *setting.Enabled
	}
	return name, enabled
}

// ResolvePaymentMethodDisplayName returns a user-facing name while keeping the
// stable gateway identifier available for routing and historical records.
// Legacy Epay names remain authoritative unless the global overlay overrides
// them; dedicated gateways use canonical names that frontends can translate.
func ResolvePaymentMethodDisplayName(paymentType string) string {
	paymentType = strings.TrimSpace(paymentType)
	if paymentType == "" {
		return ""
	}

	fallbackName := ""
	for _, method := range PayMethods {
		if strings.TrimSpace(method["type"]) == paymentType {
			fallbackName = strings.TrimSpace(method["name"])
			break
		}
	}
	if fallbackName == "" {
		switch normalizePaymentMethodType(paymentType) {
		case "alipay":
			fallbackName = "Alipay"
		case "wxpay", "wechat_native_v3":
			fallbackName = "WeChat Pay"
		case "stripe":
			fallbackName = "Stripe"
		case "creem":
			fallbackName = "Creem"
		case "waffo":
			fallbackName = "Waffo Payment"
		case "waffo_pancake":
			fallbackName = "Waffo Pancake"
		default:
			fallbackName = paymentType
		}
	}

	name, _ := ResolvePaymentMethod(paymentType, fallbackName)
	name = strings.TrimSpace(name)
	if name == "" {
		return fallbackName
	}
	return name
}

// ValidatePaymentMethodSettings validates the JSON overlay before it is
// persisted. Names and types are bounded to keep the option small and safe to
// render, and duplicate types are rejected to make the result deterministic.
func ValidatePaymentMethodSettings(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > maxPaymentMethodSettingsBytes {
		return fmt.Errorf("payment method settings cannot exceed %d bytes", maxPaymentMethodSettingsBytes)
	}
	if common.GetJsonType(json.RawMessage(trimmed)) != "array" {
		return errors.New("payment method settings must be a JSON array")
	}

	var entries []struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Enabled *bool  `json:"enabled"`
	}
	if err := common.UnmarshalJsonStr(trimmed, &entries); err != nil {
		return fmt.Errorf("invalid payment method settings: %w", err)
	}
	if len(entries) > maxPaymentMethodSettings {
		return fmt.Errorf("payment method settings cannot contain more than %d entries", maxPaymentMethodSettings)
	}

	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		paymentType := strings.TrimSpace(entry.Type)
		if paymentType == "" || len(paymentType) > maxPaymentMethodSettingTypeLen {
			return fmt.Errorf("payment method settings entry %d has an invalid type", index+1)
		}
		for _, r := range paymentType {
			if r < 0x21 || r > 0x7e {
				return fmt.Errorf("payment method settings entry %d has an invalid type", index+1)
			}
		}
		canonicalType := normalizePaymentMethodType(paymentType)
		if _, exists := seen[canonicalType]; exists {
			return fmt.Errorf("payment method settings contains duplicate type %q", paymentType)
		}
		seen[canonicalType] = struct{}{}

		name := strings.TrimSpace(entry.Name)
		if name == "" || len([]rune(name)) > maxPaymentMethodSettingNameLen {
			return fmt.Errorf("payment method settings entry %d has an invalid name", index+1)
		}
		for _, r := range name {
			if unicode.IsControl(r) {
				return fmt.Errorf("payment method settings entry %d has an invalid name", index+1)
			}
		}
		if entry.Enabled == nil {
			// Omitted enabled fields are accepted for hand-written legacy values;
			// they retain the historical enabled behavior.
			continue
		}
	}
	return nil
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
