package middleware

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelGroupLimitsPreserveGlobalAndUserOverrides(t *testing.T) {
	oldEnabled, oldMinutes := setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes
	oldTotal, oldSuccess := setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount
	oldGroups, oldRedis := setting.ModelRequestRateLimitGroup, common.RedisEnabled
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes = oldEnabled, oldMinutes
		setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = oldTotal, oldSuccess
		setting.ModelRequestRateLimitGroup, common.RedisEnabled = oldGroups, oldRedis
	})
	for index, tc := range []struct {
		name                string
		redis, userOverride bool
		total, success      int
	}{
		{name: "memory success", success: 1},
		{name: "memory total", total: 1, success: 100},
		{name: "redis success", redis: true, success: 1},
		{name: "redis total", redis: true, total: 1, success: 100},
		{name: "user override remains shared across groups", redis: true, userOverride: true, success: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes = true, 1
			setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = tc.total, tc.success
			setting.ModelRequestRateLimitGroup = map[string][2]int{"a": {tc.total, tc.success}, "b": {tc.total, tc.success}}
			common.RedisEnabled = false
			userID := 7240000 + index
			if tc.redis {
				useRateLimitMiniRedis(t)
				// Explicitly seed the existing user-limit cache so this fixture never
				// falls through to an unrelated or uninitialized application database.
				limit := model.UserRateLimit{Enabled: tc.userOverride, DurationMinutes: 1, SuccessCount: 1}
				require.NoError(t, common.RedisHSetObj(fmt.Sprintf("user_rate_limit:%d", userID), &limit, time.Minute))
			}
			router := gin.New()
			router.GET("/:group", func(c *gin.Context) {
				c.Set("id", userID)
				common.SetContextKey(c, constant.ContextKeyTokenGroup, c.Param("group"))
			}, ModelRequestRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
			wantFirst := http.StatusOK
			for _, group := range []string{"a", "b", "c", "d"} {
				if group == "d" || (tc.userOverride && group != "a") {
					wantFirst = http.StatusTooManyRequests
				}
				assert.Equal(t, wantFirst, performRateLimitRequest(router, "/"+group, "127.0.0.1:1234").Code, group)
				assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/"+group, "127.0.0.1:1234").Code, group)
			}
		})
	}
}
