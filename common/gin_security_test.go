package common

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiErrorRedactsDatabaseDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalTranslate := TranslateMessage
	TranslateMessage = func(_ *gin.Context, key string, _ ...map[string]any) string {
		return key
	}
	t.Cleanup(func() { TranslateMessage = originalTranslate })

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/users", nil)
	ApiError(context, errors.New(`SQLSTATE 23505: duplicate key violates unique constraint "users_email_key", value private@example.com`))

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "common.database_error", response.Message)
	assert.NotContains(t, recorder.Body.String(), "private@example.com")
	assert.NotContains(t, recorder.Body.String(), "users_email_key")
}

func TestApiErrorPreservesPublicValidationMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/invalid", nil)
	ApiError(context, errors.New("invalid channel id"))

	var response struct {
		Message string `json:"message"`
	}
	require.NoError(t, Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "invalid channel id", response.Message)
}

func TestPublicErrorMessageRedactsDirectResponseDatabaseDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalTranslate := TranslateMessage
	TranslateMessage = func(_ *gin.Context, key string, _ ...map[string]any) string {
		return key
	}
	t.Cleanup(func() { TranslateMessage = originalTranslate })

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/option", nil)
	err := errors.New(`SQLSTATE 23505: rejected value private@example.com`)

	message := PublicErrorMessage(context, err)

	assert.Equal(t, "common.database_error", message)
	assert.NotContains(t, message, "private@example.com")
}
