package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func seedTokenUsageToken(t *testing.T, token *Token) {
	t.Helper()
	require.NoError(t, DB.Create(token).Error)
}

func seedTokenUsageLog(t *testing.T, log *Log) {
	t.Helper()
	require.NoError(t, LOG_DB.Create(log).Error)
}

func TestGetUserTokenModelUsageAggregatesAndIsolates(t *testing.T) {
	truncateTables(t)

	userToken := &Token{UserId: 10, Name: "usage-key", Key: "sk-user-secret-123456", Status: 1, CreatedTime: 100, AccessedTime: 200, ExpiredTime: -1, RemainQuota: 900}
	emptyToken := &Token{UserId: 10, Name: "empty-key", Key: "sk-empty-secret-123456", Status: 1, CreatedTime: 101, ExpiredTime: -1}
	otherToken := &Token{UserId: 11, Name: "other-key", Key: "sk-other-secret-123456", Status: 1, CreatedTime: 102, ExpiredTime: -1}
	renamedLogToken := &Token{UserId: 10, Name: "renamed-key", Key: "sk-renamed-secret-123456", Status: 1, CreatedTime: 103, ExpiredTime: -1}
	seedTokenUsageToken(t, userToken)
	seedTokenUsageToken(t, emptyToken)
	seedTokenUsageToken(t, otherToken)
	seedTokenUsageToken(t, renamedLogToken)

	seedTokenUsageLog(t, &Log{UserId: 10, TokenId: userToken.Id, Type: LogTypeConsume, CreatedAt: 1000, TokenName: "old-name", ModelName: "gpt-4o-mini", Quota: 100, PromptTokens: 10, CompletionTokens: 20})
	seedTokenUsageLog(t, &Log{UserId: 10, TokenId: userToken.Id, Type: LogTypeConsume, CreatedAt: 1100, TokenName: "usage-key", ModelName: "gpt-4o-mini", Quota: 200, PromptTokens: 30, CompletionTokens: 40})
	seedTokenUsageLog(t, &Log{UserId: 10, TokenId: userToken.Id, Type: LogTypeConsume, CreatedAt: 1200, TokenName: "usage-key", ModelName: "claude-3-5", Quota: 50, PromptTokens: 5, CompletionTokens: 6})
	seedTokenUsageLog(t, &Log{UserId: 10, TokenId: renamedLogToken.Id, Type: LogTypeConsume, CreatedAt: 1200, TokenName: "old-renamed-key", ModelName: "gpt-4o-mini", Quota: 70, PromptTokens: 7, CompletionTokens: 8})
	seedTokenUsageLog(t, &Log{UserId: 10, TokenId: userToken.Id, Type: LogTypeManage, CreatedAt: 1200, ModelName: "gpt-4o-mini", Quota: 999, PromptTokens: 99, CompletionTokens: 99})
	seedTokenUsageLog(t, &Log{UserId: 10, TokenId: userToken.Id, Type: LogTypeConsume, CreatedAt: 900, ModelName: "out-of-range", Quota: 999, PromptTokens: 99, CompletionTokens: 99})
	seedTokenUsageLog(t, &Log{UserId: 11, TokenId: otherToken.Id, Type: LogTypeConsume, CreatedAt: 1200, ModelName: "leaked-model", Quota: 999, PromptTokens: 99, CompletionTokens: 99})

	items, total, summary, err := GetUserTokenModelUsage(10, "key", 1000, 1300, 0, 100)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.EqualValues(t, 3, summary.TotalKeyCount)
	require.EqualValues(t, 2, summary.ActiveKeyCount)
	require.EqualValues(t, 2, summary.ModelCount)
	require.EqualValues(t, 420, summary.Quota)
	require.EqualValues(t, 52, summary.PromptTokens)
	require.EqualValues(t, 74, summary.CompletionTokens)
	require.EqualValues(t, 126, summary.TotalTokens)
	require.EqualValues(t, 4, summary.Requests)

	byName := map[string]*TokenModelUsageItem{}
	for _, item := range items {
		byName[item.TokenName] = item
		require.NotContains(t, item.Key, "secret")
	}
	require.Equal(t, []string{"usage-key", "renamed-key", "empty-key"}, []string{items[0].TokenName, items[1].TokenName, items[2].TokenName})

	require.Contains(t, byName, "usage-key")
	require.EqualValues(t, 350, byName["usage-key"].Quota)
	require.EqualValues(t, 2, byName["usage-key"].ModelCount)
	require.Len(t, byName["usage-key"].Models, 2)

	require.Contains(t, byName, "renamed-key")
	require.EqualValues(t, 70, byName["renamed-key"].Quota)
	require.EqualValues(t, 1, byName["renamed-key"].ModelCount)

	require.Contains(t, byName, "empty-key")
	require.Zero(t, byName["empty-key"].Quota)
	require.Empty(t, byName["empty-key"].Models)
	require.NotContains(t, byName, "other-key")
}

func TestGetUserTokenModelUsagePaginationAndPageSizeCap(t *testing.T) {
	truncateTables(t)

	for i := 0; i < 120; i++ {
		seedTokenUsageToken(t, &Token{UserId: 20, Name: "page-key", Key: "sk-page-secret-" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Status: 1, ExpiredTime: -1})
	}

	items, total, _, err := GetUserTokenModelUsage(20, "page-key", 0, 0, 0, 500)
	require.NoError(t, err)
	require.EqualValues(t, 120, total)
	require.Len(t, items, 100)
}

func TestGetUserTokenModelUsageKeywordContainsAndUsageSort(t *testing.T) {
	truncateTables(t)

	lowToken := &Token{UserId: 30, Name: "alpha-low", Key: "sk-alpha-low-secret", Status: 1, ExpiredTime: -1}
	highToken := &Token{UserId: 30, Name: "prefix-alpha-high", Key: "sk-alpha-high-secret", Status: 1, ExpiredTime: -1}
	noUsageToken := &Token{UserId: 30, Name: "alpha-empty", Key: "sk-alpha-empty-secret", Status: 1, ExpiredTime: -1}
	otherToken := &Token{UserId: 30, Name: "beta-high", Key: "sk-beta-high-secret", Status: 1, ExpiredTime: -1}
	seedTokenUsageToken(t, lowToken)
	seedTokenUsageToken(t, highToken)
	seedTokenUsageToken(t, noUsageToken)
	seedTokenUsageToken(t, otherToken)

	seedTokenUsageLog(t, &Log{UserId: 30, TokenId: lowToken.Id, Type: LogTypeConsume, CreatedAt: 1000, ModelName: "gpt-4o-mini", Quota: 10})
	seedTokenUsageLog(t, &Log{UserId: 30, TokenId: highToken.Id, Type: LogTypeConsume, CreatedAt: 1000, ModelName: "gpt-4o-mini", Quota: 100})
	seedTokenUsageLog(t, &Log{UserId: 30, TokenId: otherToken.Id, Type: LogTypeConsume, CreatedAt: 1000, ModelName: "gpt-4o-mini", Quota: 200})

	items, total, _, err := GetUserTokenModelUsage(30, "alpha", 0, 0, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Equal(t, []string{"prefix-alpha-high", "alpha-low", "alpha-empty"}, []string{items[0].TokenName, items[1].TokenName, items[2].TokenName})

	items, total, _, err = GetUserTokenModelUsage(30, "alpha", 0, 0, 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, items, 1)
	require.Equal(t, "alpha-low", items[0].TokenName)
}
