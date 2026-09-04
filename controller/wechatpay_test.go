package controller

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	controllerWechatMchID       = "1234567890"
	controllerWechatAppID       = "wx1234567890abcdef"
	controllerWechatAPIv3Key    = "0123456789abcdef0123456789abcdef"
	controllerWechatPublicKeyID = "PUB_KEY_ID_3000000001"
)

type wechatRoundTripFunc func(*http.Request) (*http.Response, error)

func (function wechatRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func resetWechatClientCacheForTest() {
	wechatClientCache.Lock()
	wechatClientCache.initialized = false
	wechatClientCache.config = service.WechatPayConfig{}
	wechatClientCache.client = nil
	wechatClientCache.err = nil
	wechatClientCache.Unlock()
}

func configureWechatNotifyTest(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	originalConfig := wechatConfig()
	originalEnabled := setting.WechatEnabled
	originalNotifyURL := setting.WechatNotifyURL
	originalExpireMinutes := setting.WechatOrderExpireMinutes
	originalUnitPrice := setting.WechatUnitPrice

	merchantPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	platformPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	merchantDER, err := x509.MarshalPKCS8PrivateKey(merchantPrivate)
	require.NoError(t, err)
	platformDER, err := x509.MarshalPKIXPublicKey(&platformPrivate.PublicKey)
	require.NoError(t, err)

	setting.WechatEnabled = true
	setting.WechatMchID = controllerWechatMchID
	setting.WechatAppID = controllerWechatAppID
	setting.WechatApiV3Key = controllerWechatAPIv3Key
	setting.WechatCertificateSerialNumber = "A1B2C3D4"
	setting.WechatPrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: merchantDER}))
	setting.WechatMerchantCertificate = ""
	setting.WechatPayPublicKeyID = controllerWechatPublicKeyID
	setting.WechatPayPublicKey = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: platformDER}))
	setting.WechatPlatformCertificate = ""
	setting.WechatNotifyURL = "https://pay.example.com/api/wechat/notify"
	setting.WechatOrderExpireMinutes = 120
	// Keep existing request fixtures deterministic; conversion behavior is
	// covered separately with an explicit non-default rate.
	setting.WechatUnitPrice = 1
	resetWechatClientCacheForTest()

	t.Cleanup(func() {
		setting.WechatEnabled = originalEnabled
		setting.WechatMchID = originalConfig.MchID
		setting.WechatAppID = originalConfig.AppID
		setting.WechatApiV3Key = originalConfig.ApiV3Key
		setting.WechatCertificateSerialNumber = originalConfig.CertificateSerialNumber
		setting.WechatPrivateKey = originalConfig.PrivateKey
		setting.WechatMerchantCertificate = originalConfig.MerchantCertificate
		setting.WechatPayPublicKeyID = originalConfig.PayPublicKeyID
		setting.WechatPayPublicKey = originalConfig.PayPublicKey
		setting.WechatPlatformCertificate = originalConfig.PlatformCertificate
		setting.WechatNotifyURL = originalNotifyURL
		setting.WechatOrderExpireMinutes = originalExpireMinutes
		setting.WechatUnitPrice = originalUnitPrice
		resetWechatClientCacheForTest()
	})
	return platformPrivate
}

func signedWechatNotifyRequest(t *testing.T, platformPrivate *rsa.PrivateKey, tradeNo string, amountMinor int64) *http.Request {
	t.Helper()
	plaintext, err := common.Marshal(map[string]any{
		"mchid":          controllerWechatMchID,
		"appid":          controllerWechatAppID,
		"out_trade_no":   tradeNo,
		"transaction_id": "4200001234567890123456789012",
		"trade_state":    "SUCCESS",
		"amount": map[string]any{
			"total": amountMinor, "currency": "CNY",
		},
	})
	require.NoError(t, err)
	block, err := aes.NewCipher([]byte(controllerWechatAPIv3Key))
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce := "0123456789ab"
	associatedData := "transaction"
	ciphertext := aead.Seal(nil, []byte(nonce), plaintext, []byte(associatedData))
	body, err := common.Marshal(map[string]any{
		"id":         "notification-controller-test",
		"event_type": "TRANSACTION.SUCCESS",
		"resource": map[string]any{
			"algorithm":       "AEAD_AES_256_GCM",
			"ciphertext":      base64.StdEncoding.EncodeToString(ciphertext),
			"associated_data": associatedData,
			"nonce":           nonce,
			"original_type":   "transaction",
		},
	})
	require.NoError(t, err)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	responseNonce := "controller-notify-nonce"
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s\n", timestamp, responseNonce, body)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, platformPrivate, crypto.SHA256, digest[:])
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/api/wechat/notify", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Wechatpay-Timestamp", timestamp)
	request.Header.Set("Wechatpay-Nonce", responseNonce)
	request.Header.Set("Wechatpay-Serial", controllerWechatPublicKeyID)
	request.Header.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(signature))
	return request
}

func callWechatNotifyTest(t *testing.T, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	WechatPayNotify(context)
	return recorder
}

