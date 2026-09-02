package controller

import (
	"crypto/hmac"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const checkinNonceTTL = 10 * time.Minute

type checkinRequest struct {
	Nonce string `json:"nonce"`
}

func issueCheckinNonce(userID int, now time.Time) string {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	random := common.GetRandomString(24)
	payload := fmt.Sprintf("%d:%s:%s", userID, timestamp, random)
	signature := common.GenerateHMACWithKey([]byte(common.SessionSecret), "checkin:"+payload)
	return timestamp + "." + random + "." + signature
}

func verifyCheckinNonce(nonce string, userID int, now time.Time) bool {
	parts := strings.Split(nonce, ".")
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return false
	}
	issuedAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	age := now.Sub(time.Unix(issuedAt, 0))
	if age < -time.Minute || age > checkinNonceTTL {
		return false
	}
	payload := fmt.Sprintf("%d:%s:%s", userID, parts[0], parts[1])
	expected := common.GenerateHMACWithKey([]byte(common.SessionSecret), "checkin:"+payload)
	return hmac.Equal([]byte(parts[2]), []byte(expected))
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
			"message": common.PublicErrorMessage(c, err),
		})
		return
	}

	nonce := issueCheckinNonce(userId, time.Now())

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
	if !verifyCheckinNonce(req.Nonce, userId, time.Now()) {
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
			"message": common.PublicErrorMessage(c, err),
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
