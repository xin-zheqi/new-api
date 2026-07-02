package model

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type TokenModelUsageModel struct {
	ModelName        string `json:"model_name"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	Requests         int64  `json:"requests"`
}

type TokenModelUsageItem struct {
	TokenId          int                     `json:"token_id"`
	TokenName        string                  `json:"token_name"`
	Key              string                  `json:"key"`
	Status           int                     `json:"status"`
	CreatedTime      int64                   `json:"created_time"`
	AccessedTime     int64                   `json:"accessed_time"`
	ExpiredTime      int64                   `json:"expired_time"`
	RemainQuota      int                     `json:"remain_quota"`
	UsedQuota        int                     `json:"used_quota"`
	UnlimitedQuota   bool                    `json:"unlimited_quota"`
	Quota            int64                   `json:"quota"`
	PromptTokens     int64                   `json:"prompt_tokens"`
	CompletionTokens int64                   `json:"completion_tokens"`
	TotalTokens      int64                   `json:"total_tokens"`
	Requests         int64                   `json:"requests"`
	ModelCount       int                     `json:"model_count"`
	Models           []*TokenModelUsageModel `json:"models"`
}

type TokenModelUsageSummary struct {
	TotalKeyCount    int64 `json:"total_key_count"`
	ActiveKeyCount   int64 `json:"active_key_count"`
	ModelCount       int64 `json:"model_count"`
	Quota            int64 `json:"quota"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	Requests         int64 `json:"requests"`
}

type tokenModelUsageAggregate struct {
	TokenId          int
	ModelName        string
	Quota            int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	Requests         int64
}

type tokenModelUsageTotalAggregate struct {
	TokenId          int
	Quota            int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	Requests         int64
}

type tokenModelUsageTokenRow struct {
	Id             int
	Name           string
	Key            string
	Status         int
	CreatedTime    int64
	AccessedTime   int64
	ExpiredTime    int64
	RemainQuota    int
	UsedQuota      int
	UnlimitedQuota bool
}

func tokenUsageNameLikePattern(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	keyword = strings.ReplaceAll(keyword, "!", "!!")
	keyword = strings.ReplaceAll(keyword, `%`, `!%`)
	keyword = strings.ReplaceAll(keyword, `_`, `!_`)
	return "%" + keyword + "%"
}

func getTokenUsageLogBase(userId int, tokenIds []int, startTimestamp int64, endTimestamp int64) (tx *gorm.DB) {
	tx = LOG_DB.Model(&Log{}).
		Where("user_id = ? AND type = ?", userId, LogTypeConsume)
	if len(tokenIds) > 0 {
		tx = tx.Where("token_id IN ?", tokenIds)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	return tx
}

func GetUserTokenModelUsage(userId int, keyword string, startTimestamp int64, endTimestamp int64, startIdx int, num int) ([]*TokenModelUsageItem, int64, TokenModelUsageSummary, error) {
	if userId == 0 {
		return nil, 0, TokenModelUsageSummary{}, errors.New("userId 无效")
	}
	if num <= 0 {
		num = common.ItemsPerPage
	}
	if num > 100 {
		num = 100
	}
	if startIdx < 0 {
		startIdx = 0
	}

	tokenQuery := DB.Model(&Token{}).Where("user_id = ?", userId)
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		tokenQuery = tokenQuery.Where("name LIKE ? ESCAPE '!'", tokenUsageNameLikePattern(keyword))
	}

	var total int64
	if err := tokenQuery.Count(&total).Error; err != nil {
		common.SysError("failed to count token model usage tokens: " + err.Error())
		return nil, 0, TokenModelUsageSummary{}, errors.New("查询令牌用量失败")
	}

	var tokens []tokenModelUsageTokenRow
	if err := tokenQuery.
		Select("id", "name", "key", "status", "created_time", "accessed_time", "expired_time", "remain_quota", "used_quota", "unlimited_quota").
		Order("id desc").
		Find(&tokens).Error; err != nil {
		common.SysError("failed to query token model usage tokens: " + err.Error())
		return nil, 0, TokenModelUsageSummary{}, errors.New("查询令牌用量失败")
	}

	items := make([]*TokenModelUsageItem, 0, len(tokens))
	tokenIds := make([]int, 0, len(tokens))
	itemByTokenId := make(map[int]*TokenModelUsageItem, len(tokens))
	for i := range tokens {
		token := tokens[i]
		item := &TokenModelUsageItem{
			TokenId:        token.Id,
			TokenName:      token.Name,
			Key:            MaskTokenKey(token.Key),
			Status:         token.Status,
			CreatedTime:    token.CreatedTime,
			AccessedTime:   token.AccessedTime,
			ExpiredTime:    token.ExpiredTime,
			RemainQuota:    token.RemainQuota,
			UsedQuota:      token.UsedQuota,
			UnlimitedQuota: token.UnlimitedQuota,
			Models:         []*TokenModelUsageModel{},
		}
		items = append(items, item)
		tokenIds = append(tokenIds, token.Id)
		itemByTokenId[token.Id] = item
	}

	if len(tokenIds) > 0 {
		var totalAggregates []*tokenModelUsageTotalAggregate
		err := getTokenUsageLogBase(userId, tokenIds, startTimestamp, endTimestamp).
			Select("token_id, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(completion_tokens), 0) AS completion_tokens, COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS total_tokens, COUNT(*) AS requests").
			Group("token_id").
			Scan(&totalAggregates).Error
		if err != nil {
			common.SysError("failed to aggregate token usage totals: " + err.Error())
			return nil, 0, TokenModelUsageSummary{}, errors.New("查询令牌用量失败")
		}
		for _, aggregate := range totalAggregates {
			item := itemByTokenId[aggregate.TokenId]
			if item == nil {
				continue
			}
			item.Quota = aggregate.Quota
			item.PromptTokens = aggregate.PromptTokens
			item.CompletionTokens = aggregate.CompletionTokens
			item.TotalTokens = aggregate.TotalTokens
			item.Requests = aggregate.Requests
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Quota != items[j].Quota {
			return items[i].Quota > items[j].Quota
		}
		return items[i].TokenId > items[j].TokenId
	})

	summary, err := getUserTokenModelUsageSummaryByTokenIds(userId, total, tokenIds, startTimestamp, endTimestamp)
	if err != nil {
		return nil, 0, TokenModelUsageSummary{}, err
	}

	endIdx := startIdx + num
	if startIdx > len(items) {
		startIdx = len(items)
	}
	if endIdx > len(items) {
		endIdx = len(items)
	}
	pageItems := items[startIdx:endIdx]
	pageTokenIds := make([]int, 0, len(pageItems))
	pageItemByTokenId := make(map[int]*TokenModelUsageItem, len(pageItems))
	for _, item := range pageItems {
		pageTokenIds = append(pageTokenIds, item.TokenId)
		pageItemByTokenId[item.TokenId] = item
	}

	if len(pageTokenIds) > 0 {
		var aggregates []*tokenModelUsageAggregate
		err := getTokenUsageLogBase(userId, pageTokenIds, startTimestamp, endTimestamp).
			Select("token_id, model_name, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(completion_tokens), 0) AS completion_tokens, COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS total_tokens, COUNT(*) AS requests").
			Group("token_id, model_name").
			Order("quota DESC").
			Scan(&aggregates).Error
		if err != nil {
			common.SysError("failed to aggregate token model usage: " + err.Error())
			return nil, 0, TokenModelUsageSummary{}, errors.New("查询令牌用量失败")
		}
		for _, aggregate := range aggregates {
			item := pageItemByTokenId[aggregate.TokenId]
			if item == nil {
				continue
			}
			item.Models = append(item.Models, &TokenModelUsageModel{
				ModelName:        aggregate.ModelName,
				Quota:            aggregate.Quota,
				PromptTokens:     aggregate.PromptTokens,
				CompletionTokens: aggregate.CompletionTokens,
				TotalTokens:      aggregate.TotalTokens,
				Requests:         aggregate.Requests,
			})
		}
		for _, item := range pageItems {
			item.ModelCount = len(item.Models)
		}
	}

	return pageItems, total, summary, nil
}