func installWechatQueryTransport(t *testing.T, platformPrivate *rsa.PrivateKey, tradeNo, tradeState string, amountMinor int64) *int {
	t.Helper()
	requestCount := 0
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = wechatRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/v3/pay/transactions/out-trade-no/"+tradeNo, request.URL.Path)
		assert.Equal(t, controllerWechatMchID, request.URL.Query().Get("mchid"))

		order := service.WechatOrder{
			MchID:          controllerWechatMchID,
			AppID:          controllerWechatAppID,
			OutTradeNo:     tradeNo,
			TransactionID:  "4200001234567890123456789012",
			TradeState:     tradeState,
			TradeStateDesc: tradeState,
		}
		order.Amount.Total = amountMinor
		order.Amount.Currency = "CNY"
		body, err := common.Marshal(order)
		require.NoError(t, err)
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := "controller-query-nonce"
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, body)))
		signature, err := rsa.SignPKCS1v15(rand.Reader, platformPrivate, crypto.SHA256, digest[:])
		require.NoError(t, err)
		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")
		headers.Set("Wechatpay-Timestamp", timestamp)
		headers.Set("Wechatpay-Nonce", nonce)
		headers.Set("Wechatpay-Serial", controllerWechatPublicKeyID)
		headers.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(signature))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    request,
		}, nil
	})
	resetWechatClientCacheForTest()
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
		resetWechatClientCacheForTest()
	})
	return &requestCount
}

type wechatQueryCloseCounts struct {
	query int
	close int
}

type capturedWechatNativeOrder struct {
	AppID       string `json:"appid"`
	MchID       string `json:"mchid"`
	OutTradeNo  string `json:"out_trade_no"`
	TimeExpire  string `json:"time_expire"`
	NotifyURL   string `json:"notify_url"`
	Description string `json:"description"`
	Amount      struct {
		Total    int64  `json:"total"`
		Currency string `json:"currency"`
	} `json:"amount"`
}

func installWechatNativeCreateTransport(t *testing.T, platformPrivate *rsa.PrivateKey) <-chan capturedWechatNativeOrder {
	t.Helper()
	captured := make(chan capturedWechatNativeOrder, 1)
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = wechatRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v3/pay/transactions/native", request.URL.Path)
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		var order capturedWechatNativeOrder
		require.NoError(t, common.Unmarshal(body, &order))
		captured <- order

		responseBody := []byte(`{"code_url":"weixin://wxpay/bizpayurl/up?pr=controller-test"}`)
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := "controller-create-nonce"
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, responseBody)))
		signature, err := rsa.SignPKCS1v15(rand.Reader, platformPrivate, crypto.SHA256, digest[:])
		require.NoError(t, err)
		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")
		headers.Set("Wechatpay-Timestamp", timestamp)
		headers.Set("Wechatpay-Nonce", nonce)
		headers.Set("Wechatpay-Serial", controllerWechatPublicKeyID)
		headers.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(signature))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body:       io.NopCloser(bytes.NewReader(responseBody)),
			Request:    request,
		}, nil
	})
	resetWechatClientCacheForTest()
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
		resetWechatClientCacheForTest()
	})
	return captured
}

func installWechatNativeCreateFailureTransport(t *testing.T, platformPrivate *rsa.PrivateKey, queryResult string) *wechatQueryCloseCounts {
	t.Helper()
	counts := new(wechatQueryCloseCounts)
	tradeNo := ""
	amountMinor := int64(0)
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = wechatRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := []byte(nil)
		statusCode := http.StatusOK
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v3/pay/transactions/native":
			var order capturedWechatNativeOrder
			require.NoError(t, common.DecodeJson(request.Body, &order))
			tradeNo = order.OutTradeNo
			amountMinor = order.Amount.Total
			statusCode = http.StatusInternalServerError
			body = []byte(`{"code":"SYSTEM_ERROR","message":"create failed"}`)
		case request.Method == http.MethodGet:
			counts.query++
			assert.NotEmpty(t, tradeNo)
			assert.Contains(t, request.URL.Path, tradeNo)
			switch queryResult {
			case "ORDER_NOT_EXIST":
				statusCode = http.StatusNotFound
				body = []byte(`{"code":"ORDER_NOT_EXIST","message":"order does not exist"}`)
			case "SYSTEM_ERROR":
				statusCode = http.StatusInternalServerError
				body = []byte(`{"code":"SYSTEM_ERROR","message":"query failed"}`)
			default:
				order := service.WechatOrder{
					MchID: controllerWechatMchID, AppID: controllerWechatAppID,
					OutTradeNo: tradeNo, TradeState: queryResult, TradeStateDesc: queryResult,
				}
				if queryResult == "SUCCESS" {
					order.TransactionID = "4200001234567890123456789012"
					order.Amount.Total = amountMinor
					order.Amount.Currency = "CNY"
				}
				var err error
				body, err = common.Marshal(order)
				require.NoError(t, err)
			}
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/close"):
			counts.close++
			assert.Contains(t, request.URL.Path, tradeNo)
			statusCode = http.StatusNoContent
		default:
			require.FailNow(t, "unexpected WeChat request", request.Method+" "+request.URL.Path)
		}
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := "controller-create-failure"
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, body)))
		signature, err := rsa.SignPKCS1v15(rand.Reader, platformPrivate, crypto.SHA256, digest[:])
		require.NoError(t, err)
		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")
		headers.Set("Wechatpay-Timestamp", timestamp)
		headers.Set("Wechatpay-Nonce", nonce)
		headers.Set("Wechatpay-Serial", controllerWechatPublicKeyID)
		headers.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(signature))
		return &http.Response{
			StatusCode: statusCode,
			Header:     headers,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    request,
		}, nil
	})
	resetWechatClientCacheForTest()
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
		resetWechatClientCacheForTest()
	})
	return counts
}

