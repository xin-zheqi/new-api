package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

const recentTokenRequestLimit = 20

// TokenRecentRequest exposes only the billing and token counts needed by the
// public key-lookup page. Log content, IPs, channel details and raw metadata
// must never be returned by this API.
type TokenRecentRequest struct {
	RequestId          string `json:"request_id"`
	CreatedAt          int64  `json:"created_at"`
	ModelName          string `json:"model_name"`
	PromptTokens       int    `json:"prompt_tokens"`
	CompletionTokens   int    `json:"completion_tokens"`
	CacheReadTokens    int    `json:"cache_read_tokens"`
	CacheWriteTokens   int    `json:"cache_write_tokens"`
	CacheWrite5mTokens int    `json:"cache_write_5m_tokens"`
	CacheWrite1hTokens int    `json:"cache_write_1h_tokens"`
	TextInputTokens    int    `json:"text_input_tokens"`
	TextOutputTokens   int    `json:"text_output_tokens"`
	AudioInputTokens   int    `json:"audio_input_tokens"`
	AudioOutputTokens  int    `json:"audio_output_tokens"`
	Quota              int    `json:"quota"`
}

type tokenRequestLogDetails struct {
	CacheReadTokens    int `json:"cache_tokens"`
	CacheWriteTokens   int `json:"cache_creation_tokens"`
	CacheWrite5mTokens int `json:"cache_creation_tokens_5m"`
	CacheWrite1hTokens int `json:"cache_creation_tokens_1h"`
	TextInputTokens    int `json:"text_input"`
	TextOutputTokens   int `json:"text_output"`
	AudioInputTokens   int `json:"audio_input"`
	AudioOutputTokens  int `json:"audio_output"`
}

func GetRecentTokenRequests(ctx context.Context, userID, tokenID int) ([]TokenRecentRequest, error) {
	requests := make([]TokenRecentRequest, 0)
	if userID <= 0 || tokenID <= 0 {
		return requests, nil
	}

	var logs []Log
	order := "created_at desc, id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("")
	}
	err := LOG_DB.WithContext(ctx).Model(&Log{}).
		Select("request_id, created_at, model_name, quota, prompt_tokens, completion_tokens, other").
		Where("user_id = ? AND token_id = ? AND type = ?", userID, tokenID, LogTypeConsume).
		Order(order).Limit(recentTokenRequestLimit).Find(&logs).Error
	if err != nil {
		return nil, err
	}

	for _, log := range logs {
		request := TokenRecentRequest{
			RequestId:        log.RequestId,
			CreatedAt:        log.CreatedAt,
			ModelName:        log.ModelName,
			PromptTokens:     log.PromptTokens,
			CompletionTokens: log.CompletionTokens,
			Quota:            log.Quota,
		}
		var details tokenRequestLogDetails
		if err := common.UnmarshalJsonStr(log.Other, &details); err == nil {
			request.CacheReadTokens = details.CacheReadTokens
			request.CacheWriteTokens = details.CacheWriteTokens
			request.CacheWrite5mTokens = details.CacheWrite5mTokens
			request.CacheWrite1hTokens = details.CacheWrite1hTokens
			request.TextInputTokens = details.TextInputTokens
			request.TextOutputTokens = details.TextOutputTokens
			request.AudioInputTokens = details.AudioInputTokens
			request.AudioOutputTokens = details.AudioOutputTokens
		}
		requests = append(requests, request)
	}
	return requests, nil
}
