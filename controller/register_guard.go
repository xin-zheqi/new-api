package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type registrationIPDecision struct {
	Policy model.RegistrationRewardPolicy
}

func validateEmailPolicy(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	if common.EmailQQNumericOnlyEnabled && !common.IsQQEmailNumericAddress(email) {
		return fmt.Errorf("管理员已限制 QQ 邮箱只能使用数字 QQ 号，不能使用 QQ 别名邮箱")
	}
	return nil
}

func reserveRegistrationIP(c *gin.Context) (registrationIPDecision, func(), error) {
	if c == nil || !common.RegisterIPLimitEnabled || !common.RedisEnabled || common.RDB == nil {
		return registrationIPDecision{}, func() {}, nil
	}
	limit := common.RegisterIPLimitDailyCount
	if limit < 1 {
		limit = 1
	}
	ip := strings.TrimSpace(common.GetClientIP(c))
	if ip == "" {
		return registrationIPDecision{}, func() {}, nil
	}
	key := fmt.Sprintf("register_ip:%s:%s", time.Now().Format("20060102"), ip)
	ctx := context.Background()
	count, err := common.RDB.Incr(ctx, key).Result()
	if err != nil {
		common.SysLog("failed to increment register ip limit: " + err.Error())
		return registrationIPDecision{}, func() {}, nil
	}
	if count == 1 {
		common.RDB.Expire(ctx, key, 25*time.Hour)
	}
	rollback := func() {
		common.RDB.Decr(ctx, key)
	}
	if count <= int64(limit) {
		return registrationIPDecision{}, rollback, nil
	}
	if common.RegisterIPLimitBlockRegistration {
		return registrationIPDecision{}, rollback, fmt.Errorf("当前 IP 今日注册账号数量已超过限制，请稍后再试")
	}
	return registrationIPDecision{
		Policy: model.RegistrationRewardPolicy{
			SkipInitialQuota: common.RegisterIPLimitDisableInitialQuota,
			SkipInviteReward: common.RegisterIPLimitDisableInviteReward,
		},
	}, rollback, nil
}
