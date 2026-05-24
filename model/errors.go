package model

import "errors"

// Common errors
var (
	ErrDatabase = errors.New("database error")
)

// User auth errors
var (
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrUserEmptyCredentials = errors.New("empty credentials")
)

// Token auth errors
var (
	ErrTokenNotProvided = errors.New("token not provided")
	ErrTokenInvalid     = errors.New("token invalid")
)

// Redemption errors
var (
	ErrRedeemFailed              = errors.New("redeem.failed")
	ErrRedemptionInvalid         = errors.New("redemption.invalid")
	ErrRedemptionUsed            = errors.New("redemption.used")
	ErrRedemptionExpired         = errors.New("redemption.expired")
	ErrRedemptionTypeInvalid     = errors.New("redemption.type_invalid")
	ErrRedemptionQuotaInvalid    = errors.New("redemption.quota_positive")
	ErrRedemptionPlanRequired    = errors.New("redemption.subscription_plan_required")
	ErrRedemptionPlanNotFound    = errors.New("redemption.subscription_plan_not_found")
	ErrRedemptionUsedImmutable   = errors.New("redemption.used_immutable")
)

// 2FA errors
var ErrTwoFANotEnabled = errors.New("2fa not enabled")
