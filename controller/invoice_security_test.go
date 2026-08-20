package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceApplicationAdminResponseExcludesSensitiveUserFields(t *testing.T) {
	application := model.InvoiceApplication{
		Id: 1,
		User: &model.User{
			Id: 2, Username: "invoice-user", DisplayName: "Invoice User",
			Email: "invoice@example.com", Identity: model.UserIdentityEnterprise,
			Password: "password-hash", GitHubId: "github-id", StripeCustomer: "stripe-customer",
		},
	}

	payload, err := common.Marshal(newInvoiceApplicationAdminResponse(application))
	require.NoError(t, err)
	responseBody := string(payload)
	assert.Contains(t, responseBody, "invoice-user")
	assert.NotContains(t, responseBody, "password")
	assert.NotContains(t, responseBody, "password-hash")
	assert.NotContains(t, responseBody, "github-id")
	assert.NotContains(t, responseBody, "stripe-customer")
}
