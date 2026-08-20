package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetApiRouterRegistersStaticTicketAdminRoutesAlongsideTicketId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	require.NotPanics(t, func() {
		SetApiRouter(engine)
	})

	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		http.MethodGet + " /api/ticket/self",
		http.MethodPost + " /api/ticket",
		http.MethodGet + " /api/ticket/:id",
		http.MethodPost + " /api/ticket/:id/reply",
		http.MethodGet + " /api/ticket/:id/attachment/:attachment_id",
		http.MethodGet + " /api/ticket/admin",
		http.MethodGet + " /api/ticket/admin/:id",
		http.MethodPost + " /api/ticket/admin/:id/reply",
		http.MethodPost + " /api/ticket/admin/:id/close",
	} {
		_, exists := routes[route]
		assert.True(t, exists, "missing route %s", route)
	}
}
