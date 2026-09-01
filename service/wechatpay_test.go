package service

import (
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	wechatTestMchID       = "1234567890"
	wechatTestAppID       = "wx1234567890abcdef"
	wechatTestAPIv3Key    = "0123456789abcdef0123456789abcdef"
	wechatTestMerchantSer = "A1B2C3D4"
	wechatTestPublicKeyID = "PUB_KEY_ID_3000000001"
)

type wechatTestFixture struct {
	config          WechatPayConfig
	merchantPrivate *rsa.PrivateKey
	platformPrivate *rsa.PrivateKey
}

func newWechatTestFixture(t *testing.T) wechatTestFixture {
	t.Helper()
	merchantPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	platformPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	merchantDER, err := x509.MarshalPKCS8PrivateKey(merchantPrivate)
	require.NoError(t, err)
	platformDER, err := x509.MarshalPKIXPublicKey(&platformPrivate.PublicKey)
	require.NoError(t, err)

	return wechatTestFixture{
		config: WechatPayConfig{
			MchID:                   wechatTestMchID,
			AppID:                   wechatTestAppID,
			ApiV3Key:                wechatTestAPIv3Key,
			CertificateSerialNumber: wechatTestMerchantSer,
			PrivateKey: string(pem.EncodeToMemory(&pem.Block{
				Type:  "PRIVATE KEY",
				Bytes: merchantDER,
			})),
			PayPublicKeyID: wechatTestPublicKeyID,
			PayPublicKey: string(pem.EncodeToMemory(&pem.Block{
				Type:  "PUBLIC KEY",
				Bytes: platformDER,
			})),
		},
		merchantPrivate: merchantPrivate,
		platformPrivate: platformPrivate,
	}
}