func installWechatExpiredOrderTransport(t *testing.T, platformPrivate *rsa.PrivateKey, tradeNo string, closeStatus int) *wechatQueryCloseCounts {
	t.Helper()
	counts := new(wechatQueryCloseCounts)
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = wechatRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body []byte
		statusCode := http.StatusOK
		switch request.Method {
		case http.MethodGet:
			counts.query++
			order := service.WechatOrder{
				MchID: controllerWechatMchID, AppID: controllerWechatAppID,
				OutTradeNo: tradeNo, TradeState: "NOTPAY", TradeStateDesc: "NOTPAY",
			}
			var err error
			body, err = common.Marshal(order)
			require.NoError(t, err)
		case http.MethodPost:
			counts.close++
			assert.Equal(t, "/v3/pay/transactions/out-trade-no/"+tradeNo+"/close", request.URL.Path)
			requestBody, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			var payload struct {
				MchID string `json:"mchid"`
			}
			require.NoError(t, common.Unmarshal(requestBody, &payload))
			assert.Equal(t, controllerWechatMchID, payload.MchID)
			statusCode = closeStatus
			if closeStatus != http.StatusNoContent {
				body = []byte(`{"code":"SYSTEM_ERROR","message":"close failed"}`)
			}
		default:
			require.FailNow(t, "unexpected WeChat request method", request.Method)
		}
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := "controller-expiry-nonce"
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, body)))
		signature, err := rsa.SignPKCS1v15(rand.Reader, platformPrivate, crypto.SHA256, digest[:])
		require.NoError(t, err)
		headers := make(http.Header)
		headers.Set("Wechatpay-Timestamp", timestamp)
		headers.Set("Wechatpay-Nonce", nonce)
		headers.Set("Wechatpay-Serial", controllerWechatPublicKeyID)
		headers.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(signature))
		return &http.Response{
			StatusCode: statusCode,
			Header:     headers,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    request,
		}, nil
	})
	resetWechatClientCacheForTest()
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
		resetWechatClientCacheForTest()
	})
	return counts
}

func queryWechatOrderTest(t *testing.T, userID int, tradeNo string) (wechatOrderResult, bool) {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/wechat/order/"+tradeNo, nil)
	context.Set("id", userID)
	context.Params = gin.Params{{Key: "trade_no", Value: tradeNo}}
	QueryWechatOrder(context)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool              `json:"success"`
		Data    wechatOrderResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response.Data, response.Success
}

func TestWechatPaymentSnapshotConvertsCNYMajorUnitsToFenOnce(t *testing.T) {
	tests := []struct {
		name       string
		majorUnits float64
		wantMicros int64
		wantFen    int64
	}{
		{name: "one fen", majorUnits: 0.01, wantMicros: 10_000, wantFen: 1},
		{name: "ordinary amount", majorUnits: 9.99, wantMicros: 9_990_000, wantFen: 999},
		{name: "large amount", majorUnits: 12_345.67, wantMicros: 12_345_670_000, wantFen: 1_234_567},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, fen, err := wechatPaymentSnapshot(test.majorUnits)
			require.NoError(t, err)
			assert.Equal(t, "CNY", snapshot.Currency)
			assert.Equal(t, test.wantMicros, snapshot.AmountMicros)
			assert.Equal(t, test.wantFen, fen)

			fromWechat, err := model.NewSubscriptionPaymentFromMinorUnits(fen, "CNY")
			require.NoError(t, err)
			assert.Equal(t, snapshot, fromWechat)
		})
	}
}

func TestWechatSubscriptionPaymentSnapshotUsesConfiguredCNYRate(t *testing.T) {
	tests := []struct {
		name       string
		price      float64
		currency   string
		unitPrice  float64
		wantAmount int64
		wantFen    int64
		wantError  bool
	}{
		{
			name:       "USD converted to CNY",
			price:      9.99,
			currency:   "USD",
			unitPrice:  7.3,
			wantAmount: 72930000,
			wantFen:    7293,
		},
		{
			name:       "CNY remains CNY",
			price:      9.99,
			currency:   "CNY",
			unitPrice:  7.3,
			wantAmount: 9990000,
			wantFen:    999,
		},
		{
			name:      "unsupported currency",
			price:     9.99,
			currency:  "EUR",
			unitPrice: 7.3,
			wantError: true,
		},
		{
			name:      "invalid rate",
			price:     9.99,
			currency:  "USD",
			unitPrice: 0,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, fen, err := wechatSubscriptionPaymentSnapshot(&model.SubscriptionPlan{
				Id:          1,
				Title:       "test plan",
				PriceAmount: test.price,
				Currency:    test.currency,
			}, test.unitPrice)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantAmount, snapshot.AmountMicros)
			assert.Equal(t, "CNY", snapshot.Currency)
			assert.Equal(t, test.wantFen, fen)
		})
	}
}

