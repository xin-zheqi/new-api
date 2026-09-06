package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetApiRouterRegistersLotteryRoutesWithoutConflict(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	server := gin.New()

	require.NotPanics(t, func() {
		SetApiRouter(server)
	})
}

func TestSetApiRouterDoesNotRegisterSubscriptionBalancePayment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := gin.New()
	require.NotPanics(t, func() { SetApiRouter(server) })

	routes := make(map[string]struct{}, len(server.Routes()))
	for _, route := range server.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	require.NotContains(t, routes, http.MethodPost+" /api/subscription/balance/pay")
}