func signWechatTestPayload(privateKey *rsa.PrivateKey, message string) (string, error) {
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func signedWechatTestHeaders(privateKey *rsa.PrivateKey, serial string, body []byte, timestamp int64) (http.Header, error) {
	timestampValue := strconv.FormatInt(timestamp, 10)
	nonce := "response-nonce"
	signature, err := signWechatTestPayload(privateKey, fmt.Sprintf("%s\n%s\n%s\n", timestampValue, nonce, body))
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	headers.Set("Wechatpay-Timestamp", timestampValue)
	headers.Set("Wechatpay-Nonce", nonce)
	headers.Set("Wechatpay-Serial", serial)
	headers.Set("Wechatpay-Signature", signature)
	return headers, nil
}

func parseWechatTestAuthorization(value string) (map[string]string, bool) {
	const prefix = "WECHATPAY2-SHA256-RSA2048 "
	if !strings.HasPrefix(value, prefix) {
		return nil, false
	}
	fields := make(map[string]string)
	for _, item := range strings.Split(strings.TrimPrefix(value, prefix), ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 || len(parts[1]) < 2 || parts[1][0] != '"' || parts[1][len(parts[1])-1] != '"' {
			return nil, false
		}
		fields[parts[0]] = parts[1][1 : len(parts[1])-1]
	}
	return fields, true
}

func TestWechatPayClientCreatesSignedNativeOrder(t *testing.T) {
	fixture := newWechatTestFixture(t)
	type capturedRequest struct {
		method        string
		requestURI    string
		authorization string
		contentType   string
		body          []byte
	}
	captured := make(chan capturedRequest, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		captured <- capturedRequest{
			method:        request.Method,
			requestURI:    request.URL.RequestURI(),
			authorization: request.Header.Get("Authorization"),
			contentType:   request.Header.Get("Content-Type"),
			body:          body,
		}
		responseBody := []byte(`{"code_url":"weixin://wxpay/bizpayurl/up?pr=test-token"}`)
		headers, headerErr := signedWechatTestHeaders(fixture.platformPrivate, wechatTestPublicKeyID, responseBody, time.Now().Unix())
		if headerErr != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		for key, values := range headers {
			writer.Header()[key] = values
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(responseBody)
	}))
	t.Cleanup(server.Close)

	client, err := newWechatPayClient(fixture.config, server.Client(), server.URL)
	require.NoError(t, err)
	expiresAt := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	response, err := client.CreateNativeOrder(context.Background(), WechatNativeOrder{
		AppID:       wechatTestAppID,
		MchID:       wechatTestMchID,
		Description: "Wallet recharge",
		OutTradeNo:  "W123456789",
		NotifyURL:   "https://pay.example.com/api/wechat/notify",
		AmountTotal: 1234,
		TimeExpire:  &expiresAt,
	})
	require.NoError(t, err)
	assert.Equal(t, "weixin://wxpay/bizpayurl/up?pr=test-token", response.CodeURL)

	request := <-captured
	assert.Equal(t, http.MethodPost, request.method)
	assert.Equal(t, wechatPayNativePath, request.requestURI)
	assert.Equal(t, "application/json", request.contentType)
	var payload struct {
		AppID       string `json:"appid"`
		MchID       string `json:"mchid"`
		Description string `json:"description"`
		OutTradeNo  string `json:"out_trade_no"`
		TimeExpire  string `json:"time_expire"`
		NotifyURL   string `json:"notify_url"`
		Amount      struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	require.NoError(t, common.Unmarshal(request.body, &payload))
	assert.Equal(t, wechatTestAppID, payload.AppID)
	assert.Equal(t, wechatTestMchID, payload.MchID)
	assert.Equal(t, "Wallet recharge", payload.Description)
	assert.Equal(t, "W123456789", payload.OutTradeNo)
	assert.Equal(t, expiresAt.Format(time.RFC3339), payload.TimeExpire)
	assert.Equal(t, "https://pay.example.com/api/wechat/notify", payload.NotifyURL)
	assert.Equal(t, int64(1234), payload.Amount.Total)
	assert.Equal(t, "CNY", payload.Amount.Currency)

	authorization, ok := parseWechatTestAuthorization(request.authorization)
	require.True(t, ok)
	assert.Equal(t, wechatTestMchID, authorization["mchid"])
	assert.Equal(t, wechatTestMerchantSer, authorization["serial_no"])
	timestamp, err := strconv.ParseInt(authorization["timestamp"], 10, 64)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), time.Unix(timestamp, 0), 5*time.Second)
	signature, err := base64.StdEncoding.DecodeString(authorization["signature"])
	require.NoError(t, err)
	message := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n", request.method, request.requestURI, authorization["timestamp"], authorization["nonce_str"], request.body)
	digest := sha256.Sum256([]byte(message))
	assert.NoError(t, rsa.VerifyPKCS1v15(&fixture.merchantPrivate.PublicKey, crypto.SHA256, digest[:], signature))
}

