package controller

import (
	"net/http"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetGroupStatus(c *gin.Context) {
	const bucketSeconds int64 = 3 * 60
	const bucketCount int64 = 60

	groupRatios := ratio_setting.GetGroupRatioCopy()
	enabledGroups := operation_setting.GetGroupStatusSetting().EnabledGroups
	groups := make([]string, 0, len(enabledGroups))
	for group := range groupRatios {
		if enabledGroups[group] {
			groups = append(groups, group)
		}
	}
	sort.Strings(groups)

	now := time.Now().Unix()
	latestBucketStart := now / bucketSeconds * bucketSeconds
	startTs := latestBucketStart - (bucketCount-1)*bucketSeconds
	endTs := latestBucketStart + bucketSeconds

	timelines, err := model.GetGroupStatusTimelines(groups, startTs, endTs, bucketSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"start_ts":       startTs,
			"end_ts":         endTs,
			"bucket_seconds": bucketSeconds,
			"groups":         timelines,
		},
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			usableGroups[groupName] = map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, groupName),
				"desc":  desc,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
