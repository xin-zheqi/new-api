package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func isPaymentComplianceConfirmed() bool {
	return operation_setting.IsPaymentComplianceConfirmed()
}

func isStripeTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return strings.TrimSpace(setting.StripeApiSecret) != "" &&
		strings.TrimSpace(setting.StripeWebhookSecret) != "" &&
		strings.TrimSpace(setting.StripePriceId) != ""
}

func isStripeWebhookConfigured() bool {
	return strings.TrimSpace(setting.StripeWebhookSecret) != ""
}

func isStripeWebhookEnabled() bool {
	return isStripeTopUpEnabled()
}

func isCreemTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	products := strings.TrimSpace(setting.CreemProducts)
	return strings.TrimSpace(setting.CreemApiKey) != "" &&
		strings.TrimSpace(setting.CreemWebhookSecret) != "" &&
		products != "" &&
		products != "[]"
}

func isCreemWebhookConfigured() bool {
	return strings.TrimSpace(setting.CreemWebhookSecret) != ""
}

func isCreemWebhookEnabled() bool {
	return isCreemTopUpEnabled() && isCreemWebhookConfigured()
}

func isWaffoTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	if !setting.WaffoEnabled {
		return false
	}

	return isWaffoWebhookConfigured()
}

func isWaffoWebhookConfigured() bool {
	if setting.WaffoSandbox {
		return strings.TrimSpace(setting.WaffoSandboxApiKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPrivateKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPublicCert) != ""
	}

	return strings.TrimSpace(setting.WaffoApiKey) != "" &&
		strings.TrimSpace(setting.WaffoPrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPublicCert) != ""
}

func isWaffoWebhookEnabled() bool {
	return isWaffoTopUpEnabled()
}

func isWaffoPancakeTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// Presence-of-credentials = enabled. Webhook public keys ship inside
	// the SDK; mode (test/prod) is read from each event.
	return strings.TrimSpace(setting.WaffoPancakeMerchantID) != "" &&
		strings.TrimSpace(setting.WaffoPancakePrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPancakeProductID) != ""
}

func isWaffoPancakeWebhookConfigured() bool {
	return isWaffoPancakeTopUpEnabled()
}

func isWaffoPancakeWebhookEnabled() bool {
	return isWaffoPancakeTopUpEnabled()
}

func isEpayTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	if !isEpayWebhookConfigured() {
		return false
	}
	for _, method := range operation_setting.PayMethods {
		if isEpayPaymentMethod(method["type"]) {
			return true
		}
	}
	return false
}

// isEpayPaymentMethod identifies payment types handled by the legacy Epay
// gateway. Dedicated gateways have their own signed request and callback
// paths, so accepting them through /api/user/pay would mix provider state.
func isEpayPaymentMethod(paymentType string) bool {
	switch strings.TrimSpace(paymentType) {
	case "", model.PaymentMethodStripe, model.PaymentMethodCreem,
		model.PaymentMethodWaffo, model.PaymentMethodWaffoPancake,
		model.PaymentMethodWechat:
		return false
	default:
		return true
	}
}

func isEpayWebhookConfigured() bool {
	return strings.TrimSpace(operation_setting.PayAddress) != "" &&
		strings.TrimSpace(operation_setting.EpayId) != "" &&
		strings.TrimSpace(operation_setting.EpayKey) != ""
}

func isEpayWebhookEnabled() bool {
	// Keep callbacks available for already-created orders even after an
	// administrator removes every Epay checkout method. Signature verification
	// still requires the configured merchant credentials.
	return isPaymentComplianceConfirmed() && isEpayWebhookConfigured()
}

// isWechatNotifyURLConfigured validates the callback destination before it is
// sent to WeChat. An empty dedicated URL intentionally falls back to the
// server callback address, matching the other payment integrations.
func isWechatNotifyURLConfigured(settings wechatRuntimeSettings) bool {
	_, err := resolveWechatNotifyURL(settings.NotifyURL)
	return err == nil
}

// isWechatTopUpEnabled is deliberately stricter than a single enable flag:
// exposing a payment method with incomplete credentials only creates orders
// that can never settle and makes configuration mistakes hard to diagnose.
func isWechatTopUpEnabled() bool {
	return isWechatTopUpEnabledForSettings(currentWechatRuntimeSettings())
}

func isWechatTopUpEnabledForSettings(settings wechatRuntimeSettings) bool {
	_, err := newConfiguredWechatClient(settings)
	return err == nil
}

func wechatCredentialsPresent(config service.WechatPayConfig) bool {
	if strings.TrimSpace(config.MchID) == "" ||
		strings.TrimSpace(config.AppID) == "" ||
		len(strings.TrimSpace(config.ApiV3Key)) != 32 ||
		strings.TrimSpace(config.CertificateSerialNumber) == "" ||
		strings.TrimSpace(config.PrivateKey) == "" {
		return false
	}
	hasPayPublicKey := strings.TrimSpace(config.PayPublicKey) != "" && strings.TrimSpace(config.PayPublicKeyID) != ""
	hasPlatformCertificate := strings.TrimSpace(config.PlatformCertificate) != ""
	return hasPayPublicKey || hasPlatformCertificate
}