func TestWechatTopUpPaymentSnapshotUsesConfiguredCNYRate(t *testing.T) {
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalQuotaPerUnit := common.QuotaPerUnit
	originalEpayPrice := operation_setting.Price
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.Price = originalEpayPrice
	})

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	common.QuotaPerUnit = 500_000
	operation_setting.Price = 6.5
	snapshot, fen, err := wechatTopUpPaymentSnapshot(2, "default", 7.3)
	require.NoError(t, err)
	assert.Equal(t, int64(14_600_000), snapshot.AmountMicros)
	assert.Equal(t, int64(1460), fen)
	assert.Equal(t, 13.0, getPayMoney(2, "default"))
}

func TestRequestWechatPayPersistsExactExpiryAndPaymentSnapshot(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	confirmPaymentComplianceForTest(t)
	setting.WechatOrderExpireMinutes = 5
	originalPrice := operation_setting.Price
	originalMinimum := operation_setting.MinTopUp
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	operation_setting.Price = 1
	operation_setting.MinTopUp = 1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		operation_setting.MinTopUp = originalMinimum
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})

	user := model.User{Username: "wechat-create-topup", Password: "password", AffCode: "wechat-create-topup", Group: "default"}
	require.NoError(t, model.DB.Create(&user).Error)
	captured := installWechatNativeCreateTransport(t, platformPrivate)
	request := httptest.NewRequest(http.MethodPost, "/api/user/wechat/pay", strings.NewReader(`{"amount":2}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Set("id", user.Id)
	startedAt := time.Now().Unix()

	RequestWechatPay(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool              `json:"success"`
		Data    wechatOrderResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, common.TopUpStatusPending, response.Data.Status)
	assert.Equal(t, "CNY", response.Data.Currency)
	assert.InDelta(t, startedAt+5*60, response.Data.ExpiresAt, 2)

	nativeOrder := <-captured
	assert.Equal(t, response.Data.TradeNo, nativeOrder.OutTradeNo)
	assert.Equal(t, controllerWechatMchID, nativeOrder.MchID)
	assert.Equal(t, controllerWechatAppID, nativeOrder.AppID)
	assert.Equal(t, "CNY", nativeOrder.Amount.Currency)
	expiresAt, err := time.Parse(time.RFC3339, nativeOrder.TimeExpire)
	require.NoError(t, err)
	assert.Equal(t, response.Data.ExpiresAt, expiresAt.Unix())

	stored, err := model.FindTopUpByTradeNo(response.Data.TradeNo)
	require.NoError(t, err)
	assert.Equal(t, response.Data.ExpiresAt, stored.PaymentExpireTime)
	assert.Equal(t, nativeOrder.Amount.Total*10_000, stored.ExpectedAmountMicros)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
	assert.Equal(t, model.PaymentProviderWechat, stored.PaymentProvider)
}

func TestRequestWechatSubscriptionPayPersistsExactExpiryAndPaymentSnapshot(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	confirmPaymentComplianceForTest(t)
	setting.WechatOrderExpireMinutes = 5
	user := model.User{Username: "wechat-create-subscription", Password: "password", AffCode: "wechat-create-subscription", Group: "default"}
	require.NoError(t, model.DB.Create(&user).Error)
	plan := model.SubscriptionPlan{
		Title: "WeChat creation plan", PriceAmount: 9.99, Currency: "USD",
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: 1000,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	captured := installWechatNativeCreateTransport(t, platformPrivate)
	request := httptest.NewRequest(http.MethodPost, "/api/subscription/wechat/pay", strings.NewReader(fmt.Sprintf(`{"plan_id":%d}`, plan.Id)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Set("id", user.Id)
	startedAt := time.Now().Unix()

	RequestWechatSubscriptionPay(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool              `json:"success"`
		Data    wechatOrderResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, common.TopUpStatusPending, response.Data.Status)
	assert.Equal(t, "CNY", response.Data.Currency)
	assert.InDelta(t, startedAt+5*60, response.Data.ExpiresAt, 2)

	nativeOrder := <-captured
	assert.Equal(t, response.Data.TradeNo, nativeOrder.OutTradeNo)
	assert.Equal(t, int64(999), nativeOrder.Amount.Total)
	assert.Equal(t, "CNY", nativeOrder.Amount.Currency)
	expiresAt, err := time.Parse(time.RFC3339, nativeOrder.TimeExpire)
	require.NoError(t, err)
	assert.Equal(t, response.Data.ExpiresAt, expiresAt.Unix())

	stored, err := model.FindSubscriptionOrderByTradeNo(response.Data.TradeNo)
	require.NoError(t, err)
	assert.Equal(t, response.Data.ExpiresAt, stored.PaymentExpireTime)
	assert.Equal(t, nativeOrder.Amount.Total*10_000, stored.ExpectedAmountMicros)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
	assert.Equal(t, model.PaymentProviderWechat, stored.PaymentProvider)
}

func TestWechatNativeCreateFailureUsesVerifiedReconciliation(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	confirmPaymentComplianceForTest(t)

	t.Run("wallet recharge", func(t *testing.T) {
		originalPrice := operation_setting.Price
		originalMinimum := operation_setting.MinTopUp
		originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
		operation_setting.Price = 1
		operation_setting.MinTopUp = 1
		operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
		t.Cleanup(func() {
			operation_setting.Price = originalPrice
			operation_setting.MinTopUp = originalMinimum
			operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		})

		user := model.User{Username: "wechat-create-failed-topup", Password: "password", AffCode: "wechat-create-failed-topup", Group: "default"}
		require.NoError(t, model.DB.Create(&user).Error)
		counts := installWechatNativeCreateFailureTransport(t, platformPrivate, "ORDER_NOT_EXIST")
		request := httptest.NewRequest(http.MethodPost, "/api/user/wechat/pay", strings.NewReader(`{"amount":2}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		context.Set("id", user.Id)

		RequestWechatPay(context)

		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		var order model.TopUp
		require.NoError(t, model.DB.Where("user_id = ? AND payment_provider = ?", user.Id, model.PaymentProviderWechat).First(&order).Error)
		assert.Equal(t, common.TopUpStatusFailed, order.Status)
		assert.Positive(t, order.CompleteTime)
		assert.Equal(t, 1, counts.query)
		assert.Zero(t, counts.close)
	})

	t.Run("verified success is fulfilled", func(t *testing.T) {
		originalPrice := operation_setting.Price
		originalMinimum := operation_setting.MinTopUp
		originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
		originalQuotaPerUnit := common.QuotaPerUnit
		operation_setting.Price = 1
		operation_setting.MinTopUp = 1
		operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
		common.QuotaPerUnit = 100
		t.Cleanup(func() {
			operation_setting.Price = originalPrice
			operation_setting.MinTopUp = originalMinimum
			operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
			common.QuotaPerUnit = originalQuotaPerUnit
		})

		user := model.User{Username: "wechat-create-success-topup", Password: "password", AffCode: "wechat-create-success-topup", Group: "default"}
		require.NoError(t, model.DB.Create(&user).Error)
		counts := installWechatNativeCreateFailureTransport(t, platformPrivate, "SUCCESS")
		request := httptest.NewRequest(http.MethodPost, "/api/user/wechat/pay", strings.NewReader(`{"amount":2}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		context.Set("id", user.Id)

		RequestWechatPay(context)

		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		var order model.TopUp
		require.NoError(t, model.DB.Where("user_id = ? AND payment_provider = ?", user.Id, model.PaymentProviderWechat).First(&order).Error)
		assert.Equal(t, common.TopUpStatusSuccess, order.Status)
		assert.Positive(t, order.CompleteTime)
		var storedUser model.User
		require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
		assert.Equal(t, 200, storedUser.Quota)
		assert.Equal(t, 1, counts.query)
		assert.Zero(t, counts.close)
	})

	t.Run("subscription remains pending when query is indeterminate", func(t *testing.T) {
		user := model.User{Username: "wechat-create-failed-subscription", Password: "password", AffCode: "wechat-create-failed-subscription", Group: "default"}
		require.NoError(t, model.DB.Create(&user).Error)
		plan := model.SubscriptionPlan{
			Title: "WeChat failed creation plan", PriceAmount: 9.99, Currency: "CNY",
			DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
			Enabled: true, TotalAmount: 1000, MaxPurchasePerUser: 1,
		}
		require.NoError(t, model.DB.Create(&plan).Error)
		counts := installWechatNativeCreateFailureTransport(t, platformPrivate, "SYSTEM_ERROR")
		request := httptest.NewRequest(http.MethodPost, "/api/subscription/wechat/pay", strings.NewReader(fmt.Sprintf(`{"plan_id":%d}`, plan.Id)))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		context.Set("id", user.Id)

		RequestWechatSubscriptionPay(context)

		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		var order model.SubscriptionOrder
		require.NoError(t, model.DB.Where("user_id = ? AND payment_provider = ?", user.Id, model.PaymentProviderWechat).First(&order).Error)
		assert.Equal(t, common.TopUpStatusPending, order.Status)
		assert.Zero(t, order.CompleteTime)
		assert.Equal(t, 1, counts.query)
		assert.Zero(t, counts.close)
	})

	t.Run("verified unpaid subscription is closed", func(t *testing.T) {
		user := model.User{Username: "wechat-create-close-subscription", Password: "password", AffCode: "wechat-create-close-subscription", Group: "default"}
		require.NoError(t, model.DB.Create(&user).Error)
		plan := model.SubscriptionPlan{
			Title: "WeChat close creation plan", PriceAmount: 9.99, Currency: "CNY",
			DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
			Enabled: true, TotalAmount: 1000, MaxPurchasePerUser: 1,
		}
		require.NoError(t, model.DB.Create(&plan).Error)
		counts := installWechatNativeCreateFailureTransport(t, platformPrivate, "NOTPAY")
		request := httptest.NewRequest(http.MethodPost, "/api/subscription/wechat/pay", strings.NewReader(fmt.Sprintf(`{"plan_id":%d}`, plan.Id)))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		context.Set("id", user.Id)

		RequestWechatSubscriptionPay(context)

		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		var order model.SubscriptionOrder
		require.NoError(t, model.DB.Where("user_id = ? AND payment_provider = ?", user.Id, model.PaymentProviderWechat).First(&order).Error)
		assert.Equal(t, common.TopUpStatusExpired, order.Status)
		assert.Positive(t, order.CompleteTime)
		assert.Equal(t, 1, counts.query)
		assert.Equal(t, 1, counts.close)
	})
}

func TestWechatPayNotifyCompletesTopUpOnce(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	user := model.User{Username: "wechat-notify-user", Password: "password", AffCode: "wechat-notify-user"}
	require.NoError(t, model.DB.Create(&user).Error)
	topUp := model.TopUp{
		UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: "WcontrollerTopup1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(&topUp).Error)

	response := callWechatNotifyTest(t, signedWechatNotifyRequest(t, platformPrivate, topUp.TradeNo, 999))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"code":"SUCCESS","message":"成功"}`, response.Body.String())
	response = callWechatNotifyTest(t, signedWechatNotifyRequest(t, platformPrivate, topUp.TradeNo, 999))
	assert.Equal(t, http.StatusOK, response.Code)

	var storedUser model.User
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, 200, storedUser.Quota)
	var storedTopUp model.TopUp
	require.NoError(t, model.DB.First(&storedTopUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, storedTopUp.Status)
	var logCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeTopup).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)
}

