package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type checkinRequest struct {
	Nonce string `json:"nonce"`
}

// GetCheckinStatus 获取用户签到状态和历史记录
func GetCheckinStatus(c *gin.Context) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "签到功能未启用")
		return
	}
	userId := c.GetInt("id")
	// 获取月份参数，默认为当前月份
	month := c.DefaultQuery("month", time.Now().Format("2006-01"))

	stats, err := model.GetUserCheckinStats(userId, month)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	nonce := common.GetRandomString(24)
	session := sessions.Default(c)
	session.Set("checkin_nonce", nonce)
	if err := session.Save(); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":       setting.Enabled,
			"min_quota":     setting.MinQuota,
			"max_quota":     setting.MaxQuota,
			"stats":         stats,
			"checkin_nonce": nonce,
		},
	})
}

// DoCheckin 执行用户签到
func DoCheckin(c *gin.Context) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "签到功能未启用")
		return
	}

	userId := c.GetInt("id")
	var req checkinRequest
	if c.Request.Body != nil {
		_ = common.DecodeJson(c.Request.Body, &req)
	}
	session := sessions.Default(c)
	nonce, _ := session.Get("checkin_nonce").(string)
	session.Delete("checkin_nonce")
	_ = session.Save()
	if nonce == "" || req.Nonce == "" || nonce != req.Nonce {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "签到校验失败，请刷新页面后重试",
		})
		return
	}

	checkin, err := model.UserCheckin(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	model.RecordSystemLogWithRequestContext(
		c,
		userId,
		fmt.Sprintf("用户签到，获得额度 %s", logger.LogQuota(checkin.QuotaAwarded)),
		"user.checkin",
		map[string]interface{}{
			"quota":        checkin.QuotaAwarded,
			"checkin_date": checkin.CheckinDate,
		},
	)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "签到成功",
		"data": gin.H{
			"quota_awarded": checkin.QuotaAwarded,
			"checkin_date":  checkin.CheckinDate},
	})
}
