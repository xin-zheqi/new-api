package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type tokenAutoGroupsInput struct {
	Set    bool
	Groups []string
}

func (input *tokenAutoGroupsInput) UnmarshalJSON(data []byte) error {
	input.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		input.Groups = nil
		return nil
	}
	return common.Unmarshal(data, &input.Groups)
}

type tokenRequest struct {
	model.Token
	AutoGroups tokenAutoGroupsInput `json:"auto_groups"`
}

type tokenResponse struct {
	*model.Token
	AutoGroups []string `json:"auto_groups"`
	// PeriodRemaining is calculated from the current calendar period so token
	// list responses do not show yesterday's usage after a reset boundary.
	PeriodRemaining int `json:"period_remaining"`
}

func maxTokenQuota() int {
	quota, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromInt(1_000_000_000).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if err != nil {
		return common.MaxWalletQuota
	}
	return quota
}

func normalizeAndValidateTokenQuota(c *gin.Context, token *model.Token) bool {
	period := model.NormalizeQuotaPeriod(token.QuotaPeriod)
	if token.QuotaPeriod != "" && period == "" {
		common.ApiError(c, fmt.Errorf("无效的周期额度类型"))
		return false
	}
	token.QuotaPeriod = period
	if period != "" {
		if token.QuotaPeriodLimit <= 0 || token.QuotaPeriodLimit > maxTokenQuota() {
			common.ApiError(c, fmt.Errorf("周期额度必须在 1 到 %d 之间", maxTokenQuota()))
			return false
		}
		// A periodic key has no legacy total quota. Keep the old fields in a
		// canonical state so older clients cannot accidentally consume both.
		token.UnlimitedQuota = true
		token.RemainQuota = 0
		token.QuotaPeriodUsed = 0
		token.QuotaPeriodStart = 0
		return true
	}
	token.QuotaPeriodLimit = 0
	token.QuotaPeriodUsed = 0
	token.QuotaPeriodStart = 0
	if token.UnlimitedQuota {
		token.RemainQuota = 0
		return true
	}
	if token.RemainQuota < 0 {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
		return false
	}
	if token.RemainQuota > maxTokenQuota() {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxTokenQuota()})
		return false
	}
	return true
}

func buildMaskedTokenResponse(token *model.Token) *tokenResponse {
	if token == nil {
		return nil
	}
	maskedToken := *token
	periodRemaining := model.GetTokenPeriodRemaining(&maskedToken)
	maskedToken.Key = token.GetMaskedKey()
	autoGroups, err := token.GetAutoGroups()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to parse auto groups for token %d: %v", token.Id, err))
		autoGroups = nil
	}
	if len(autoGroups) == 0 {
		autoGroups = nil
	}
	return &tokenResponse{
		Token:           &maskedToken,
		AutoGroups:      autoGroups,
		PeriodRemaining: periodRemaining,
	}
}

func buildMaskedTokenResponses(tokens []*model.Token) []*tokenResponse {
	maskedTokens := make([]*tokenResponse, 0, len(tokens))
	for _, token := range tokens {
		maskedTokens = append(maskedTokens, buildMaskedTokenResponse(token))
	}
	return maskedTokens
}

func normalizeAndValidateTokenGroup(c *gin.Context, token *model.Token) bool {
	if token == nil {
		return true
	}
	if len(token.Group) > model.MaxTokenGroupConfigLength {
		common.ApiError(c, fmt.Errorf("令牌分组配置过长"))
		return false
	}
	token.NormalizeGroup()
	if token.Group == "" {
		token.CrossGroupRetry = false
		return true
	}
	groups := token.GetGroups()
	maxGroupCount := operation_setting.GetMaxGroupCount()
	if len(groups) > maxGroupCount {
		common.ApiError(c, fmt.Errorf("令牌分组数量过多，最多允许 %d 个分组", maxGroupCount))
		return false
	}
	for _, group := range groups {
		if len(group) > model.MaxTokenGroupNameLength {
			common.ApiError(c, fmt.Errorf("分组 %s 名称过长", group))
			return false
		}
	}
	if token.IsMultiGroup() {
		for _, group := range groups {
			if group == "auto" {
				common.ApiError(c, fmt.Errorf("多分组令牌不能同时选择 auto 分组"))
				return false
			}
		}
	}
	if token.IsAutoGroup() {
		return true
	}
	userGroup := c.GetString("group")
	if userGroup == "" {
		userGroup = c.GetString("user_group")
	}
	if userGroup == "" {
		if dbGroup, err := model.GetUserGroup(c.GetInt("id"), false); err == nil {
			userGroup = dbGroup
		}
	}
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for _, group := range groups {
		if _, ok := userUsableGroups[group]; !ok {
			common.ApiError(c, fmt.Errorf("无权访问 %s 分组", group))
			return false
		}
		if !ratio_setting.ContainsGroupRatio(group) && group != "auto" {
			common.ApiError(c, fmt.Errorf("分组 %s 已被弃用", group))
			return false
		}
	}
	if !token.IsAutoGroup() && !token.IsMultiGroup() {
		token.CrossGroupRetry = false
	}
	return true
}

