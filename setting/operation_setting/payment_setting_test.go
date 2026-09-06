package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePaymentMethodSettings(t *testing.T) {
	testCases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty overlay", value: ""},
		{name: "valid entries", value: `[{"type":"stripe","name":"Stripe","enabled":false},{"type":"custom_1","name":"自定义"}]`},
		{name: "not an array", value: `{"type":"stripe","name":"Stripe"}`, wantErr: true},
		{name: "duplicate type", value: `[{"type":"stripe","name":"Stripe"},{"type":" stripe ","name":"Other"}]`, wantErr: true},
		{name: "duplicate normalized Waffo type", value: `[{"type":"waffo","name":"Waffo"},{"type":"waffo:0","name":"Waffo card"}]`, wantErr: true},
		{name: "non printable type", value: `[{"type":"微信","name":"WeChat"}]`, wantErr: true},
		{name: "control character name", value: "[{\"type\":\"stripe\",\"name\":\"Str\\nipe\"}]", wantErr: true},
		{name: "invalid enabled type", value: `[{"type":"stripe","name":"Stripe","enabled":"false"}]`, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidatePaymentMethodSettings(testCase.value)
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestResolvePaymentMethodPreservesLegacyDefaults(t *testing.T) {
	original := paymentSetting.MethodSettings
	t.Cleanup(func() { paymentSetting.MethodSettings = original })

	paymentSetting.MethodSettings = []PaymentMethodSetting{
		{Type: "stripe", Name: "信用卡", Enabled: boolPointer(false)},
		{Type: "alipay", Name: "支付宝扫码", Enabled: boolPointer(true)},
		{Type: "waffo", Name: "Waffo card", Enabled: boolPointer(false)},
	}

	name, enabled := ResolvePaymentMethod("stripe", "Stripe")
	assert.Equal(t, "信用卡", name)
	assert.False(t, enabled)

	name, enabled = ResolvePaymentMethod("alipay", "支付宝")
	assert.Equal(t, "支付宝扫码", name)
	assert.True(t, enabled)

	name, enabled = ResolvePaymentMethod("creem", "Creem")
	assert.Equal(t, "Creem", name)
	assert.True(t, enabled)

	name, enabled = ResolvePaymentMethod("waffo:0", "Waffo card")
	assert.Equal(t, "Waffo card", name)
	assert.False(t, enabled)
}

func TestResolvePaymentMethodDisplayNameSeparatesIdentifiersFromLabels(t *testing.T) {
	originalMethods := PayMethods
	originalSettings := paymentSetting.MethodSettings
	t.Cleanup(func() {
		PayMethods = originalMethods
		paymentSetting.MethodSettings = originalSettings
	})

	PayMethods = []map[string]string{
		{"type": "alipay", "name": "企业支付宝"},
	}
	paymentSetting.MethodSettings = nil

	assert.Equal(t, "WeChat Pay", ResolvePaymentMethodDisplayName("wechat_native_v3"))
	assert.Equal(t, "企业支付宝", ResolvePaymentMethodDisplayName("alipay"))
	assert.Equal(t, "bank_transfer", ResolvePaymentMethodDisplayName("bank_transfer"))

	paymentSetting.MethodSettings = []PaymentMethodSetting{
		{Type: "wechat_native_v3", Name: "微信扫码", Enabled: boolPointer(true)},
	}
	assert.Equal(t, "微信扫码", ResolvePaymentMethodDisplayName("wechat_native_v3"))
}

func TestListPaymentMethodSettingsNormalizesGatewayTypes(t *testing.T) {
	original := paymentSetting.MethodSettings
	t.Cleanup(func() { paymentSetting.MethodSettings = original })

	paymentSetting.MethodSettings = []PaymentMethodSetting{
		{Type: " waffo:0 ", Name: "国际卡", Enabled: boolPointer(false)},
		{Type: "waffo", Name: "重复配置", Enabled: boolPointer(true)},
		{Type: " stripe ", Name: "信用卡", Enabled: boolPointer(true)},
	}

	settings := ListPaymentMethodSettings()
	require.Equal(t, []PaymentMethodSetting{
		{Type: "waffo", Name: "国际卡", Enabled: boolPointer(false)},
		{Type: "stripe", Name: "信用卡", Enabled: boolPointer(true)},
	}, settings)
}

func boolPointer(value bool) *bool {
	return &value
}