func TestWechatPayNotifyRejectsAmountMismatchWithoutCrediting(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	user := model.User{Username: "wechat-mismatch-user", Password: "password", AffCode: "wechat-mismatch-user"}
	require.NoError(t, model.DB.Create(&user).Error)
	topUp := model.TopUp{
		UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: "WcontrollerMismatch1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(&topUp).Error)

	response := callWechatNotifyTest(t, signedWechatNotifyRequest(t, platformPrivate, topUp.TradeNo, 998))
	assert.Equal(t, http.StatusBadRequest, response.Code)
	var storedUser model.User
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	assert.Zero(t, storedUser.Quota)
	var storedTopUp model.TopUp
	require.NoError(t, model.DB.First(&storedTopUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, storedTopUp.Status)
}

func TestWechatPayNotifyCompletesSubscriptionOnce(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	user := model.User{Username: "wechat-subscription-user", Password: "password", AffCode: "wechat-subscription-user"}
	require.NoError(t, model.DB.Create(&user).Error)
	plan := model.SubscriptionPlan{
		Title: "WeChat plan", PriceAmount: 9.99, Currency: "CNY",
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: 1000,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	order := model.SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "ScontrollerWechat1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.CreatePendingSubscriptionOrder(&order, &plan))

	response := callWechatNotifyTest(t, signedWechatNotifyRequest(t, platformPrivate, order.TradeNo, 999))
	assert.Equal(t, http.StatusOK, response.Code)
	response = callWechatNotifyTest(t, signedWechatNotifyRequest(t, platformPrivate, order.TradeNo, 999))
	assert.Equal(t, http.StatusOK, response.Code)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
	var storedOrder model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&storedOrder).Error)
	assert.Equal(t, common.TopUpStatusSuccess, storedOrder.Status)
	assert.Equal(t, int64(9_990_000), storedOrder.PaidAmountMicros)
	assert.Equal(t, "CNY", storedOrder.PaidCurrency)
}

func TestWechatPayNotifyRejectsSubscriptionOverpayment(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	user := model.User{Username: "wechat-subscription-overpayment-user", Password: "password", AffCode: "wechat-subscription-overpayment-user"}
	require.NoError(t, model.DB.Create(&user).Error)
	plan := model.SubscriptionPlan{
		Title: "WeChat exact plan", PriceAmount: 9.99, Currency: "CNY",
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: 1000,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	order := model.SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "ScontrollerWechatOverpay1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.CreatePendingSubscriptionOrder(&order, &plan))

	response := callWechatNotifyTest(t, signedWechatNotifyRequest(t, platformPrivate, order.TradeNo, 1000))
	assert.Equal(t, http.StatusBadRequest, response.Code)
	var subscriptionCount int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)
	var storedOrder model.SubscriptionOrder
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&storedOrder).Error)
	assert.Equal(t, common.TopUpStatusPending, storedOrder.Status)
	assert.Zero(t, storedOrder.PaidAmountMicros)
}

