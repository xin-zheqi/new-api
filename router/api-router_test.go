package router

import (
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
