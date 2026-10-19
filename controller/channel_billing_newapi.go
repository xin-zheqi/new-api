package controller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// New API exposes quota units, not dollars. Match the upstream's balance
// semantics: a limited key can spend at most the smaller of its own quota and
// its owner's quota; an unlimited key still needs an owner's available quota.
func updateChannelNewAPIBalance(channel *model.Channel) (float64, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.RawQuery != "" || parsedURL.Fragment != "" || parsedURL.User != nil {
		return 0, errors.New("New API channel requires a valid HTTP(S) base URL without credentials, query or fragment")
	}
	key := strings.TrimSpace(channel.Key)
	if key == "" {
		return 0, errors.New("New API channel is missing an API key")
	}
	// A management query has a finite total lifetime, including reading both
	// bodies, even when the relay client's streaming timeout is unlimited.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := getChannelBalanceResponse(ctx, channel, baseURL+"/api/usage/token/", GetAuthHeader(key), "New API")
	if err != nil {
		return 0, err
	}
	var usage struct {
		Code    bool   `json:"code"`
		Message string `json:"message"`
		Data    *struct {
			TotalAvailable *float64 `json:"total_available"`
			UnlimitedQuota bool     `json:"unlimited_quota"`
			UserQuota      *struct {
				TotalAvailable *float64 `json:"total_available"`
			} `json:"user_quota"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &usage); err != nil {
		return 0, errors.New("New API upstream returned an invalid token usage response")
	}
	if !usage.Code {
		return 0, sanitizeFetchModelsError(fmt.Errorf("New API usage query failed: %s", usage.Message), key)
	}
	if usage.Data == nil {
		return 0, errors.New("New API token usage response is missing data")
	}
	remaining := usage.Data.TotalAvailable
	if usage.Data.UnlimitedQuota {
		remaining = nil
	}
	if owner := usage.Data.UserQuota; owner != nil {
		if owner.TotalAvailable == nil {
			return 0, errors.New("New API user quota response is missing total_available")
		}
		if usage.Data.UnlimitedQuota || (remaining != nil && *owner.TotalAvailable < *remaining) {
			remaining = owner.TotalAvailable
		}
	}
	if remaining == nil {
		if usage.Data.UnlimitedQuota {
			return 0, errors.New(`The upstream New API key has unlimited quota. Enable "Allow querying user quota" for this key on the upstream, or use a quota-limited key to query its balance.`)
		}
		return 0, errors.New("New API token usage response is missing total_available")
	}

	// /api/status is public: do not forward the channel's credential to it.
	body, err = getChannelBalanceResponse(ctx, channel, baseURL+"/api/status", nil, "New API")
	if err != nil {
		return 0, err
	}
	var status struct {
		Success bool `json:"success"`
		Data    struct {
			QuotaPerUnit float64 `json:"quota_per_unit"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &status); err != nil || !status.Success || status.Data.QuotaPerUnit <= 0 {
		return 0, errors.New("New API upstream status response is missing a valid quota_per_unit")
	}
	balance := *remaining / status.Data.QuotaPerUnit
	if math.IsNaN(balance) || math.IsInf(balance, 0) {
		return 0, errors.New("New API upstream returned an invalid balance")
	}
	// Only persist after both responses are validated. A missing field must not
	// become zero and cause the existing bulk updater to disable the channel.
	// Update a copy so a failed database write cannot alter a cached balance.
	updated := *channel
	if err := updated.UpdateBalance(balance); err != nil {
		return 0, err
	}
	channel.Balance, channel.BalanceUpdatedTime = updated.Balance, updated.BalanceUpdatedTime
	return balance, nil
}
