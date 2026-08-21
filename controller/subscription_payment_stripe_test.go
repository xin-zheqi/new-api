package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionPlanPaymentSnapshotPreservesThreeDecimalCurrency(t *testing.T) {
	plan := &model.SubscriptionPlan{PriceAmount: 1.234, Currency: "KWD"}

	snapshot, err := subscriptionPlanPaymentSnapshot(plan)

	require.NoError(t, err)
	assert.Equal(t, int64(1_234_000), snapshot.AmountMicros)
	assert.Equal(t, "KWD", snapshot.Currency)
}

func TestSubscriptionPlanPaymentSnapshotPreservesSixDecimalPrice(t *testing.T) {
	plan := &model.SubscriptionPlan{PriceAmount: 12.345678, Currency: "USD"}

	snapshot, err := subscriptionPlanPaymentSnapshot(plan)

	require.NoError(t, err)
	assert.Equal(t, int64(12_345_678), snapshot.AmountMicros)
}
