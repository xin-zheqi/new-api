package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetClientIPUsesInternalRealIPHeader(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ctx.Request.Header.Set(InternalRealIPHeader, "203.0.113.10")

	assert.Equal(t, "203.0.113.10", GetClientIP(ctx))
}

func TestGetClientIPIgnoresInvalidInternalRealIPHeader(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ctx.Request.Header.Set(InternalRealIPHeader, "203.0.113.10, 198.51.100.20")

	require.NotEqual(t, "203.0.113.10, 198.51.100.20", GetClientIP(ctx))
}