func TestQueryWechatOrderMapsVerifiedTerminalStates(t *testing.T) {
	tests := []struct {
		tradeState string
		status     string
	}{
		{tradeState: "CLOSED", status: common.TopUpStatusExpired},
		{tradeState: "REVOKED", status: common.TopUpStatusExpired},
		{tradeState: "PAYERROR", status: common.TopUpStatusFailed},
		{tradeState: "REFUND", status: common.TopUpStatusFailed},
	}
	for _, test := range tests {
		t.Run(test.tradeState, func(t *testing.T) {
			setupStripeWebhookTestDB(t)
			gin.SetMode(gin.TestMode)
			platformPrivate := configureWechatNotifyTest(t)
			user := model.User{Username: "wechat-query-" + test.tradeState, Password: "password", AffCode: "wechat-query-" + test.tradeState}
			require.NoError(t, model.DB.Create(&user).Error)
			tradeNo := "Wquery" + test.tradeState + "1"
			topUp := model.TopUp{
				UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: tradeNo,
				PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
				Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
				Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
			}
			require.NoError(t, model.DB.Create(&topUp).Error)
			requestCount := installWechatQueryTransport(t, platformPrivate, tradeNo, test.tradeState, 999)

			result, success := queryWechatOrderTest(t, user.Id, tradeNo)
			assert.True(t, success)
			assert.Equal(t, test.status, result.Status)
			assert.Equal(t, test.tradeState, result.TradeState)
			assert.Equal(t, 1, *requestCount)
			require.NoError(t, model.DB.First(&topUp, topUp.Id).Error)
			assert.Equal(t, test.status, topUp.Status)
		})
	}
}

