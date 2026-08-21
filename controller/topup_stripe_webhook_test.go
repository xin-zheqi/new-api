package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
	"gorm.io/gorm"
)

func setupStripeWebhookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalMainDatabaseType, originalLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stripe-webhook.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(
		&model.Option{}, &model.User{}, &model.Log{}, &model.TopUp{}, &model.SubscriptionPlan{},
		&model.SubscriptionOrder{}, &model.UserSubscription{},
	))
	require.NoError(t, db.Create(&model.Option{Key: "payment_setting.mall_enabled", Value: "false"}).Error)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func enableStripeWebhookForTest(t *testing.T) string {
	t.Helper()
	previousAPISecret := setting.StripeApiSecret
	previousWebhookSecret := setting.StripeWebhookSecret
	previousPriceID := setting.StripePriceId
	paymentSetting := operation_setting.GetPaymentSetting()
	previousPaymentSetting := *paymentSetting
	secret := "whsec_stripe_webhook_test"
	setting.StripeApiSecret = "sk_test_webhook"
	setting.StripeWebhookSecret = secret
	setting.StripePriceId = "price_test_webhook"
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		setting.StripeApiSecret = previousAPISecret
		setting.StripeWebhookSecret = previousWebhookSecret
		setting.StripePriceId = previousPriceID
		*paymentSetting = previousPaymentSetting
	})
	return secret
}

func stripeCheckoutEvent(t *testing.T, eventType stripe.EventType, object map[string]interface{}) stripe.Event {
	t.Helper()
	payload, err := common.Marshal(map[string]interface{}{
		"id": "evt_test", "type": eventType,
		"data": map[string]interface{}{"object": object},
	})
	require.NoError(t, err)
	var event stripe.Event
	require.NoError(t, common.Unmarshal(payload, &event))
	return event
}

func callStripeWebhook(t *testing.T, secret string, eventType stripe.EventType, object map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := common.Marshal(map[string]interface{}{
		"id": "evt_http_test", "type": eventType,
		"data": map[string]interface{}{"object": object},
	})
	require.NoError(t, err)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: secret})
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewReader(signed.Payload))
	context.Request.Header.Set("Stripe-Signature", signed.Header)
	StripeWebhook(context)
	return recorder
}

func TestStripeCompletedTopUpCallbackIsIdempotent(t *testing.T) {
	setupStripeWebhookTestDB(t)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	user := model.User{Username: "stripe-idempotent-user", Password: "password", AffCode: "stripe-idempotent-user"}
	require.NoError(t, model.DB.Create(&user).Error)
	topUp := model.TopUp{
		UserId: user.Id, Amount: 2, Money: 2, TradeNo: "stripe-idempotent-topup",
		PaymentMethod: model.PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe,
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(&topUp).Error)
	event := stripeCheckoutEvent(t, stripe.EventTypeCheckoutSessionCompleted, map[string]interface{}{
		"object": "checkout.session", "client_reference_id": topUp.TradeNo,
		"customer": "cus_test", "status": "complete", "payment_status": "paid",
		"amount_total": 200, "currency": "usd",
	})

	require.NoError(t, sessionCompleted(context.Background(), event, "127.0.0.1"))
	require.NoError(t, sessionCompleted(context.Background(), event, "127.0.0.1"))

	var storedUser model.User
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, 200, storedUser.Quota)
	var storedTopUp model.TopUp
	require.NoError(t, model.DB.First(&storedTopUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, storedTopUp.Status)
	assert.Equal(t, model.TopUpSourceRecharge, storedTopUp.Source)
	assert.Equal(t, "USD", storedTopUp.Currency)
}

