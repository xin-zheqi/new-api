package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreemTopUpPreservesConfiguredFractionalAmount(t *testing.T) {
	database := setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)

	paymentSetting := operation_setting.GetPaymentSetting()
	originalPaymentSetting := *paymentSetting
	originalAPIKey := setting.CreemApiKey
	originalProducts := setting.CreemProducts
	originalWebhookSecret := setting.CreemWebhookSecret
	originalTestMode := setting.CreemTestMode
	originalTransport := http.DefaultTransport
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	setting.CreemApiKey = "creem_amount_test_key"
	setting.CreemProducts = `[{"productId":"prod_kwd","name":"KWD product","price":1.234,"currency":"KWD","quota":1000}]`
	setting.CreemWebhookSecret = "creem_amount_test_secret"
	setting.CreemTestMode = true
	http.DefaultTransport = creemRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"checkout_url":"https://checkout.example.test/order","id":"checkout_amount_test"}`)),
			Request:    request,
		}, nil
	})
	t.Cleanup(func() {
		*paymentSetting = originalPaymentSetting
		setting.CreemApiKey = originalAPIKey
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalWebhookSecret
		setting.CreemTestMode = originalTestMode
		http.DefaultTransport = originalTransport
	})

	user := model.User{
		Username: "creem-amount-user", Password: "password", Email: "creem-amount@example.test",
		AffCode: "creem-amount-user",
	}
	require.NoError(t, database.Create(&user).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", user.Id)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/creem/pay",
		strings.NewReader(`{"product_id":"prod_kwd","payment_method":"creem"}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	RequestCreemPay(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var topUp model.TopUp
	require.NoError(t, database.Where("user_id = ?", user.Id).First(&topUp).Error)
	assert.Equal(t, int64(1_234_000), topUp.ExpectedAmountMicros)
	assert.Equal(t, "KWD", topUp.ExpectedCurrency)
	assert.Equal(t, 1.234, topUp.Money)

	payload := fmt.Sprintf(`{"id":"evt_amount_test","eventType":"checkout.completed","object":{"request_id":%q,"order":{"id":"order_amount_test","amount_paid":1234,"currency":"KWD","status":"paid","type":"onetime"},"product":{"id":"prod_kwd","name":"KWD product"},"customer":{"email":"paid@example.test","name":"Paid User"}}}`, topUp.TradeNo)
	callWebhook := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook", strings.NewReader(payload))
		context.Request.Header.Set("Content-Type", "application/json")
		context.Request.Header.Set(CreemSignatureHeader, generateCreemSignature(payload, setting.CreemWebhookSecret))
		CreemWebhook(context)
		return recorder
	}

	assert.Equal(t, http.StatusOK, callWebhook().Code)
	assert.Equal(t, http.StatusOK, callWebhook().Code)
	require.NoError(t, database.Where("trade_no = ?", topUp.TradeNo).First(&topUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, int64(1_234_000), topUp.ExpectedAmountMicros)
	assert.Equal(t, "KWD", topUp.ExpectedCurrency)
	var paidUser model.User
	require.NoError(t, database.First(&paidUser, user.Id).Error)
	assert.Equal(t, 1000, paidUser.Quota)
}
