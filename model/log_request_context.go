package model

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	logRequestUserAgentMaxSize = 512
)

func trimLogRequestUserAgent(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= logRequestUserAgentMaxSize {
		return value
	}
	return value[:logRequestUserAgentMaxSize] + "...[truncated]"
}

func BuildLogRequestAdminInfo(c *gin.Context) map[string]interface{} {
	if c == nil || c.Request == nil {
		return nil
	}
	adminInfo := map[string]interface{}{
		"request_ip": c.ClientIP(),
	}
	if userAgent := trimLogRequestUserAgent(c.Request.UserAgent()); userAgent != "" {
		adminInfo["user_agent"] = userAgent
	}
	return adminInfo
}

func MergeLogRequestAdminInfo(c *gin.Context, adminInfo map[string]interface{}) map[string]interface{} {
	requestInfo := BuildLogRequestAdminInfo(c)
	if len(requestInfo) == 0 {
		return adminInfo
	}
	if adminInfo == nil {
		adminInfo = map[string]interface{}{}
	}
	for key, value := range requestInfo {
		adminInfo[key] = value
	}
	return adminInfo
}

func addLogRequestAdminInfo(c *gin.Context, other map[string]interface{}) map[string]interface{} {
	if c == nil || c.Request == nil {
		return other
	}
	if other == nil {
		other = map[string]interface{}{}
	}
	adminInfo, _ := other["admin_info"].(map[string]interface{})
	other["admin_info"] = MergeLogRequestAdminInfo(c, adminInfo)
	return other
}