func TestQueryWechatOrderAcceptsOnlyDocumentedOpenStates(t *testing.T) {
	for _, tradeState := range []string{"NOTPAY", "USERPAYING"} {
		t.Run(tradeState, func(t *testing.T) {
			setupStripeWebhookTestDB(t)
			gin.SetMode(gin.TestMode)
			platformPrivate := configureWechatNotifyTest(t)
			user := model.User{Username: "wechat-open-" + tradeState, Password: "password", AffCode: "wechat-open-" + tradeState}
			require.NoError(t, model.DB.Create(&user).Error)
			tradeNo := "Wopen" + tradeState + "1"
			topUp := model.TopUp{
				UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: tradeNo,
				PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
				Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
				Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
			}
			require.NoError(t, model.DB.Create(&topUp).Error)
			installWechatQueryTransport(t, platformPrivate, tradeNo, tradeState, 999)

			result, success := queryWechatOrderTest(t, user.Id, tradeNo)
			assert.True(t, success)
			assert.Equal(t, common.TopUpStatusPending, result.Status)
			require.NoError(t, model.DB.First(&topUp, topUp.Id).Error)
			assert.Equal(t, common.TopUpStatusPending, topUp.Status)
		})
	}
}

func TestQueryWechatOrderClosesOnlyExpiredNotPayOrders(t *testing.T) {
	tests := []struct {
		name        string
		closeStatus int
		success     bool
		status      string
	}{
		{name: "verified close", closeStatus: http.StatusNoContent, success: true, status: common.TopUpStatusExpired},
		{name: "close failure", closeStatus: http.StatusInternalServerError, success: false, status: common.TopUpStatusPending},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupStripeWebhookTestDB(t)
			gin.SetMode(gin.TestMode)
			platformPrivate := configureWechatNotifyTest(t)
			suffix := strconv.Itoa(test.closeStatus)
			user := model.User{Username: "wechat-expired-" + suffix, Password: "password", AffCode: "wechat-expired-" + suffix}
			require.NoError(t, model.DB.Create(&user).Error)
			topUp := model.TopUp{
				UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: "WexpiredNotPay1",
				PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
				Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
				Status: common.TopUpStatusPending, CreateTime: time.Now().Add(-time.Hour).Unix(),
				PaymentExpireTime: time.Now().Add(-time.Minute).Unix(),
			}
			require.NoError(t, model.DB.Create(&topUp).Error)
			counts := installWechatExpiredOrderTransport(t, platformPrivate, topUp.TradeNo, test.closeStatus)

			result, success := queryWechatOrderTest(t, user.Id, topUp.TradeNo)
			assert.Equal(t, test.success, success)
			if test.success {
				assert.Equal(t, common.TopUpStatusExpired, result.Status)
				assert.Equal(t, "CLOSED", result.TradeState)
			}
			assert.Equal(t, 1, counts.query)
			assert.Equal(t, 1, counts.close)
			require.NoError(t, model.DB.First(&topUp, topUp.Id).Error)
			assert.Equal(t, test.status, topUp.Status)
		})
	}
}

