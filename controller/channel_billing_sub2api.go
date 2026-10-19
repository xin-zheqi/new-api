package controller

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func updateChannelSub2APIBalance(channel *model.Channel) (float64, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.RawQuery != "" || parsedURL.Fragment != "" || parsedURL.User != nil {
		return 0, errors.New("Sub2API channel requires a valid HTTP(S) base URL without credentials, query or fragment")
	}
	key := strings.TrimSpace(channel.Key)
	if key == "" {
		return 0, errors.New("Sub2API channel is missing an API key")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := getChannelBalanceResponse(ctx, channel, baseURL+"/v1/usage", GetAuthHeader(key), "Sub2API")
	if err != nil {
		return 0, err
	}
	var usage struct {
		Balance      *float64 `json:"balance"`
		Remaining    *float64 `json:"remaining"`
		IsValid      *bool    `json:"isValid"`
		Subscription *struct {
			DailyLimitUSD   *float64 `json:"daily_limit_usd"`
			WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
			MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
		} `json:"subscription"`
	}
	if err := common.Unmarshal(body, &usage); err != nil {
		// Do not echo an upstream body that may contain credentials or metadata.
		return 0, errors.New("Sub2API upstream returned an invalid usage response")
	}
	if usage.IsValid != nil && !*usage.IsValid {
		return 0, errors.New("Sub2API key is invalid (isValid=false)")
	}
	// Pointers distinguish missing/null amounts from an explicit zero balance.
	balance := usage.Remaining
	if balance == nil {
		balance = usage.Balance
	}
	if balance == nil {
		return 0, errors.New("Sub2API /v1/usage response is missing remaining and balance fields (the key may belong to a subscription group without an available balance)")
	}
	// Match upstream: remaining=-1 can represent unlimited subscription quota.
	// Detect that by its limits; a negative wallet balance is still legitimate.
	if subscription := usage.Subscription; subscription != nil && subscription.DailyLimitUSD == nil && subscription.WeeklyLimitUSD == nil && subscription.MonthlyLimitUSD == nil {
		return 0, errors.New("Sub2API /v1/usage reports an unlimited subscription without a finite balance")
	}
	// Only publish the new balance after persistence succeeds, so failed queries
	// or database writes cannot overwrite the cached value or trigger disabling.
	updated := *channel
	if err := updated.UpdateBalance(*balance); err != nil {
		return 0, err
	}
	channel.Balance, channel.BalanceUpdatedTime = updated.Balance, updated.BalanceUpdatedTime
	return *balance, nil
}
