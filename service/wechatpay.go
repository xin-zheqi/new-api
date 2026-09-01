package service

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
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const (
	wechatPayAPIBaseURL        = "https://api.mch.weixin.qq.com"
	wechatPaySignatureWindow   = 5 * time.Minute
	wechatPayMaxResponseBytes  = 2 << 20
	wechatPayMaxNotifyBytes    = 1 << 20
	wechatPayMaxNotifyURLBytes = 255
	wechatPayNativePath        = "/v3/pay/transactions/native"
	wechatPayMaxAmountMinor    = int64(1_000_000_000)
)

// WechatPayConfig contains the credentials needed by the API v3 Native API.
// The service never serializes this structure or includes credential values in
// errors and logs.
type WechatPayConfig struct {
	MchID                   string
	AppID                   string
	ApiV3Key                string
	CertificateSerialNumber string
	PrivateKey              string
	MerchantCertificate     string
	PayPublicKeyID          string
	PayPublicKey            string
	PlatformCertificate     string
}

// WechatNativeOrder is the small subset of a Native order used by new-api.
type WechatNativeOrder struct {
	AppID       string
	MchID       string
	Description string
	OutTradeNo  string
	NotifyURL   string
	AmountTotal int64
	TimeExpire  *time.Time
}

// WechatNativeOrderResponse is returned by the Native prepay endpoint.
type WechatNativeOrderResponse struct {
	CodeURL string `json:"code_url"`
}

// WechatOrder is the subset returned by the order query endpoint.
type WechatOrder struct {
	MchID          string `json:"mchid"`
	AppID          string `json:"appid"`
	OutTradeNo     string `json:"out_trade_no"`
	TransactionID  string `json:"transaction_id"`
	TradeState     string `json:"trade_state"`
	TradeStateDesc string `json:"trade_state_desc"`
	Amount         struct {
		Total    int64  `json:"total"`
		Currency string `json:"currency"`
	} `json:"amount"`
}

// WechatPayAPIError represents an authenticated non-2xx response from the
// WeChat Pay API. Code is exposed for narrow state-machine decisions, while
// Error deliberately omits the provider body so upstream text cannot reach
// logs or users unchecked.
type WechatPayAPIError struct {
	StatusCode int
	Code       string
}

func (err *WechatPayAPIError) Error() string {
	if err == nil {
		return "wechat pay API request failed"
	}
	return fmt.Sprintf("wechat pay request returned HTTP %d", err.StatusCode)
}

func IsWechatOrderNotExist(err error) bool {
	var apiError *WechatPayAPIError
	return errors.As(err, &apiError) && apiError.Code == "ORDER_NOT_EXIST"
}

// WechatNotificationOrder is the decrypted payment notification payload.
type WechatNotificationOrder struct {
	MchID         string `json:"mchid"`
	AppID         string `json:"appid"`
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	TradeState    string `json:"trade_state"`
	Amount        struct {
		Total    int64  `json:"total"`
		Currency string `json:"currency"`
	} `json:"amount"`
}

type WechatPayClient struct {
	config     WechatPayConfig
	privateKey *rsa.PrivateKey
	verifiers  map[string]*rsa.PublicKey
	httpClient *http.Client
	baseURL    string
}

// NewWechatPayClient validates credentials and creates a client for the
// production WeChat Pay API endpoint.
func NewWechatPayClient(config WechatPayConfig) (*WechatPayClient, error) {
	return newWechatPayClient(config, http.DefaultClient, wechatPayAPIBaseURL)
}

