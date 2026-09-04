package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const wechatSettingsRequestMaxBytes int64 = 512 * 1024

type wechatSettingsUpdateRequest struct {
	Enabled                 *bool    `json:"enabled"`
	MchID                   string   `json:"mch_id"`
	AppID                   string   `json:"app_id"`
	ApiV3Key                string   `json:"api_v3_key"`
	CertificateSerialNumber string   `json:"certificate_serial_number"`
	PrivateKey              string   `json:"private_key"`
	MerchantCertificate     string   `json:"merchant_certificate"`
	PayPublicKeyID          string   `json:"pay_public_key_id"`
	PayPublicKey            string   `json:"pay_public_key"`
	PlatformCertificate     string   `json:"platform_certificate"`
	NotifyURL               string   `json:"notify_url"`
	OrderExpireMinutes      *int     `json:"order_expire_minutes"`
	MinTopUp                *int     `json:"min_topup"`
	UnitPrice               *float64 `json:"unit_price"`
}

func isWechatPaymentOptionKey(key string) bool {
	switch key {
	case "WechatEnabled", "WechatMchID", "WechatAppID", "WechatApiV3Key",
		"WechatCertificateSerialNumber", "WechatPrivateKey", "WechatMerchantCertificate",
		"WechatPayPublicKeyID", "WechatPayPublicKey", "WechatPlatformCertificate",
		"WechatNotifyURL", "WechatOrderExpireMinutes", "WechatMinTopUp", "WechatUnitPrice":
		return true
	default:
		return false
	}
}