func GetUserTokenModelUsageSummary(userId int, keyword string, startTimestamp int64, endTimestamp int64) (TokenModelUsageSummary, error) {
	var summary TokenModelUsageSummary
	tokenQuery := DB.Model(&Token{}).Where("user_id = ?", userId)
	var tokenIds []int
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		tokenQuery = tokenQuery.Where("name LIKE ? ESCAPE '!'", tokenUsageNameLikePattern(keyword))
	}
	if err := tokenQuery.Count(&summary.TotalKeyCount).Error; err != nil {
		common.SysError("failed to count token model usage summary tokens: " + err.Error())
		return summary, errors.New("查询令牌用量失败")
	}
	if summary.TotalKeyCount == 0 {
		return summary, nil
	}
	if err := tokenQuery.Select("id").Find(&tokenIds).Error; err != nil {
		common.SysError("failed to query token model usage summary token ids: " + err.Error())
		return summary, errors.New("查询令牌用量失败")
	}
	if len(tokenIds) == 0 {
		return summary, nil
	}

	return getUserTokenModelUsageSummaryByTokenIds(userId, summary.TotalKeyCount, tokenIds, startTimestamp, endTimestamp)
}

func getUserTokenModelUsageSummaryByTokenIds(userId int, totalKeyCount int64, tokenIds []int, startTimestamp int64, endTimestamp int64) (TokenModelUsageSummary, error) {
	summary := TokenModelUsageSummary{TotalKeyCount: totalKeyCount}
	if totalKeyCount == 0 || len(tokenIds) == 0 {
		return summary, nil
	}
	base := getTokenUsageLogBase(userId, tokenIds, startTimestamp, endTimestamp)
	if err := base.
		Select("COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(completion_tokens), 0) AS completion_tokens, COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS total_tokens, COUNT(*) AS requests").
		Scan(&summary).Error; err != nil {
		common.SysError("failed to aggregate token model usage summary: " + err.Error())
		return summary, errors.New("查询令牌用量失败")
	}
	if err := getTokenUsageLogBase(userId, tokenIds, startTimestamp, endTimestamp).
		Distinct("token_id").
		Count(&summary.ActiveKeyCount).Error; err != nil {
		common.SysError("failed to count active token model usage tokens: " + err.Error())
		return summary, errors.New("查询令牌用量失败")
	}
	if err := getTokenUsageLogBase(userId, tokenIds, startTimestamp, endTimestamp).
		Where("model_name <> ?", "").
		Distinct("model_name").
		Count(&summary.ModelCount).Error; err != nil {
		common.SysError("failed to count token model usage models: " + err.Error())
		return summary, errors.New("查询令牌用量失败")
	}
	return summary, nil
}
