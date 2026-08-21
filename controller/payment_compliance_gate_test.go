package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopUpPaymentEndpointsRequireComplianceConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payment := operation_setting.GetPaymentSetting()
	original := *payment
	payment.ComplianceConfirmed = false
	payment.ComplianceTermsVersion = ""
	t.Cleanup(func() { *payment = original })

	handlers := []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{name: "epay amount", handler: RequestAmount, body: `{"amount":10}`},
		{name: "epay checkout", handler: RequestEpay, body: `{"amount":10,"payment_method":"alipay"}`},
		{name: "stripe amount", handler: RequestStripeAmount, body: `{"amount":10}`},
		{name: "stripe checkout", handler: RequestStripePay, body: `{"amount":10}`},
		{name: "creem checkout", handler: RequestCreemPay, body: `{"amount":10}`},
		{name: "waffo amount", handler: RequestWaffoAmount, body: `{"amount":10}`},
		{name: "waffo checkout", handler: RequestWaffoPay, body: `{"amount":10}`},
		{name: "waffo pancake amount", handler: RequestWaffoPancakeAmount, body: `{"amount":10}`},
		{name: "waffo pancake checkout", handler: RequestWaffoPancakePay, body: `{"amount":10}`},
	}

	for _, test := range handlers {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup/pay", bytes.NewBufferString(test.body))
			context.Request.Header.Set("Content-Type", "application/json")

			test.handler(context)

			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
		})
	}
}
