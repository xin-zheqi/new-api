package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createSubscriptionExpiryTestUser(t *testing.T, username, group string) int {
	t.Helper()
	user := &User{
		Username: username,
		Password: "password",
		Group:    group,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(user).Error)
	return user.Id
}

func createSubscriptionExpiryTestSubscription(t *testing.T, sub UserSubscription) {
	t.Helper()
	require.NoError(t, DB.Create(&sub).Error)
}

func subscriptionExpiryTestUserGroup(t *testing.T, userId int) string {
	t.Helper()
	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", userId).Select(commonGroupCol).Find(&group).Error)
	return group
}

func TestExpireDueSubscriptionsRevertsExpiredChainToBaseGroup(t *testing.T) {
	truncateTables(t)

	userId := createSubscriptionExpiryTestUser(t, "subscription-chain-user", "enterprise")
	now := GetDBTimestamp()
	createSubscriptionExpiryTestSubscription(t, UserSubscription{
		UserId:        userId,
		PlanId:        1,
		EndTime:       now - 60,
		Status:        "active",
		UpgradeGroup:  "pro",
		PrevUserGroup: "basic",
	})
	createSubscriptionExpiryTestSubscription(t, UserSubscription{
		UserId:        userId,
		PlanId:        2,
		EndTime:       now - 30,
		Status:        "active",
		UpgradeGroup:  "enterprise",
		PrevUserGroup: "pro",
	})

	expired, err := ExpireDueSubscriptions(200)
	require.NoError(t, err)
	assert.Equal(t, 2, expired)
	assert.Equal(t, "basic", subscriptionExpiryTestUserGroup(t, userId))
}

func TestExpireDueSubscriptionsKeepsGroupWhenActiveUpgradeRemains(t *testing.T) {
	truncateTables(t)

	userId := createSubscriptionExpiryTestUser(t, "subscription-active-upgrade-user", "enterprise")
	now := GetDBTimestamp()
	createSubscriptionExpiryTestSubscription(t, UserSubscription{
		UserId:        userId,
		PlanId:        1,
		EndTime:       now - 60,
		Status:        "active",
		UpgradeGroup:  "pro",
		PrevUserGroup: "basic",
	})
	createSubscriptionExpiryTestSubscription(t, UserSubscription{
		UserId:        userId,
		PlanId:        2,
		EndTime:       now + 3600,
		Status:        "active",
		UpgradeGroup:  "enterprise",
		PrevUserGroup: "pro",
	})

	expired, err := ExpireDueSubscriptions(200)
	require.NoError(t, err)
	assert.Equal(t, 1, expired)
	assert.Equal(t, "enterprise", subscriptionExpiryTestUserGroup(t, userId))
}

func TestExpireDueSubscriptionsUsesLatestExplicitDowngradeGroup(t *testing.T) {
	truncateTables(t)

	userId := createSubscriptionExpiryTestUser(t, "subscription-explicit-downgrade-user", "enterprise")
	now := GetDBTimestamp()
	createSubscriptionExpiryTestSubscription(t, UserSubscription{
		UserId:         userId,
		PlanId:         1,
		EndTime:        now - 60,
		Status:         "active",
		UpgradeGroup:   "pro",
		PrevUserGroup:  "basic",
		DowngradeGroup: "basic",
	})
	createSubscriptionExpiryTestSubscription(t, UserSubscription{
		UserId:         userId,
		PlanId:         2,
		EndTime:        now - 30,
		Status:         "active",
		UpgradeGroup:   "enterprise",
		PrevUserGroup:  "pro",
		DowngradeGroup: "vip",
	})

	expired, err := ExpireDueSubscriptions(200)
	require.NoError(t, err)
	assert.Equal(t, 2, expired)
	assert.Equal(t, "vip", subscriptionExpiryTestUserGroup(t, userId))
}

func TestExpireDueSubscriptionsKeepsGroupForIncompleteChain(t *testing.T) {
	truncateTables(t)

	userId := createSubscriptionExpiryTestUser(t, "subscription-incomplete-chain-user", "enterprise")
	createSubscriptionExpiryTestSubscription(t, UserSubscription{
		UserId:        userId,
		PlanId:        1,
		EndTime:       GetDBTimestamp() - 60,
		Status:        "active",
		UpgradeGroup:  "pro",
		PrevUserGroup: "basic",
	})

	expired, err := ExpireDueSubscriptions(200)
	require.NoError(t, err)
	assert.Equal(t, 1, expired)
	assert.Equal(t, "enterprise", subscriptionExpiryTestUserGroup(t, userId))
}