func TestWechatPayClientClosesOrderAndVerifiesEmptyResponse(t *testing.T) {
	fixture := newWechatTestFixture(t)
	requestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v3/pay/transactions/out-trade-no/W123456789/close", request.URL.Path)
		body, err := io.ReadAll(request.Body)
		if !assert.NoError(t, err) {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		var payload struct {
			MchID string `json:"mchid"`
		}
		if !assert.NoError(t, common.Unmarshal(body, &payload)) {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		assert.Equal(t, wechatTestMchID, payload.MchID)
		if requestCount == 1 {
			headers, headerErr := signedWechatTestHeaders(fixture.platformPrivate, wechatTestPublicKeyID, nil, time.Now().Unix())
			if !assert.NoError(t, headerErr) {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			for key, values := range headers {
				writer.Header()[key] = values
			}
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client, err := newWechatPayClient(fixture.config, server.Client(), server.URL)
	require.NoError(t, err)
	require.NoError(t, client.CloseOrder(context.Background(), "W123456789"))
	err = client.CloseOrder(context.Background(), "W123456789")
	require.ErrorContains(t, err, "signature headers are incomplete")
}

func TestWechatPayClientVerifiesResponseSignatureHeaders(t *testing.T) {
	fixture := newWechatTestFixture(t)
	client, err := newWechatPayClient(fixture.config, nil, wechatPayAPIBaseURL)
	require.NoError(t, err)
	body := []byte(`{"trade_state":"SUCCESS"}`)

	tests := []struct {
		name      string
		serial    string
		timestamp int64
		mutate    func(http.Header)
		wantError string
	}{
		{name: "valid public key ID", serial: wechatTestPublicKeyID, timestamp: time.Now().Unix()},
		{name: "unknown public key ID", serial: "PUB_KEY_ID_3000000002", timestamp: time.Now().Unix(), wantError: "serial is not configured"},
		{name: "expired timestamp", serial: wechatTestPublicKeyID, timestamp: time.Now().Add(-10 * time.Minute).Unix(), wantError: "timestamp is expired"},
		{name: "invalid signature", serial: wechatTestPublicKeyID, timestamp: time.Now().Unix(), mutate: func(headers http.Header) {
			headers.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(make([]byte, 256)))
		}, wantError: "signature verification failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			headers, err := signedWechatTestHeaders(fixture.platformPrivate, test.serial, body, test.timestamp)
			require.NoError(t, err)
			if test.mutate != nil {
				test.mutate(headers)
			}
			err = client.verifySignedMessage(headers, body)
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestValidWechatCodeURLAcceptsOfficialNativeFormat(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "official format", value: "weixin://wxpay/bizpayurl/up?pr=NwY5Mz9", valid: true},
		{name: "legacy path", value: "weixin://wxpay/bizpayurl?pr=NwY5Mz9", valid: true},
		{name: "https URL", value: "https://wxpay/bizpayurl/up?pr=NwY5Mz9"},
		{name: "wrong host", value: "weixin://example.com/bizpayurl/up?pr=NwY5Mz9"},
		{name: "missing query", value: "weixin://wxpay/bizpayurl/up"},
		{name: "unexpected path", value: "weixin://wxpay/other?pr=NwY5Mz9"},
		{name: "fragment", value: "weixin://wxpay/bizpayurl/up?pr=NwY5Mz9#fragment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.valid, validWechatCodeURL(test.value))
		})
	}
}

func TestWechatPayClientRejectsRedirectsAndUnsignedErrors(t *testing.T) {
	fixture := newWechatTestFixture(t)
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		wantError string
	}{
		{
			name: "redirect",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Location", "https://example.com/capture")
				writer.WriteHeader(http.StatusFound)
			},
			wantError: "HTTP 302",
		},
		{
			name: "unsigned error body",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = writer.Write([]byte(`{"code":"PARAM_ERROR"}`))
			},
			wantError: "signature headers are incomplete",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(test.handler)
			t.Cleanup(server.Close)
			client, err := newWechatPayClient(fixture.config, server.Client(), server.URL)
			require.NoError(t, err)
			_, err = client.CreateNativeOrder(context.Background(), WechatNativeOrder{
				AppID: wechatTestAppID, MchID: wechatTestMchID, Description: "Test order",
				OutTradeNo: "W987654321", NotifyURL: "https://pay.example.com/api/wechat/notify", AmountTotal: 100,
			})
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func encryptWechatTestNotification(t *testing.T, fixture wechatTestFixture, eventType, originalType, nonce string) ([]byte, http.Header) {
	t.Helper()
	order := WechatNotificationOrder{
		MchID:         wechatTestMchID,
		AppID:         wechatTestAppID,
		OutTradeNo:    "W123456789",
		TransactionID: "4200001234567890123456789012",
		TradeState:    "SUCCESS",
	}
	order.Amount.Total = 1234
	order.Amount.Currency = "CNY"
	plaintext, err := common.Marshal(order)
	require.NoError(t, err)
	block, err := aes.NewCipher([]byte(wechatTestAPIv3Key))
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	associatedData := "transaction"
	ciphertext := []byte("invalid")
	if len([]byte(nonce)) == aead.NonceSize() {
		ciphertext = aead.Seal(nil, []byte(nonce), plaintext, []byte(associatedData))
	}
	envelope := encryptedNotification{ID: "notification-id", EventType: eventType}
	envelope.Resource.Algorithm = "AEAD_AES_256_GCM"
	envelope.Resource.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext)
	envelope.Resource.AssociatedData = associatedData
	envelope.Resource.Nonce = nonce
	envelope.Resource.OriginalType = originalType
	body, err := common.Marshal(envelope)
	require.NoError(t, err)
	headers, err := signedWechatTestHeaders(fixture.platformPrivate, wechatTestPublicKeyID, body, time.Now().Unix())
	require.NoError(t, err)
	return body, headers
}

func TestWechatPayClientDecryptsVerifiedNotification(t *testing.T) {
	fixture := newWechatTestFixture(t)
	client, err := newWechatPayClient(fixture.config, nil, wechatPayAPIBaseURL)
	require.NoError(t, err)
	body, headers := encryptWechatTestNotification(t, fixture, "TRANSACTION.SUCCESS", "transaction", "0123456789ab")

	order, err := client.DecryptNotification(headers, body)
	require.NoError(t, err)
	assert.Equal(t, wechatTestMchID, order.MchID)
	assert.Equal(t, wechatTestAppID, order.AppID)
	assert.Equal(t, "W123456789", order.OutTradeNo)
	assert.Equal(t, "4200001234567890123456789012", order.TransactionID)
	assert.Equal(t, int64(1234), order.Amount.Total)
	assert.Equal(t, "CNY", order.Amount.Currency)
}

func TestWechatPayClientRejectsUnsupportedOrMalformedNotifications(t *testing.T) {
	fixture := newWechatTestFixture(t)
	client, err := newWechatPayClient(fixture.config, nil, wechatPayAPIBaseURL)
	require.NoError(t, err)

	tests := []struct {
		name         string
		eventType    string
		originalType string
		nonce        string
		wantError    string
	}{
		{name: "wrong event", eventType: "TRANSACTION.CLOSED", originalType: "transaction", nonce: "0123456789ab", wantError: "unsupported"},
		{name: "wrong resource type", eventType: "TRANSACTION.SUCCESS", originalType: "refund", nonce: "0123456789ab", wantError: "unsupported"},
		{name: "wrong nonce length", eventType: "TRANSACTION.SUCCESS", originalType: "transaction", nonce: "short", wantError: "invalid wechat notification nonce"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, headers := encryptWechatTestNotification(t, fixture, test.eventType, test.originalType, test.nonce)
			assert.NotPanics(t, func() {
				_, err = client.DecryptNotification(headers, body)
			})
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestWechatPayClientValidatesPublicKeyModeAndNotifyURL(t *testing.T) {
	fixture := newWechatTestFixture(t)
	tests := []struct {
		name      string
		mutate    func(*WechatPayConfig)
		wantError string
	}{
		{name: "valid public key ID"},
		{name: "certificate serial used as key ID", mutate: func(config *WechatPayConfig) {
			config.PayPublicKeyID = "AABBCCDD"
		}, wantError: "public key ID is required"},
		{name: "non-numeric public key ID", mutate: func(config *WechatPayConfig) {
			config.PayPublicKeyID = "PUB_KEY_ID_TEST123"
		}, wantError: "public key ID is required"},
		{name: "key without ID", mutate: func(config *WechatPayConfig) {
			config.PayPublicKeyID = ""
		}, wantError: "must be configured together"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := fixture.config
			if test.mutate != nil {
				test.mutate(&config)
			}
			_, err := newWechatPayClient(config, nil, wechatPayAPIBaseURL)
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}

	client, err := newWechatPayClient(fixture.config, nil, wechatPayAPIBaseURL)
	require.NoError(t, err)
	_, err = client.CreateNativeOrder(context.Background(), WechatNativeOrder{
		AppID: wechatTestAppID, MchID: wechatTestMchID, Description: "Test order",
		OutTradeNo: "W123456789", NotifyURL: "https://pay.example.com/api/wechat/notify?token=secret", AmountTotal: 100,
	})
	require.ErrorContains(t, err, "invalid wechat notify_url")
	_, err = client.CreateNativeOrder(context.Background(), WechatNativeOrder{
		AppID: wechatTestAppID, MchID: wechatTestMchID, Description: "Test order",
		OutTradeNo: "W123456789", NotifyURL: "https://pay.example.com/" + strings.Repeat("a", 240), AmountTotal: 100,
	})
	require.ErrorContains(t, err, "invalid wechat notify_url")
}