// newWechatPayClient is kept injectable for deterministic service tests.
func newWechatPayClient(config WechatPayConfig, httpClient *http.Client, baseURL string) (*WechatPayClient, error) {
	config.MchID = strings.TrimSpace(config.MchID)
	config.AppID = strings.TrimSpace(config.AppID)
	config.ApiV3Key = strings.TrimSpace(config.ApiV3Key)
	config.CertificateSerialNumber = strings.TrimSpace(config.CertificateSerialNumber)
	config.PrivateKey = strings.TrimSpace(config.PrivateKey)
	config.MerchantCertificate = strings.TrimSpace(config.MerchantCertificate)
	config.PayPublicKeyID = strings.TrimSpace(config.PayPublicKeyID)
	config.PayPublicKey = strings.TrimSpace(config.PayPublicKey)
	config.PlatformCertificate = strings.TrimSpace(config.PlatformCertificate)
	if !isWechatMchID(config.MchID) || !isWechatAppID(config.AppID) || !isWechatSerial(config.CertificateSerialNumber) {
		return nil, errors.New("wechat pay merchant credentials are incomplete")
	}
	config.CertificateSerialNumber = normalizeSerial(config.CertificateSerialNumber)
	if len([]byte(config.ApiV3Key)) != 32 {
		return nil, errors.New("wechat pay APIv3 key must contain 32 bytes")
	}
	privateKey, err := parseWechatPrivateKey(config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid wechat pay private key: %w", err)
	}
	if err := privateKey.Validate(); err != nil {
		return nil, errors.New("wechat pay private key validation failed")
	}
	if privateKey.N.BitLen() < 2048 {
		return nil, errors.New("wechat pay private key must be RSA-2048 or stronger")
	}
	if config.MerchantCertificate != "" {
		certificate, certErr := parseWechatCertificate(config.MerchantCertificate)
		if certErr != nil {
			return nil, fmt.Errorf("invalid wechat merchant certificate: %w", certErr)
		}
		if !wechatCertificateCurrentlyValid(certificate, time.Now()) {
			return nil, errors.New("wechat merchant certificate is not currently valid")
		}
		if normalizeSerial(config.CertificateSerialNumber) != normalizeSerial(certificateSerial(certificate)) {
			return nil, errors.New("wechat merchant certificate serial number does not match configuration")
		}
		merchantPublicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
		if !ok || merchantPublicKey.N.Cmp(privateKey.N) != 0 || merchantPublicKey.E != privateKey.E {
			return nil, errors.New("wechat merchant certificate does not match private key")
		}
	}

	if (config.PayPublicKeyID == "") != (config.PayPublicKey == "") {
		return nil, errors.New("wechat pay public key and public key ID must be configured together")
	}
	verifiers := make(map[string]*rsa.PublicKey, 2)
	if config.PayPublicKey != "" {
		if !isWechatPayPublicKeyID(config.PayPublicKeyID) {
			return nil, errors.New("wechat pay public key ID is required")
		}
		publicKey, keyErr := parseWechatPublicKey(config.PayPublicKey)
		if keyErr != nil {
			return nil, fmt.Errorf("invalid wechat pay public key: %w", keyErr)
		}
		if publicKey.N.BitLen() < 2048 {
			return nil, errors.New("wechat pay public key must be RSA-2048 or stronger")
		}
		verifiers[config.PayPublicKeyID] = publicKey
	} else if config.PlatformCertificate != "" {
		// The stable WeChat Pay public key is the current recommended verifier.
		// Only parse the legacy platform certificate when no public key is
		// configured, so an expired retained certificate cannot break a completed
		// migration to public-key verification.
		certificate, certErr := parseWechatCertificate(config.PlatformCertificate)
		if certErr != nil {
			return nil, fmt.Errorf("invalid wechat platform certificate: %w", certErr)
		}
		publicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("wechat platform certificate does not contain an RSA key")
		}
		if publicKey.N.BitLen() < 2048 {
			return nil, errors.New("wechat platform certificate key must be RSA-2048 or stronger")
		}
		if !wechatCertificateCurrentlyValid(certificate, time.Now()) {
			return nil, errors.New("wechat platform certificate is not currently valid")
		}
		serial := normalizeSerial(certificateSerial(certificate))
		verifiers[serial] = publicKey
	}
	if len(verifiers) == 0 {
		return nil, errors.New("wechat pay platform public key or certificate is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	} else {
		// Never mutate a caller-owned client (especially http.DefaultClient).
		// Payment requests must have a bounded timeout and must reject redirects.
		clientCopy := *httpClient
		httpClient = &clientCopy
		if httpClient.Timeout <= 0 {
			httpClient.Timeout = 15 * time.Second
		}
	}
	// The API endpoint must never redirect a signed request to another host.
	// Reject redirects even when a caller supplied the default HTTP client.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	parsedBase, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsedBase.Scheme != "https" || parsedBase.Host == "" ||
		parsedBase.User != nil || parsedBase.Opaque != "" || parsedBase.RawQuery != "" || parsedBase.Fragment != "" ||
		(parsedBase.Path != "" && parsedBase.Path != "/") {
		return nil, errors.New("invalid wechat pay API endpoint")
	}
	return &WechatPayClient{
		config:     config,
		privateKey: privateKey,
		verifiers:  verifiers,
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}, nil
}

func parseWechatPrivateKey(value string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("PEM block is missing")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("PEM does not contain an RSA private key")
}