func getTokenRequestUserGroup(c *gin.Context) (string, error) {
	if userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup); userGroup != "" {
		return userGroup, nil
	}
	if userGroup := c.GetString("group"); userGroup != "" {
		return userGroup, nil
	}
	return model.GetUserGroup(c.GetInt("id"), false)
}

func setTokenAutoGroups(c *gin.Context, token *model.Token, groups []string) bool {
	if len(groups) == 0 {
		if err := token.SetAutoGroups(nil); err != nil {
			common.ApiError(c, err)
			return false
		}
		return true
	}

	maxCount := setting.GetMaxTokenAutoGroups()
	if len(groups) > maxCount {
		common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsTooMany, map[string]any{"Max": maxCount})
		return false
	}

	userGroup, err := getTokenRequestUserGroup(c)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if _, ok := seen[group]; ok {
			common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsDuplicate, map[string]any{"Group": group})
			return false
		}
		seen[group] = struct{}{}
		if !service.IsUserSelectableGroup(userGroup, group) {
			common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsInvalid, map[string]any{"Group": group})
			return false
		}
	}

	if err := token.SetAutoGroups(groups); err != nil {
		common.ApiError(c, err)
		return false
	}
	return true
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")

	pageInfo := common.GetPageQuery(c)

	tokens, total, err := model.SearchUserTokens(userId, keyword, token, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

func GetTokenModelUsage(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	keyword := c.Query("keyword")

	items, total, summary, err := model.GetUserTokenModelUsage(userId, keyword, startTimestamp, endTimestamp, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, gin.H{
		"page":    pageInfo,
		"summary": summary,
	})
}

func GetTokenAutoGroups(c *gin.Context) {
	userGroup, err := getTokenRequestUserGroup(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"groups":    service.GetUserAutoGroup(userGroup),
		"max_count": setting.GetMaxTokenAutoGroups(),
	})
}

func GetTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key": token.GetFullKey(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	periodRemaining := model.GetTokenPeriodRemaining(token)
	c.JSON(http.StatusOK, gin.H{
		"object":           "credit_summary",
		"total_granted":    token.RemainQuota,
		"total_used":       0, // not supported currently
		"total_available":  token.RemainQuota,
		"quota_period":     token.QuotaPeriod,
		"period_limit":     token.QuotaPeriodLimit,
		"period_used":      token.QuotaPeriodUsed,
		"period_remaining": periodRemaining,
		"period_start":     token.QuotaPeriodStart,
		"expires_at":       expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	// This endpoint is public: TokenAuthReadOnly authenticates the API key and
	// sets its owner without requiring a browser login or session cookie.
	tokenID, userID, tokenKey := c.GetInt("token_id"), c.GetInt("id"), c.GetString("token_key")
	if tokenID <= 0 || userID <= 0 || tokenKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid Bearer token"})
		return
	}
	// Keep the cache-aware lookup used by legacy balance queries, while using
	// the normalized key from middleware so group suffixes resolve correctly.
	token, err := model.GetTokenByKey(tokenKey, false)
	if err != nil {
		common.SysError("failed to get token by key: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}
	if token.Id != tokenID || token.UserId != userID {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid Bearer token"})
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	periodRemaining := model.GetTokenPeriodRemaining(token)
	recentRequests, recentRequestsErr := model.GetRecentTokenRequests(c.Request.Context(), token.UserId, token.Id)
	if recentRequestsErr != nil {
		common.SysError("failed to get recent token requests: " + recentRequestsErr.Error())
		recentRequests = []model.TokenRecentRequest{}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":                "token_usage",
			"name":                  token.Name,
			"total_granted":         token.RemainQuota + token.UsedQuota,
			"total_used":            token.UsedQuota,
			"total_available":       token.RemainQuota,
			"unlimited_quota":       token.UnlimitedQuota,
			"model_limits":          token.GetModelLimitsMap(),
			"model_limits_enabled":  token.ModelLimitsEnabled,
			"expires_at":            expiredAt,
			"quota_period":          token.QuotaPeriod,
			"period_limit":          token.QuotaPeriodLimit,
			"period_used":           token.QuotaPeriodUsed,
			"period_remaining":      periodRemaining,
			"period_start":          token.QuotaPeriodStart,
			"recent_requests":       recentRequests,
			"recent_requests_error": recentRequestsErr != nil,
		},
	})
}

func AddToken(c *gin.Context) {
	request := tokenRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token := request.Token
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if !normalizeAndValidateTokenGroup(c, &token) {
		return
	}
	// 非无限额度时，检查额度值是否超出有效范围
	if !normalizeAndValidateTokenQuota(c, &token) {
		return
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= maxTokens {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens),
		})
		return
	}
	if token.Group == "auto" {
		if !setTokenAutoGroups(c, &token, request.AutoGroups.Groups) {
			return
		}
	} else {
		token.CrossGroupRetry = false
		_ = token.SetAutoGroups(nil)
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	cleanToken := model.Token{
		UserId:             c.GetInt("id"),
		Name:               token.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		QuotaPeriod:        token.QuotaPeriod,
		QuotaPeriodLimit:   token.QuotaPeriodLimit,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		Group:              token.Group,
		CrossGroupRetry:    token.CrossGroupRetry,
		AutoGroups:         token.AutoGroups,
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	request := tokenRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token := request.Token
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if !normalizeAndValidateTokenGroup(c, &token) {
		return
	}
	if !normalizeAndValidateTokenQuota(c, &token) {
		return
	}
	cleanToken, err := model.GetTokenByIds(token.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if token.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			common.ApiErrorI18n(c, i18n.MsgTokenExpiredCannotEnable)
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			common.ApiErrorI18n(c, i18n.MsgTokenExhaustedCannotEable)
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		// If you add more fields, please also update token.Update()
		cleanToken.Name = token.Name
		cleanToken.ExpiredTime = token.ExpiredTime
		cleanToken.RemainQuota = token.RemainQuota
		cleanToken.UnlimitedQuota = token.UnlimitedQuota
		if cleanToken.QuotaPeriod != token.QuotaPeriod || cleanToken.QuotaPeriodLimit != token.QuotaPeriodLimit {
			cleanToken.QuotaPeriodUsed = 0
			cleanToken.QuotaPeriodStart = 0
		}
		cleanToken.QuotaPeriod = token.QuotaPeriod
		cleanToken.QuotaPeriodLimit = token.QuotaPeriodLimit
		cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
		cleanToken.ModelLimits = token.ModelLimits
		cleanToken.AllowIps = token.AllowIps
		cleanToken.Group = token.Group
		cleanToken.CrossGroupRetry = token.CrossGroupRetry
		if token.Group != "auto" {
			cleanToken.CrossGroupRetry = false
			_ = cleanToken.SetAutoGroups(nil)
		} else if request.AutoGroups.Set {
			if !setTokenAutoGroups(c, cleanToken, request.AutoGroups.Groups) {
				return
			}
		}
	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(cleanToken),
	})
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

func GetTokenKeysBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(tokenBatch.Ids) > 100 {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": 100})
		return
	}
	userId := c.GetInt("id")
	tokens, err := model.GetTokenKeysByIds(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keysMap := make(map[int]string)
	for _, t := range tokens {
		keysMap[t.Id] = t.GetFullKey()
	}
	common.ApiSuccess(c, gin.H{"keys": keysMap})
}
