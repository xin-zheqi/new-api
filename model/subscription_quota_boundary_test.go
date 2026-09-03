package model

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSubscriptionQuotaOverflowHasNoSideEffects(t *testing.T) {
	const subscriptionID = 988001
	t.Cleanup(func() { DB.Where("id = ?", subscriptionID).Delete(&UserSubscription{}) })
	require.NoError(t, DB.Create(&UserSubscription{
		Id: subscriptionID, UserId: 988001, PlanId: 988001,
		AmountTotal: 0, AmountUsed: math.MaxInt64 - 1,
		StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(),
		Status: "active",
	}).Error)

	require.Error(t, PostConsumeUserSubscriptionDelta(subscriptionID, int64(common.MaxQuota)+1))
	require.Error(t, PostConsumeUserSubscriptionDelta(subscriptionID, 2))

	var stored UserSubscription
	require.NoError(t, DB.First(&stored, subscriptionID).Error)
	assert.Equal(t, int64(math.MaxInt64-1), stored.AmountUsed)
}

func TestSubscriptionPreConsumeOverflowCreatesNoIdempotencyRecord(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	const subscriptionID = 988002
	const userID = 988002
	const requestID = "subscription-overflow-no-record"
	plan := &SubscriptionPlan{
		Title: "quota-overflow-plan", PriceAmount: 1, Currency: "USD",
		DurationUnit: "month", DurationValue: 1, Enabled: true,
	}
	require.NoError(t, DB.Create(plan).Error)
	t.Cleanup(func() {
		DB.Where("request_id = ?", requestID).Delete(&SubscriptionPreConsumeRecord{})
		DB.Where("id = ?", subscriptionID).Delete(&UserSubscription{})
		DB.Delete(plan)
	})
	require.NoError(t, DB.Create(&UserSubscription{
		Id: subscriptionID, UserId: userID, PlanId: plan.Id,
		AmountTotal: 0, AmountUsed: math.MaxInt64 - 1,
		StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(),
		Status: "active",
	}).Error)

	result, err := PreConsumeUserSubscription(requestID, userID, "test-model", 0, 2)

	require.Error(t, err)
	assert.Nil(t, result)
	var recordCount int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", requestID).Count(&recordCount).Error)
	assert.Zero(t, recordCount)
	var stored UserSubscription
	require.NoError(t, DB.First(&stored, subscriptionID).Error)
	assert.Equal(t, int64(math.MaxInt64-1), stored.AmountUsed)
}

func TestRefundSubscriptionPreConsumeIsAtomic(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	const subscriptionID = 988003
	const requestID = "subscription-refund-atomic"
	require.NoError(t, DB.Create(&UserSubscription{
		Id: subscriptionID, UserId: 988003, PlanId: 988003,
		AmountTotal: 1_000, AmountUsed: 100,
		StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(),
		Status: "active",
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId: requestID, UserId: 988003, UserSubscriptionId: subscriptionID,
		PreConsumed: 40, Status: "consumed",
	}).Error)
	t.Cleanup(func() {
		DB.Where("request_id = ?", requestID).Delete(&SubscriptionPreConsumeRecord{})
		DB.Where("id = ?", subscriptionID).Delete(&UserSubscription{})
	})

	injectedErr := errors.New("injected pre-consume record update failure")
	const callbackName = "test:subscription_refund_atomicity"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "SubscriptionPreConsumeRecord" {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(callbackName)) })

	err := RefundSubscriptionPreConsume(requestID)

	require.ErrorIs(t, err, injectedErr)
	var storedSubscription UserSubscription
	require.NoError(t, DB.First(&storedSubscription, subscriptionID).Error)
	assert.Equal(t, int64(100), storedSubscription.AmountUsed)
	var storedRecord SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", requestID).First(&storedRecord).Error)
	assert.Equal(t, "consumed", storedRecord.Status)
}