func TestQueryWechatOrderRejectsUnknownProviderState(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	user := model.User{Username: "wechat-unknown-state", Password: "password", AffCode: "wechat-unknown-state"}
	require.NoError(t, model.DB.Create(&user).Error)
	topUp := model.TopUp{
		UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: "WunknownState1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(&topUp).Error)
	installWechatQueryTransport(t, platformPrivate, topUp.TradeNo, "FUTURE_STATE", 999)

	_, success := queryWechatOrderTest(t, user.Id, topUp.TradeNo)
	assert.False(t, success)
	require.NoError(t, model.DB.First(&topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestQueryWechatOrderMapsSubscriptionRefundToFailed(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	user := model.User{Username: "wechat-subscription-refund", Password: "password", AffCode: "wechat-subscription-refund"}
	require.NoError(t, model.DB.Create(&user).Error)
	plan := model.SubscriptionPlan{
		Title: "Refunded WeChat plan", PriceAmount: 9.99, Currency: "CNY",
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: 1000,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	order := model.SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, Money: 9.99, TradeNo: "SqueryRefund1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.CreatePendingSubscriptionOrder(&order, &plan))
	installWechatQueryTransport(t, platformPrivate, order.TradeNo, "REFUND", 999)

	result, success := queryWechatOrderTest(t, user.Id, order.TradeNo)
	assert.True(t, success)
	assert.Equal(t, common.TopUpStatusFailed, result.Status)
	require.NoError(t, model.DB.Where("trade_no = ?", order.TradeNo).First(&order).Error)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
}

func TestQueryAndNotifyShareIdempotentWechatSettlement(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	user := model.User{Username: "wechat-query-success", Password: "password", AffCode: "wechat-query-success"}
	require.NoError(t, model.DB.Create(&user).Error)
	topUp := model.TopUp{
		UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: "WquerySuccess1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(&topUp).Error)
	requestCount := installWechatQueryTransport(t, platformPrivate, topUp.TradeNo, "SUCCESS", 999)

	result, success := queryWechatOrderTest(t, user.Id, topUp.TradeNo)
	assert.True(t, success)
	assert.Equal(t, common.TopUpStatusSuccess, result.Status)
	assert.Equal(t, "SUCCESS", result.TradeState)
	assert.Equal(t, 1, *requestCount)

	notifyResponse := callWechatNotifyTest(t, signedWechatNotifyRequest(t, platformPrivate, topUp.TradeNo, 999))
	assert.Equal(t, http.StatusOK, notifyResponse.Code)
	var storedUser model.User
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, 200, storedUser.Quota)
	var logCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeTopup).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)
}

func TestQueryWechatOrderDoesNotReopenLocalTerminalOrder(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	user := model.User{Username: "wechat-local-terminal", Password: "password", AffCode: "wechat-local-terminal"}
	require.NoError(t, model.DB.Create(&user).Error)
	topUp := model.TopUp{
		UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: "WlocalExpired1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusExpired, CreateTime: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(&topUp).Error)
	requestCount := installWechatQueryTransport(t, platformPrivate, topUp.TradeNo, "NOTPAY", 999)

	result, success := queryWechatOrderTest(t, user.Id, topUp.TradeNo)
	assert.True(t, success)
	assert.Equal(t, common.TopUpStatusExpired, result.Status)
	assert.Zero(t, *requestCount)
}

func TestWechatBackgroundReconciliationClosesExpiredNotPayOrder(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	user := model.User{Username: "wechat-background-close", Password: "password", AffCode: "wechat-background-close"}
	require.NoError(t, model.DB.Create(&user).Error)
	topUp := model.TopUp{
		UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: "WbackgroundClose1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Add(-time.Hour).Unix(),
		PaymentExpireTime: time.Now().Add(-time.Minute).Unix(),
	}
	require.NoError(t, model.DB.Create(&topUp).Error)
	counts := installWechatExpiredOrderTransport(t, platformPrivate, topUp.TradeNo, http.StatusNoContent)

	nextCursor := runWechatOrderReconciliationBatch(context.Background(), wechatReconciliationCursor{})

	assert.Equal(t, wechatReconciliationCursor{}, nextCursor)
	assert.Equal(t, 1, counts.query)
	assert.Equal(t, 1, counts.close)
	require.NoError(t, model.DB.First(&topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusExpired, topUp.Status)
	assert.Positive(t, topUp.CompleteTime)
}

func TestWechatBackgroundReconciliationRetainsUserPayingOrder(t *testing.T) {
	setupStripeWebhookTestDB(t)
	gin.SetMode(gin.TestMode)
	platformPrivate := configureWechatNotifyTest(t)
	user := model.User{Username: "wechat-background-paying", Password: "password", AffCode: "wechat-background-paying"}
	require.NoError(t, model.DB.Create(&user).Error)
	topUp := model.TopUp{
		UserId: user.Id, Amount: 2, Money: 9.99, TradeNo: "WbackgroundPaying1",
		PaymentMethod: model.PaymentMethodWechat, PaymentProvider: model.PaymentProviderWechat,
		Currency: "CNY", ExpectedAmountMicros: 9_990_000, ExpectedCurrency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Add(-time.Hour).Unix(),
		PaymentExpireTime: time.Now().Add(-time.Minute).Unix(),
	}
	require.NoError(t, model.DB.Create(&topUp).Error)
	requestCount := installWechatQueryTransport(t, platformPrivate, topUp.TradeNo, "USERPAYING", 999)

	runWechatOrderReconciliationBatch(context.Background(), wechatReconciliationCursor{})

	assert.Equal(t, 1, *requestCount)
	require.NoError(t, model.DB.First(&topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Zero(t, topUp.CompleteTime)
}

func TestBuildWechatSettingsUpdateRetainsHiddenCredentials(t *testing.T) {
	configureWechatNotifyTest(t)
	enabled := true
	expireMinutes := 90
	unitPrice := 8.2
	values, err := buildWechatSettingsUpdate(wechatSettingsUpdateRequest{
		Enabled:                 &enabled,
		MchID:                   controllerWechatMchID,
		AppID:                   controllerWechatAppID,
		CertificateSerialNumber: "a1b2c3d4",
		PayPublicKeyID:          controllerWechatPublicKeyID,
		NotifyURL:               "https://pay.example.com/api/wechat/notify/",
		OrderExpireMinutes:      &expireMinutes,
		UnitPrice:               &unitPrice,
	})
	require.NoError(t, err)
	assert.Equal(t, controllerWechatAPIv3Key, values["WechatApiV3Key"])
	assert.Equal(t, setting.WechatPrivateKey, values["WechatPrivateKey"])
	assert.Equal(t, setting.WechatPayPublicKey, values["WechatPayPublicKey"])
	assert.Equal(t, "A1B2C3D4", values["WechatCertificateSerialNumber"])
	assert.Equal(t, "https://pay.example.com/api/wechat/notify", values["WechatNotifyURL"])
	assert.Equal(t, "90", values["WechatOrderExpireMinutes"])
	assert.Equal(t, "8.2", values["WechatUnitPrice"])
}

func TestBuildWechatSettingsUpdateCanDisableInvalidRetainedConfiguration(t *testing.T) {
	configureWechatNotifyTest(t)
	setting.WechatPrivateKey = "invalid retained private key"
	enabled := false
	values, err := buildWechatSettingsUpdate(wechatSettingsUpdateRequest{
		Enabled:    &enabled,
		MchID:      "invalid-old-merchant-id",
		NotifyURL:  "http://invalid-old-notify.example.com",
		PrivateKey: "another invalid value",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"WechatEnabled": "false"}, values)
}
