package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryRateLimiterReservationReleasesFailedRequest(t *testing.T) {
	limiter := &InMemoryRateLimiter{}
	limiter.Init(0)

	reservation := limiter.Reserve("success", 1, 60)
	require.NotNil(t, reservation)
	assert.Nil(t, limiter.Reserve("success", 1, 60), "an in-flight request occupies the success slot")

	reservation.Complete(false)
	assert.NotNil(t, limiter.Reserve("success", 1, 60), "failed requests release the success slot")
}

func TestInMemoryRateLimiterReservationRecordsSuccessOnce(t *testing.T) {
	limiter := &InMemoryRateLimiter{}
	limiter.Init(0)

	reservation := limiter.Reserve("success", 1, 60)
	require.NotNil(t, reservation)
	reservation.Complete(true)
	reservation.Complete(false)
	assert.Nil(t, limiter.Reserve("success", 1, 60), "a successful request remains in the window")
}
