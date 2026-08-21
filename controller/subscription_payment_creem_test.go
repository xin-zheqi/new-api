package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

type creemRoundTripFunc func(*http.Request) (*http.Response, error)

func (function creemRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestSubscriptionCreemOrderUsesSystemCurrencyAndSnapshottedProduct(t *testing.T) {
	db := setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)

	paymentSetting := operation_setting.GetPaymentSetting()
	originalPaymentSetting := *paymentSetting
	originalAPIKey := setting.CreemApiKey
	originalWebhookSecret := setting.CreemWebhookSecret
	originalTestMode := setting.CreemTestMode
	originalTransport := http.DefaultTransport
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	setting.CreemApiKey = "creem_test_api_key"
	setting.CreemWebhookSecret = "creem_test_webhook_secret"
	setting.CreemTestMode = true
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	http.DefaultTransport = creemRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "https://test-api.creem.io/v1/checkouts", request.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"checkout_url":"https://checkout.example.test/order","id":"checkout_test"}`)),
			Request:    request,
		}, nil
	})
	t.Cleanup(func() {
		*paymentSetting = originalPaymentSetting
		setting.CreemApiKey = originalAPIKey
		setting.CreemWebhookSecret = originalWebhookSecret
		setting.CreemTestMode = originalTestMode
		http.DefaultTransport = originalTransport
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
	})

	user := model.User{
		Username: "creem-currency-user", Password: "password", Email: "creem@example.test",
		AffCode: "creem-currency-user",
	}
	require.NoError(t, db.Create(&user).Error)
	plan := model.SubscriptionPlan{
		Title: "EUR Creem plan", PriceAmount: 9.99, Currency: "EUR",
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: 1000, InvoiceEligible: true, CreemProductId: "prod_eur_test",
	}
	require.NoError(t, db.Create(&plan).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", user.Id)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/subscription/creem/pay",
		strings.NewReader(`{"plan_id":`+strconv.Itoa(plan.Id)+`}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	SubscriptionRequestCreemPay(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	var order model.SubscriptionOrder
	require.NoError(t, db.Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).First(&order).Error)
	assert.Equal(t, int64(9_990_000), order.ExpectedAmountMicros)
	assert.Equal(t, "CNY", order.ExpectedCurrency)
	assert.NotEmpty(t, order.PlanSnapshot)

	err := model.CompleteSubscriptionOrder(
		order.TradeNo, `{}`, model.PaymentProviderCreem, "",
		model.SubscriptionPaymentSnapshot{AmountMicros: 9_990_000, Currency: "CNY", ProductId: "prod_wrong"},
	)
	require.ErrorIs(t, err, model.ErrSubscriptionPaymentMismatch)
	var subscriptionCount int64
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)

	require.NoError(t, model.CompleteSubscriptionOrder(
		order.TradeNo, `{}`, model.PaymentProviderCreem, "",
		model.SubscriptionPaymentSnapshot{AmountMicros: 8_990_000, Currency: "EUR", ProductId: plan.CreemProductId},
	))
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
	var completedOrder model.SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", order.TradeNo).First(&completedOrder).Error)
	assert.Equal(t, int64(8_990_000), completedOrder.PaidAmountMicros)
	assert.Equal(t, "EUR", completedOrder.PaidCurrency)
}

func TestCreemSubscriptionWebhookExtractsAndValidatesProductId(t *testing.T) {
	db := setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	user := model.User{Username: "creem-product-user", Password: "password", AffCode: "creem-product-user"}
	require.NoError(t, db.Create(&user).Error)
	plan := model.SubscriptionPlan{
		Title: "Creem product plan", PriceAmount: 9.99, Currency: "USD",
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: 1000, CreemProductId: "prod_expected",
	}
	require.NoError(t, db.Create(&plan).Error)
	order := model.SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: plan.PriceAmount, TradeNo: "creem-product-webhook-order",
		PaymentMethod: model.PaymentMethodCreem, PaymentProvider: model.PaymentProviderCreem,
		Status: common.TopUpStatusPending, ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
	}
	require.NoError(t, model.CreatePendingSubscriptionOrder(&order, &plan))

	event := CreemWebhookEvent{Id: "event_product_check", EventType: "checkout.completed"}
	event.Object.RequestId = order.TradeNo
	event.Object.Order.Id = "creem_order_product_check"
	event.Object.Order.Status = "paid"
	event.Object.Order.AmountPaid = 999
	event.Object.Order.Currency = "CNY"
	event.Object.Order.Product = "prod_wrong"
	event.Object.Product.Id = "prod_wrong"

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook", nil)
	handleCheckoutCompleted(context, &event)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var subscriptionCount int64
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)

	event.Object.Order.Product = "prod_expected"
	event.Object.Product.Id = "prod_expected"
	recorder = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook", nil)
	handleCheckoutCompleted(context, &event)
	assert.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
}

func TestSubscriptionPaymentRequestsLimitJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paymentSetting := operation_setting.GetPaymentSetting()
	originalPaymentSetting := *paymentSetting
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() { *paymentSetting = originalPaymentSetting })

	handlers := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "balance", handler: SubscriptionRequestBalancePay},
		{name: "epay", handler: SubscriptionRequestEpay},
		{name: "stripe", handler: SubscriptionRequestStripePay},
		{name: "creem", handler: SubscriptionRequestCreemPay},
		{name: "waffo pancake", handler: SubscriptionRequestWaffoPancakePay},
	}
	body := `{"plan_id":1,"padding":"` + strings.Repeat("a", int(paymentRequestMaxBytes*2)) + `"}`
	for _, test := range handlers {
		t.Run(test.name, func(t *testing.T) {
			reader := strings.NewReader(body)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/subscription/pay", reader)
			context.Request.Header.Set("Content-Type", "application/json")

			test.handler(context)

			assert.Positive(t, reader.Len(), "handler consumed the entire oversized request")
			assert.LessOrEqual(t, len(body)-reader.Len(), int(paymentRequestMaxBytes)+1)
		})
	}
}
