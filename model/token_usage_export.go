package model

import (
	"context"
	"errors"
	"sort"
)

type TokenUsageExportRow struct {
	TokenID                                         int
	TokenName                                       string
	ModelName                                       string
	Requests, PromptTokens, CompletionTokens, Quota int64
	FirstRequest, LastRequest                       int64
}

// Query only names and IDs from the token table: secrets cannot accidentally
// reach workbook serialization, even if its column list changes later.
func GetTokenUsageExport(ctx context.Context, userID int, keyword string, start, end int64) ([]TokenUsageExportRow, error) {
	var tokens []struct {
		ID   int
		Name string
	}
	tx := DB.WithContext(ctx).Model(&Token{}).Where("user_id = ?", userID)
	if keyword != "" {
		tx = tx.Where("name LIKE ? ESCAPE '!'", tokenUsageNameLikePattern(keyword))
	}
	if err := tx.Select("id", "name").Order("id").Limit(10001).Find(&tokens).Error; err != nil {
		return nil, err
	}
	if len(tokens) > 10000 {
		return nil, errors.New("Too many keys; narrow the name filter")
	}
	rows := make([]TokenUsageExportRow, 0)
	for offset := 0; offset < len(tokens); offset += 400 {
		stop := min(offset+400, len(tokens))
		ids := make([]int, 0, stop-offset)
		names := map[int]string{}
		for _, token := range tokens[offset:stop] {
			ids = append(ids, token.ID)
			names[token.ID] = token.Name
		}
		var batch []TokenUsageExportRow
		query := getTokenUsageLogBase(userID, ids, start, end).WithContext(ctx)
		if err := query.Select("token_id, model_name, COUNT(*) AS requests, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(quota),0) AS quota, MIN(created_at) AS first_request, MAX(created_at) AS last_request").Group("token_id, model_name").Limit(10001).Scan(&batch).Error; err != nil {
			return nil, err
		}
		if len(rows)+len(batch) > 10000 {
			return nil, errors.New("Too many rows; narrow the export filters")
		}
		seen := map[int]bool{}
		for i := range batch {
			batch[i].TokenName = names[batch[i].TokenID]
			seen[batch[i].TokenID] = true
		}
		rows = append(rows, batch...)
		for _, token := range tokens[offset:stop] {
			if !seen[token.ID] {
				rows = append(rows, TokenUsageExportRow{TokenID: token.ID, TokenName: token.Name})
			}
		}
		if len(rows) > 10000 {
			return nil, errors.New("Too many rows; narrow the export filters")
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TokenID != rows[j].TokenID {
			return rows[i].TokenID < rows[j].TokenID
		}
		return rows[i].ModelName < rows[j].ModelName
	})
	return rows, nil
}