// buildWechatSettingsUpdate merges hidden sensitive fields with the submitted
// values and returns one validated option set for atomic publication.
func buildWechatSettingsUpdate(request wechatSettingsUpdateRequest) (map[string]string, error) {
	if request.Enabled == nil {
		return nil, errors.New("wechat enabled state is required")
	}
	currentSettings := currentWechatRuntimeSettings()
	minTopUp := currentSettings.MinTopUp
	if request.MinTopUp != nil {
		normalized, err := normalizeOptionUpdateValue("WechatMinTopUp", strconv.Itoa(*request.MinTopUp), "")
		if err != nil {
			return nil, err
		}
		minTopUp, _ = strconv.Atoi(normalized)
	}
	unitPrice := strconv.FormatFloat(currentSettings.UnitPrice, 'f', -1, 64)
	if request.UnitPrice != nil {
		var err error
		unitPrice, err = normalizeOptionUpdateValue(
			"WechatUnitPrice",
			strconv.FormatFloat(*request.UnitPrice, 'f', -1, 64),
			"",
		)
		if err != nil {
			return nil, err
		}
	}
	// Disabling an active gateway must remain possible even when retained
	// credentials or callback URLs have since expired. Preserve the old values
	// so in-flight callbacks can still be verified after checkout is hidden.
	if !*request.Enabled && currentSettings.Enabled {
		values := map[string]string{"WechatEnabled": "false"}
		if request.MinTopUp != nil {
			values["WechatMinTopUp"] = strconv.Itoa(minTopUp)
		}
		if request.UnitPrice != nil {
			values["WechatUnitPrice"] = unitPrice
		}
		return values, nil
	}
	if request.OrderExpireMinutes == nil {
		return nil, errors.New("wechat order expiration is required")
	}

	identifierValues := []struct {
		key   string
		value string
	}{
		{key: "WechatMchID", value: request.MchID},
		{key: "WechatAppID", value: request.AppID},
		{key: "WechatCertificateSerialNumber", value: request.CertificateSerialNumber},
		{key: "WechatPayPublicKeyID", value: request.PayPublicKeyID},
	}
	normalizedIdentifiers := make(map[string]string, len(identifierValues))
	for _, field := range identifierValues {
		value := strings.TrimSpace(field.value)
		if value == "" {
			normalizedIdentifiers[field.key] = ""
			continue
		}
		normalized, err := normalizeOptionUpdateValue(field.key, value, "")
		if err != nil {
			return nil, err
		}
		if field.key == "WechatCertificateSerialNumber" {
			normalized = strings.ToUpper(normalized)
		}
		normalizedIdentifiers[field.key] = normalized
	}

	notifyURL, err := normalizeOptionUpdateValue("WechatNotifyURL", request.NotifyURL, "")
	if err != nil {
		return nil, err
	}
	expireMinutes, err := normalizeOptionUpdateValue("WechatOrderExpireMinutes", strconv.Itoa(*request.OrderExpireMinutes), "")
	if err != nil {
		return nil, err
	}

	sensitiveValues := []struct {
		key   string
		value string
	}{
		{key: "WechatApiV3Key", value: request.ApiV3Key},
		{key: "WechatPrivateKey", value: request.PrivateKey},
		{key: "WechatMerchantCertificate", value: request.MerchantCertificate},
		{key: "WechatPayPublicKey", value: request.PayPublicKey},
		{key: "WechatPlatformCertificate", value: request.PlatformCertificate},
	}
	normalizedSensitive := make(map[string]string, len(sensitiveValues))
	for _, field := range sensitiveValues {
		normalized, normalizeErr := normalizeOptionUpdateValue(field.key, field.value, "")
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		normalizedSensitive[field.key] = normalized
	}

	config := currentSettings.Config
	config.MchID = normalizedIdentifiers["WechatMchID"]
	config.AppID = normalizedIdentifiers["WechatAppID"]
	config.CertificateSerialNumber = normalizedIdentifiers["WechatCertificateSerialNumber"]
	if normalizedSensitive["WechatApiV3Key"] != "" {
		config.ApiV3Key = normalizedSensitive["WechatApiV3Key"]
	}
	if normalizedSensitive["WechatPrivateKey"] != "" {
		config.PrivateKey = normalizedSensitive["WechatPrivateKey"]
	}
	if normalizedSensitive["WechatMerchantCertificate"] != "" {
		config.MerchantCertificate = normalizedSensitive["WechatMerchantCertificate"]
	}
	config.PayPublicKeyID = normalizedIdentifiers["WechatPayPublicKeyID"]
	if config.PayPublicKeyID == "" {
		if normalizedSensitive["WechatPayPublicKey"] != "" {
			return nil, errors.New("wechat pay public key ID is required when a public key is supplied")
		}
		config.PayPublicKey = ""
	} else if normalizedSensitive["WechatPayPublicKey"] != "" {
		config.PayPublicKey = normalizedSensitive["WechatPayPublicKey"]
	}
	if normalizedSensitive["WechatPlatformCertificate"] != "" {
		config.PlatformCertificate = normalizedSensitive["WechatPlatformCertificate"]
	}

	if *request.Enabled {
		if !wechatCredentialsPresent(config) {
			return nil, errors.New("wechat pay credentials are incomplete")
		}
		if _, err := service.NewWechatPayClient(config); err != nil {
			return nil, errors.New("wechat pay credentials are invalid or do not match")
		}
		if _, err := resolveWechatNotifyURL(notifyURL); err != nil {
			return nil, err
		}
	}

	values := map[string]string{
		"WechatEnabled":                 strconv.FormatBool(*request.Enabled),
		"WechatMchID":                   config.MchID,
		"WechatAppID":                   config.AppID,
		"WechatApiV3Key":                config.ApiV3Key,
		"WechatCertificateSerialNumber": config.CertificateSerialNumber,
		"WechatPrivateKey":              config.PrivateKey,
		"WechatMerchantCertificate":     config.MerchantCertificate,
		"WechatPayPublicKeyID":          config.PayPublicKeyID,
		"WechatPayPublicKey":            config.PayPublicKey,
		"WechatPlatformCertificate":     config.PlatformCertificate,
		"WechatNotifyURL":               notifyURL,
		"WechatOrderExpireMinutes":      expireMinutes,
		"WechatUnitPrice":               unitPrice,
	}
	if request.MinTopUp != nil {
		values["WechatMinTopUp"] = strconv.Itoa(minTopUp)
	}
	return values, nil
}

func UpdateWechatPaymentSettings(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, wechatSettingsRequestMaxBytes)
	var request wechatSettingsUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "微信支付设置参数无效"})
		return
	}
	values, err := buildWechatSettingsUpdate(request)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		logger.LogError(c.Request.Context(), "微信支付设置保存失败: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "微信支付设置保存失败"})
		return
	}
	common.ApiSuccess(c, nil)
}
