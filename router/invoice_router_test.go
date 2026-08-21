package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetApiRouterRegistersInvoiceAndMallManagementRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	require.NotPanics(t, func() { SetApiRouter(engine) })

	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		http.MethodGet + " /api/user/invoice",
		http.MethodPost + " /api/user/invoice/apply",
		http.MethodGet + " /api/user/invoice/:id/download",
		http.MethodPut + " /api/option/invoice",
		http.MethodPut + " /api/option/mall",
		http.MethodGet + " /api/invoice/admin/applications",
		http.MethodPost + " /api/invoice/admin/applications/:id/pdf",
		http.MethodDelete + " /api/invoice/admin/applications/:id/pdf",
		http.MethodGet + " /api/invoice/admin/applications/:id/download",
		http.MethodPost + " /api/invoice/admin/applications/:id/complete",
		http.MethodPost + " /api/invoice/admin/applications/:id/reject",
	} {
		_, exists := routes[route]
		assert.True(t, exists, "missing route %s", route)
	}
}