func parseWechatPublicKey(value string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("PEM block is missing")
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("PEM does not contain an RSA public key")
}

func parseWechatCertificate(value string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("PEM certificate block is missing")
	}
	return x509.ParseCertificate(block.Bytes)
}

func wechatCertificateCurrentlyValid(certificate *x509.Certificate, now time.Time) bool {
	return certificate != nil && !now.Before(certificate.NotBefore) && !now.After(certificate.NotAfter)
}

func certificateSerial(certificate *x509.Certificate) string {
	if certificate == nil || certificate.SerialNumber == nil {
		return ""
	}
	return strings.ToUpper(hex.EncodeToString(certificate.SerialNumber.Bytes()))
}

func normalizeSerial(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func isWechatMchID(value string) bool {
	if len(value) < 8 || len(value) > 32 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

func isWechatSerial(value string) bool {
	if value == "" || len(value) > 64 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F') ||
			(character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func isWechatPayPublicKeyID(value string) bool {
	const prefix = "PUB_KEY_ID_"
	if len(value) <= len(prefix) || len(value) > 64 || strings.TrimSpace(value) != value || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

func isWechatAppID(value string) bool {
	if len(value) != 18 || !strings.HasPrefix(value, "wx") || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value[2:] {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func randomNonce() (string, error) {
	bytesValue := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, bytesValue); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytesValue), nil
}

func (client *WechatPayClient) signMessage(message string) (string, error) {
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, client.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func (client *WechatPayClient) authorization(method, requestURI, body string) (string, error) {
	nonce, err := randomNonce()
	if err != nil {
		return "", err
	}
	timestamp := time.Now().Unix()
	message := fmt.Sprintf("%s\n%s\n%d\n%s\n%s\n", method, requestURI, timestamp, nonce, body)
	signature, err := client.signMessage(message)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("WECHATPAY2-SHA256-RSA2048 mchid=\"%s\",nonce_str=\"%s\",signature=\"%s\",timestamp=\"%d\",serial_no=\"%s\"", client.config.MchID, nonce, signature, timestamp, client.config.CertificateSerialNumber), nil
}

func (client *WechatPayClient) request(ctx context.Context, method, requestPath string, requestBody []byte, result any) error {
	if len(requestBody) > wechatPayMaxResponseBytes {
		return errors.New("wechat pay request body is too large")
	}
	if !strings.HasPrefix(requestPath, "/") || strings.HasPrefix(requestPath, "//") {
		return errors.New("invalid wechat pay request path")
	}
	parsed, err := url.Parse(client.baseURL + requestPath)
	if err != nil || parsed.Path == "" || parsed.Scheme != "https" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" {
		return errors.New("invalid wechat pay request path")
	}
	requestURI := parsed.RequestURI()
	authHeader, err := client.authorization(method, requestURI, string(requestBody))
	if err != nil {
		return fmt.Errorf("create wechat pay request signature: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("User-Agent", "new-api-wechatpay/1")
	response, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wechat pay request failed: %w", err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, wechatPayMaxResponseBytes+1))
	if readErr != nil {
		return fmt.Errorf("read wechat pay response: %w", readErr)
	}
	if len(body) > wechatPayMaxResponseBytes {
		return errors.New("wechat pay response is too large")
	}
	// Redirects are never followed, so they cannot be treated as an API
	// response and do not need to pass WeChat response-signature validation.
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		return fmt.Errorf("wechat pay request returned HTTP %d", response.StatusCode)
	}
	if err := client.verifySignedMessage(response.Header, body); err != nil {
		return fmt.Errorf("verify wechat pay response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var providerError struct {
			Code string `json:"code"`
		}
		_ = common.Unmarshal(body, &providerError)
		return &WechatPayAPIError{StatusCode: response.StatusCode, Code: providerError.Code}
	}
	if len(body) == 0 {
		return nil
	}
	if result == nil {
		return nil
	}
	if err := common.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decode wechat pay response: %w", err)
	}
	return nil
}

// CreateNativeOrder calls the official Native prepay endpoint and returns the
// code_url that the client should render as a QR code.
func (client *WechatPayClient) CreateNativeOrder(ctx context.Context, order WechatNativeOrder) (*WechatNativeOrderResponse, error) {
	if err := validateNativeOrder(order); err != nil {
		return nil, err
	}
	if order.AppID != client.config.AppID || order.MchID != client.config.MchID {
		return nil, errors.New("wechat order credentials do not match client configuration")
	}
	payload := struct {
		AppID       string `json:"appid"`
		MchID       string `json:"mchid"`
		Description string `json:"description"`
		OutTradeNo  string `json:"out_trade_no"`
		TimeExpire  string `json:"time_expire,omitempty"`
		NotifyURL   string `json:"notify_url"`
		Amount      struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}{
		AppID: order.AppID, MchID: order.MchID, Description: order.Description,
		OutTradeNo: order.OutTradeNo, NotifyURL: order.NotifyURL,
	}
	if order.TimeExpire != nil {
		payload.TimeExpire = order.TimeExpire.UTC().Format(time.RFC3339)
	}
	payload.Amount.Total = order.AmountTotal
	payload.Amount.Currency = "CNY"
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	response := new(WechatNativeOrderResponse)
	if err := client.request(ctx, http.MethodPost, wechatPayNativePath, body, response); err != nil {
		return nil, err
	}
	if !validWechatCodeURL(response.CodeURL) {
		return nil, errors.New("wechat pay response did not contain a valid code_url")
	}
	return response, nil
}

func validWechatCodeURL(value string) bool {
	if strings.TrimSpace(value) != value || value == "" || len(value) > 2048 {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "weixin") ||
		!strings.EqualFold(parsed.Host, "wxpay") || parsed.User != nil || parsed.Opaque != "" ||
		parsed.Fragment != "" || parsed.RawQuery == "" {
		return false
	}
	return parsed.Path == "/bizpayurl" || strings.HasPrefix(parsed.Path, "/bizpayurl/")
}

func validateNativeOrder(order WechatNativeOrder) error {
	if !isWechatAppID(order.AppID) {
		return errors.New("invalid wechat appid")
	}
	if !isWechatMchID(order.MchID) {
		return errors.New("invalid wechat mchid")
	}
	if order.Description == "" || len([]byte(order.Description)) > 127 || !utf8.ValidString(order.Description) {
		return errors.New("invalid wechat order description")
	}
	if len(order.OutTradeNo) < 6 || len(order.OutTradeNo) > 32 || !isWechatTradeNo(order.OutTradeNo) {
		return errors.New("invalid wechat out_trade_no")
	}
	if order.NotifyURL == "" || len([]byte(order.NotifyURL)) > wechatPayMaxNotifyURLBytes {
		return errors.New("invalid wechat notify_url")
	}
	notifyURL, err := url.Parse(order.NotifyURL)
	if err != nil || notifyURL.Host == "" || notifyURL.User != nil || notifyURL.Opaque != "" ||
		notifyURL.RawQuery != "" || notifyURL.Fragment != "" || notifyURL.Scheme != "https" {
		return errors.New("invalid wechat notify_url")
	}
	if order.AmountTotal <= 0 || order.AmountTotal > wechatPayMaxAmountMinor {
		return errors.New("invalid wechat order amount")
	}
	return nil
}

func isWechatTradeNo(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

// QueryOrderByOutTradeNo actively checks a Native order when a callback has
// not arrived yet.
func (client *WechatPayClient) QueryOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*WechatOrder, error) {
	if len(outTradeNo) < 6 || len(outTradeNo) > 32 || !isWechatTradeNo(outTradeNo) {
		return nil, errors.New("invalid wechat out_trade_no")
	}
	path := wechatPayNativePath[:len(wechatPayNativePath)-len("native")] + "out-trade-no/" + url.PathEscape(outTradeNo) + "?mchid=" + url.QueryEscape(client.config.MchID)
	result := new(WechatOrder)
	if err := client.request(ctx, http.MethodGet, path, nil, result); err != nil {
		return nil, err
	}
	return result, nil
}

// CloseOrder closes a verified unpaid order after its snapshotted payment
// deadline. Callers must query the order first and only close NOTPAY orders.
func (client *WechatPayClient) CloseOrder(ctx context.Context, outTradeNo string) error {
	if len(outTradeNo) < 6 || len(outTradeNo) > 32 || !isWechatTradeNo(outTradeNo) {
		return errors.New("invalid wechat out_trade_no")
	}
	path := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(outTradeNo) + "/close"
	body, err := common.Marshal(struct {
		MchID string `json:"mchid"`
	}{MchID: client.config.MchID})
	if err != nil {
		return err
	}
	return client.request(ctx, http.MethodPost, path, body, nil)
}

type encryptedNotification struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	Resource  struct {
		Algorithm      string `json:"algorithm"`
		Ciphertext     string `json:"ciphertext"`
		AssociatedData string `json:"associated_data"`
		Nonce          string `json:"nonce"`
		OriginalType   string `json:"original_type"`
	} `json:"resource"`
}

// DecryptNotification verifies the WeChat callback signature before decrypting
// its AES-256-GCM resource. It returns only the payment fields needed by the
// order settlement path.
func (client *WechatPayClient) DecryptNotification(headers http.Header, body []byte) (*WechatNotificationOrder, error) {
	if len(body) == 0 || len(body) > wechatPayMaxNotifyBytes {
		return nil, errors.New("invalid wechat notification body")
	}
	if err := client.verifySignedMessage(headers, body); err != nil {
		return nil, fmt.Errorf("verify wechat notification: %w", err)
	}
	var envelope encryptedNotification
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, errors.New("invalid wechat notification envelope")
	}
	if envelope.ID == "" || len(envelope.ID) > 64 || envelope.EventType != "TRANSACTION.SUCCESS" ||
		envelope.Resource.OriginalType != "transaction" || envelope.Resource.Algorithm != "AEAD_AES_256_GCM" ||
		envelope.Resource.Nonce == "" || envelope.Resource.Ciphertext == "" {
		return nil, errors.New("unsupported wechat notification encryption")
	}
	block, err := aes.NewCipher([]byte(client.config.ApiV3Key))
	if err != nil {
		return nil, errors.New("invalid wechat notification key")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("invalid wechat notification cipher")
	}
	if len([]byte(envelope.Resource.Nonce)) != aead.NonceSize() {
		return nil, errors.New("invalid wechat notification nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Resource.Ciphertext)
	if err != nil {
		return nil, errors.New("invalid wechat notification ciphertext")
	}
	plaintext, err := aead.Open(nil, []byte(envelope.Resource.Nonce), ciphertext, []byte(envelope.Resource.AssociatedData))
	if err != nil {
		return nil, errors.New("wechat notification decryption failed")
	}
	order := new(WechatNotificationOrder)
	if err := common.Unmarshal(plaintext, order); err != nil {
		return nil, errors.New("invalid decrypted wechat notification")
	}
	if order.MchID != client.config.MchID || order.AppID != client.config.AppID ||
		len(order.OutTradeNo) < 6 || len(order.OutTradeNo) > 32 || !isWechatTradeNo(order.OutTradeNo) ||
		order.TransactionID == "" || len(order.TransactionID) > 32 || order.TradeState != "SUCCESS" ||
		order.Amount.Currency != "CNY" || order.Amount.Total <= 0 || order.Amount.Total > wechatPayMaxAmountMinor {
		return nil, errors.New("wechat notification order verification failed")
	}
	return order, nil
}

func (client *WechatPayClient) verifySignedMessage(headers http.Header, body []byte) error {
	timestampValue := strings.TrimSpace(headers.Get("Wechatpay-Timestamp"))
	nonce := strings.TrimSpace(headers.Get("Wechatpay-Nonce"))
	signatureValue := strings.TrimSpace(headers.Get("Wechatpay-Signature"))
	serial := strings.TrimSpace(headers.Get("Wechatpay-Serial"))
	if timestampValue == "" || nonce == "" || signatureValue == "" || serial == "" {
		return errors.New("wechat pay signature headers are incomplete")
	}
	if len(timestampValue) > 20 || len(nonce) > 64 || len(signatureValue) > 1024 || len(serial) > 64 {
		return errors.New("wechat pay signature headers are invalid")
	}
	timestamp, err := strconv.ParseInt(timestampValue, 10, 64)
	if err != nil || timestamp <= 0 {
		return errors.New("wechat pay signature timestamp is expired")
	}
	if delta := time.Now().Unix() - timestamp; delta < -int64(wechatPaySignatureWindow/time.Second) || delta > int64(wechatPaySignatureWindow/time.Second) {
		return errors.New("wechat pay signature timestamp is expired")
	}
	publicKey, ok := client.verifiers[serial]
	if !ok && isWechatSerial(serial) {
		publicKey, ok = client.verifiers[normalizeSerial(serial)]
	}
	if !ok {
		return errors.New("wechat pay signature serial is not configured")
	}
	signature, err := base64.StdEncoding.DecodeString(signatureValue)
	if err != nil {
		return errors.New("wechat pay signature is not valid base64")
	}
	message := fmt.Sprintf("%s\n%s\n%s\n", timestampValue, nonce, string(body))
	digest := sha256.Sum256([]byte(message))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return errors.New("wechat pay signature verification failed")
	}
	return nil
}
