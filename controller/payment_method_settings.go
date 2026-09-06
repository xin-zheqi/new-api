package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// paymentMethodEnabled is the single server-side gate shared by every new
// payment-order endpoint. An absent override remains enabled for backwards
// compatibility; only an explicit false disables a method.
func paymentMethodEnabled(paymentType string) bool {
	_, enabled := operation_setting.ResolvePaymentMethod(paymentType, "")
	return enabled
}

// requirePaymentMethodEnabled rejects direct requests for a method disabled
// in the global payment-method overlay. Hiding a button in the UI alone would
// leave a trivial API bypass, so all checkout handlers call this helper.
func requirePaymentMethodEnabled(c *gin.Context, paymentType string) bool {
	if paymentMethodEnabled(paymentType) {
		return true
	}
	common.ApiErrorMsg(c, "该支付方式已被管理员停用")
	return false
}

func applyPaymentMethodSetting(method map[string]string) bool {
	typ := strings.TrimSpace(method["type"])
	if typ == "" {
		return false
	}
	method["type"] = typ
	name, enabled := operation_setting.ResolvePaymentMethod(typ, method["name"])
	if !enabled {
		return false
	}
	if name != "" {
		method["name"] = name
	}
	return true
}
