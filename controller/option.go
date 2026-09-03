package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

var completionRatioMetaOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"CacheRatio",
	"CreateCacheRatio",
	"ImageRatio",
	"AudioRatio",
	"AudioCompletionRatio",
}

const mallSettingsRequestMaxBytes int64 = 8 * 1024

// isSensitiveOptionKey keeps credentials out of the admin option response.
// Values are still accepted by the update endpoint; clients should leave a
// blank password field unchanged unless they explicitly rotate it.
func isSensitiveOptionKey(key string) bool {
	for _, suffix := range []string{
		"Token", "Secret", "Key", "secret", "api_key",
	} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return key == "WechatMerchantCertificate" || key == "WechatPlatformCertificate"
}

type mallSettingsUpdateRequest struct {
	MallEnabled *bool   `json:"mall_enabled"`
	MallURL     *string `json:"mall_url"`
}

func isPaymentComplianceOptionKey(key string) bool {
	return strings.HasPrefix(key, "payment_setting.compliance_")
}

func isPositiveOptionValue(value string) bool {
	intValue, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		return intValue > 0
	}
	floatValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && floatValue > 0
}

func normalizeOptionUpdateValue(key, value, applicationHost string) (string, error) {
	switch key {
	case "MallURL":
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return "", nil
		}
		normalized := safeMallURL(trimmed, applicationHost)
		if normalized == "" {
			return "", fmt.Errorf("MallURL must be an external HTTPS URL without credentials and no longer than %d characters", maxMallURLLength)
		}
		return normalized, nil
	case "payment_setting.mall_enabled", "InvoiceEnabled":
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized != "true" && normalized != "false" {
			return "", fmt.Errorf("%s must be true or false", key)
		}
		return normalized, nil
	case "WechatEnabled":
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized != "true" && normalized != "false" {
			return "", fmt.Errorf("%s must be true or false", key)
		}
		return normalized, nil
	case "WechatOrderExpireMinutes":
		return normalizeBoundedIntegerOption(key, value, 1, 120)
	case "WechatNotifyURL":
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return "", nil
		}
		if len([]byte(trimmed)) > wechatNotifyURLMaxBytes {
			return "", fmt.Errorf("WechatNotifyURL is too long")
		}
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
			!strings.EqualFold(parsed.Scheme, "https") {
			return "", fmt.Errorf("WechatNotifyURL must be a valid HTTPS URL without credentials or query parameters")
		}
		return strings.TrimRight(trimmed, "/"), nil
	case "WechatMchID", "WechatAppID", "WechatCertificateSerialNumber", "WechatPayPublicKeyID":
		trimmed := strings.TrimSpace(value)
		if len(trimmed) == 0 || len(trimmed) > 128 {
			return "", fmt.Errorf("%s is invalid", key)
		}
		return trimmed, nil
	case "WechatApiV3Key", "WechatPrivateKey", "WechatMerchantCertificate", "WechatPayPublicKey", "WechatPlatformCertificate":
		trimmed := strings.TrimSpace(value)
		if len(trimmed) > 64*1024 {
			return "", fmt.Errorf("%s is too long", key)
		}
		return trimmed, nil
	case "InvoiceApplicationDay":
		return normalizeBoundedIntegerOption(key, value, 1, 28)
	case "InvoiceLookbackDays":
		return normalizeBoundedIntegerOption(key, value, 1, 3650)
	case "InvoiceMonthlyLimit":
		return normalizeBoundedIntegerOption(key, value, 1, 31)
	default:
		return value, nil
	}
}

func normalizeBoundedIntegerOption(key, value string, minimum, maximum int) (string, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < minimum || parsed > maximum {
		return "", fmt.Errorf("%s must be an integer between %d and %d", key, minimum, maximum)
	}
	return strconv.Itoa(parsed), nil
}

func normalizeMallSettingsUpdate(request mallSettingsUpdateRequest, applicationHost string) (map[string]string, error) {
	if request.MallEnabled == nil || request.MallURL == nil {
		return nil, fmt.Errorf("mall enabled and mall URL are required")
	}
	mallURL, err := normalizeOptionUpdateValue("MallURL", *request.MallURL, applicationHost)
	if err != nil {
		if *request.MallEnabled {
			return nil, err
		}
		mallURL = ""
	}
	if *request.MallEnabled && mallURL == "" {
		return nil, fmt.Errorf("MallURL is required when the card-store mall is enabled")
	}
	return map[string]string{
		"payment_setting.mall_enabled": strconv.FormatBool(*request.MallEnabled),
		"MallURL":                      mallURL,
	}, nil
}

