package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewUserIdentityAcceptsOnlyInvoiceEligibleRequestsAndIsIdempotent(t *testing.T) {
	user := User{
		Username:             "identity-review-regression",
		Password:             "password",
		Identity:             UserIdentityPersonal,
		IdentityRequested:    UserIdentityUniversity,
		IdentityReviewStatus: "pending",
	}
	require.NoError(t, DB.Create(&user).Error)
	t.Cleanup(func() { DB.Unscoped().Where("id = ?", user.Id).Delete(&User{}) })

	require.NoError(t, ReviewUserIdentity(user.Id, true))
	var reviewed User
	require.NoError(t, DB.First(&reviewed, user.Id).Error)
	assert.Equal(t, UserIdentityUniversity, reviewed.Identity)
	assert.Equal(t, "approved", reviewed.IdentityReviewStatus)
	assert.Empty(t, reviewed.IdentityRequested)

	// A second administrator cannot overwrite the already completed decision.
	require.Error(t, ReviewUserIdentity(user.Id, false))
	require.NoError(t, DB.First(&reviewed, user.Id).Error)
	assert.Equal(t, UserIdentityUniversity, reviewed.Identity)
	assert.Equal(t, "approved", reviewed.IdentityReviewStatus)
}

func TestReviewUserIdentityRejectsNonInvoiceIdentityRequest(t *testing.T) {
	user := User{
		Username:             "student-review-regression",
		Password:             "password",
		Identity:             UserIdentityPersonal,
		IdentityRequested:    UserIdentityStudent,
		IdentityReviewStatus: "pending",
	}
	require.NoError(t, DB.Create(&user).Error)
	t.Cleanup(func() { DB.Unscoped().Where("id = ?", user.Id).Delete(&User{}) })

	require.Error(t, ReviewUserIdentity(user.Id, true))
	var unchanged User
	require.NoError(t, DB.First(&unchanged, user.Id).Error)
	assert.Equal(t, UserIdentityPersonal, unchanged.Identity)
	assert.Equal(t, UserIdentityStudent, unchanged.IdentityRequested)
	assert.Equal(t, "pending", unchanged.IdentityReviewStatus)
}
