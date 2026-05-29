package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetModelFromJSONBodyRejectsOverlongGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"model":"gpt-test","group":"` + strings.Repeat("a", model.MaxTokenGroupConfigLength+1) + `"}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	req, err := getModelFromJSONBody(ctx)

	require.Nil(t, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field group is too long")
}

func TestGetModelRequestRejectsOverlongModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"model":"` + strings.Repeat("a", model.MaxRelayModelNameLength+1) + `"}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	req, shouldSelect, err := getModelRequest(ctx)

	require.Nil(t, req)
	require.False(t, shouldSelect)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field model is too long")
}