func UpdateMallSettings(c *gin.Context) {
	var request mallSettingsUpdateRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, mallSettingsRequestMaxBytes)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid mall settings"})
		return
	}
	values, err := normalizeMallSettingsUpdate(request, c.Request.Host)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.SysError("failed to update mall settings: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to update mall settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func collectModelNamesFromOptionValue(raw string, modelNames map[string]struct{}) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	var parsed map[string]any
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return
	}

	for modelName := range parsed {
		modelNames[modelName] = struct{}{}
	}
}

func buildCompletionRatioMetaValue(optionValues map[string]string) string {
	modelNames := make(map[string]struct{})
	for _, key := range completionRatioMetaOptionKeys {
		collectModelNamesFromOptionValue(optionValues[key], modelNames)
	}

	meta := make(map[string]ratio_setting.CompletionRatioInfo, len(modelNames))
	for modelName := range modelNames {
		meta[modelName] = ratio_setting.GetCompletionRatioInfo(modelName)
	}

	jsonBytes, err := common.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func GetOptions(c *gin.Context) {
	var options []*model.Option
	optionValues := make(map[string]string)
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		value := common.Interface2String(v)
		if isSensitiveOptionKey(k) {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: value,
		})
		for _, optionKey := range completionRatioMetaOptionKeys {
			if optionKey == k {
				optionValues[k] = value
				break
			}
		}
	}
	common.OptionMapRWMutex.Unlock()
	options = append(options, &model.Option{
		Key:   "CompletionRatioMeta",
		Value: buildCompletionRatioMetaValue(optionValues),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
}

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func UpdateOption(c *gin.Context) {
	var option OptionUpdateRequest
	err := common.DecodeJson(c.Request.Body, &option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if option.Key == "MallURL" || option.Key == "payment_setting.mall_enabled" {
		common.ApiErrorMsg(c, "商城设置必须通过专用接口整体修改")
		return
	}
	if isWechatPaymentOptionKey(option.Key) {
		common.ApiErrorMsg(c, "微信支付设置必须通过专用接口整体修改")
		return
	}
	switch option.Value.(type) {
	case bool:
		option.Value = common.Interface2String(option.Value.(bool))
	case float64:
		option.Value = common.Interface2String(option.Value.(float64))
	case int:
		option.Value = common.Interface2String(option.Value.(int))
	default:
		option.Value = fmt.Sprintf("%v", option.Value)
	}
	switch option.Key {
	case "QuotaForInviter", "QuotaForInvitee":
		if isPositiveOptionValue(option.Value.(string)) && !operation_setting.IsPaymentComplianceConfirmed() {
			common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
			return
		}
	default:
		if isPaymentComplianceOptionKey(option.Key) {
			common.ApiErrorMsg(c, "合规确认字段不允许通过通用设置接口修改")
			return
		}
	}
	option.Value, err = normalizeOptionUpdateValue(option.Key, option.Value.(string), c.Request.Host)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if option.Key == "TaskPublicAddress" && option.Value.(string) != "" {
		if err := service.ValidateTaskArtifactBaseURL(option.Value.(string)); err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
	}
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！",
			})
			return
		}
	case "discord.enabled":
		if option.Value == "true" && system_setting.GetDiscordSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Discord OAuth，请先填入 Discord Client Id 以及 Discord Client Secret！",
			})
			return
		}
	case "oidc.enabled":
		if option.Value == "true" && system_setting.GetOIDCSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！",
			})
			return
		}
	case "LinuxDOOAuthEnabled":
		if option.Value == "true" && common.LinuxDOClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！",
			})
			return
		}
	case "EmailDomainRestrictionEnabled":
		if option.Value == "true" && len(common.EmailDomainWhitelist) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用邮箱域名限制，请先填入限制的邮箱域名！",
			})
			return
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用微信登录，请先填入微信登录相关配置信息！",
			})
			return
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！",
			})

			return
		}
	case "TelegramOAuthEnabled":
		if option.Value == "true" && common.TelegramBotToken == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Telegram OAuth，请先填入 Telegram Bot Token！",
			})
			return
		}
	case "theme.frontend":
		if option.Value != "default" && option.Value != "classic" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无效的主题值，可选值：default（新版前端）、classic（经典前端）",
			})
			return
		}
	case "token_setting.max_group_count":
		value, convErr := strconv.Atoi(strings.TrimSpace(option.Value.(string)))
		if convErr != nil || value < operation_setting.MinMaxGroupCount || value > operation_setting.MaxMaxGroupCount {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("令牌最大分组数必须在 %d 到 %d 之间", operation_setting.MinMaxGroupCount, operation_setting.MaxMaxGroupCount),
			})
			return
		}
	case "GroupRatio":
		err = ratio_setting.CheckGroupRatio(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "gemini.safety_settings":
		err = model_setting.ValidateGeminiSafetySettings(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "claude.default_max_tokens":
		err = model_setting.ValidateClaudeDefaultMaxTokens(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case operation_setting.ToolPriceOptionKey:
		err = operation_setting.ValidateToolPricesJSON(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "ModelRequestRateLimitGroup":
		err = setting.CheckModelRequestRateLimitGroup(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "AutomaticDisableStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "AutomaticRetryStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "monitor_setting.auto_test_channel_time_range":
		_, _, err = operation_setting.ParseAutoTestChannelTimeRange(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "monitor_setting.channel_test_mode":
		value := strings.TrimSpace(option.Value.(string))
		if value != operation_setting.ChannelTestModeScheduledAll &&
			value != operation_setting.ChannelTestModeAutoBanOnly &&
			value != operation_setting.ChannelTestModePassiveRecovery {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "通道测试模式不正确",
			})
			return
		}
	case "monitor_setting.auto_test_only_auto_disabled":
		if _, err = strconv.ParseBool(option.Value.(string)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "仅测试自动禁用渠道设置格式不正确",
			})
			return
		}
	case "billing_setting.billing_expr":
		expressions := make(map[string]string)
		if err = common.UnmarshalJsonStr(option.Value.(string), &expressions); err != nil {
			common.ApiErrorMsg(c, "计费表达式配置必须是模型到表达式的 JSON 对象: "+err.Error())
			return
		}
		models := make([]string, 0, len(expressions))
		for modelName := range expressions {
			models = append(models, modelName)
		}
		sort.Strings(models)
		generation := jsplugin.DefaultRegistry.Generation()
		for _, modelName := range models {
			expression := expressions[modelName]
			if plugin, ok := generation.GetByModel(modelName); ok {
				err = billing_setting.SmokeTestTaskExpr(expression, plugin.Meta.UsageSchema)
			} else if target, resolved := model.ResolveTaskModelAlias(generation, modelName); resolved {
				if plugin, ok := generation.Get(target.PluginKey); ok {
					err = billing_setting.SmokeTestTaskExpr(expression, plugin.Meta.UsageSchema)
				} else {
					err = billing_setting.SmokeTestExpr(expression)
				}
			} else {
				err = billing_setting.SmokeTestExpr(expression)
			}
			if err != nil {
				common.ApiErrorMsg(c, fmt.Sprintf("模型 %s 的计费表达式无效: %v", modelName, err))
				return
			}
		}
	case "console_setting.api_info":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "ApiInfo")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.announcements":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "Announcements")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.faq":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "FAQ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.uptime_kuma_groups":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "UptimeKumaGroups")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	err = model.UpdateOption(option.Key, option.Value.(string))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	switch option.Key {
	case "monitor_setting.channel_test_mode":
		legacyValue := strconv.FormatBool(option.Value.(string) == operation_setting.ChannelTestModePassiveRecovery)
		if err = model.UpdateOption("monitor_setting.auto_test_only_auto_disabled", legacyValue); err != nil {
			common.ApiError(c, err)
			return
		}
	case "monitor_setting.auto_test_only_auto_disabled":
		onlyAutoDisabled, _ := strconv.ParseBool(option.Value.(string))
		mode := operation_setting.ChannelTestModeScheduledAll
		if onlyAutoDisabled {
			mode = operation_setting.ChannelTestModePassiveRecovery
		}
		if err = model.UpdateOption("monitor_setting.channel_test_mode", mode); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	// 出于安全考虑只记录被修改的配置项名称，不记录配置值（可能含密钥等敏感信息）。
	recordManageAudit(c, "option.update", map[string]interface{}{
		"key": option.Key,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
