package setting

import "math"

const (
	// DefaultWechatMinTopUp is used when an existing installation has not yet
	// stored a dedicated WeChat minimum. The controller may provide the legacy
	// Epay minimum as a fallback before reaching this default.
	DefaultWechatMinTopUp = 1
	// MaxWechatMinTopUp keeps the configured USD amount within the same range
	// accepted by the wallet top-up validator.
	MaxWechatMinTopUp = 10_000
	// DefaultWechatUnitPrice is used when an existing installation has not yet
	// stored the dedicated WeChat exchange rate. Keeping this fallback tied to
	// the legacy Epay price is handled by EffectiveWechatUnitPrice.
	DefaultWechatUnitPrice = 7.3
	// MaxWechatUnitPrice prevents malformed configuration from producing an
	// unreasonably large CNY amount or overflowing the WeChat minor-unit limit.
	MaxWechatUnitPrice = 1_000_000
)

// WeChat Pay Native (API v3) configuration. Sensitive values are kept in the
// option store but are never returned by the public option endpoint.
var (
	WechatEnabled                 bool
	WechatMchID                   string
	WechatAppID                   string
	WechatApiV3Key                string
	WechatCertificateSerialNumber string
	WechatPrivateKey              string
	WechatMerchantCertificate     string
	WechatPayPublicKeyID          string
	WechatPayPublicKey            string
	WechatPlatformCertificate     string
	WechatNotifyURL               string
	WechatOrderExpireMinutes      int = 120
	// WechatUnitPrice is the CNY amount charged for one USD. A zero value means
	// the option was not explicitly configured and callers should use the
	// legacy payment price as a fallback.
	WechatUnitPrice float64
	// WechatMinTopUp is the minimum wallet amount in USD. Zero means the option
	// has not been explicitly configured and callers should use their fallback.
	WechatMinTopUp int
)

// EffectiveWechatMinTopUp returns a valid USD minimum while preserving the
// legacy Epay minimum for databases created before this option existed.
func EffectiveWechatMinTopUp(fallback int) int {
	if isValidWechatMinTopUp(WechatMinTopUp) {
		return WechatMinTopUp
	}
	if isValidWechatMinTopUp(fallback) {
		return fallback
	}
	return DefaultWechatMinTopUp
}

func isValidWechatMinTopUp(value int) bool {
	return value >= DefaultWechatMinTopUp && value <= MaxWechatMinTopUp
}

// EffectiveWechatUnitPrice returns a finite positive rate for runtime use.
// The fallback argument is normally operation_setting.Price, preserving the
// behavior of installations created before the dedicated option existed.
func EffectiveWechatUnitPrice(fallback float64) float64 {
	if isValidWechatUnitPrice(WechatUnitPrice) {
		return WechatUnitPrice
	}
	if isValidWechatUnitPrice(fallback) {
		return fallback
	}
	return DefaultWechatUnitPrice
}

func isValidWechatUnitPrice(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && value <= MaxWechatUnitPrice
}