func TestStripeFailedAndExpiredCallbacksAreIdempotent(t *testing.T) {
	tests := []struct {
		name      string
		eventType stripe.EventType
		status    string
		handle    func(context.Context, stripe.Event) error
		target    string
	}{
		{
			name: "async failed", eventType: stripe.EventTypeCheckoutSessionAsyncPaymentFailed,
			handle: func(ctx context.Context, event stripe.Event) error {
				return sessionAsyncPaymentFailed(ctx, event, "127.0.0.1")
			},
			target: common.TopUpStatusFailed,
		},
		{
			name: "expired", eventType: stripe.EventTypeCheckoutSessionExpired, status: "expired",
			handle: sessionExpired, target: common.TopUpStatusExpired,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupStripeWebhookTestDB(t)
			user := model.User{Username: "stripe-terminal-" + strings.ReplaceAll(test.name, " ", "-"), Password: "password", AffCode: "stripe-terminal-" + strings.ReplaceAll(test.name, " ", "-")}
			require.NoError(t, model.DB.Create(&user).Error)
			topUp := model.TopUp{
				UserId: user.Id, Amount: 2, Money: 2, TradeNo: "stripe-terminal-" + strings.ReplaceAll(test.name, " ", "-"),
				PaymentMethod: model.PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe,
				Currency: "USD", Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
			}
			require.NoError(t, model.DB.Create(&topUp).Error)
			event := stripeCheckoutEvent(t, test.eventType, map[string]interface{}{
				"object": "checkout.session", "client_reference_id": topUp.TradeNo, "status": test.status,
			})

			require.NoError(t, test.handle(context.Background(), event))
			require.NoError(t, test.handle(context.Background(), event))
			require.NoError(t, model.DB.First(&topUp, topUp.Id).Error)
			assert.Equal(t, test.target, topUp.Status)
		})
	}
}

func TestStripeFulfillmentPropagatesOrderAndDatabaseFailures(t *testing.T) {
	t.Run("subscription completion failure", func(t *testing.T) {
		setupStripeWebhookTestDB(t)
		user := model.User{Username: "stripe-complete-failure", Password: "password", AffCode: "stripe-complete-failure"}
		require.NoError(t, model.DB.Create(&user).Error)
		plan := model.SubscriptionPlan{Title: "Stripe plan", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, Enabled: true}
		require.NoError(t, model.DB.Create(&plan).Error)
		order := model.SubscriptionOrder{
			UserId: user.Id, PlanId: plan.Id, TradeNo: "stripe-complete-failure",
			PaymentMethod: model.PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe,
			ExpectedAmountMicros: 10_000_000, ExpectedCurrency: "USD",
			Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
		}
		require.NoError(t, model.DB.Create(&order).Error)
		event := stripeCheckoutEvent(t, stripe.EventTypeCheckoutSessionCompleted, map[string]interface{}{
			"object": "checkout.session", "client_reference_id": order.TradeNo,
			"status": "complete", "payment_status": "paid", "amount_total": 100, "currency": "usd",
		})

		err := sessionCompleted(context.Background(), event, "127.0.0.1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "complete subscription order")
	})

	t.Run("database lookup failure", func(t *testing.T) {
		db := setupStripeWebhookTestDB(t)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		event := stripeCheckoutEvent(t, stripe.EventTypeCheckoutSessionCompleted, map[string]interface{}{
			"object": "checkout.session", "client_reference_id": "db-failure",
			"status": "complete", "payment_status": "paid", "amount_total": 100, "currency": "usd",
		})

		err = sessionCompleted(context.Background(), event, "127.0.0.1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lookup subscription order")
		assert.NotContains(t, err.Error(), "recharge order")
	})
}

func TestStripeWebhookReturns5xxWhenVerifiedEventCannotReachDatabase(t *testing.T) {
	db := setupStripeWebhookTestDB(t)
	secret := enableStripeWebhookForTest(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	recorder := callStripeWebhook(t, secret, stripe.EventTypeCheckoutSessionCompleted, map[string]interface{}{
		"object": "checkout.session", "client_reference_id": "db-failure-http",
		"status": "complete", "payment_status": "paid", "amount_total": 100, "currency": "usd",
	})
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}
