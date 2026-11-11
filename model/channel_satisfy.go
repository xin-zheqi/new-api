package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func IsChannelEnabledForGroupModel(group string, modelName string, channelID int) bool {
	if group == "" || modelName == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForGroupModelDB(group, modelName, channelID)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if group2model2channels == nil {
		return false
	}
	if channel, ok := channelsIDM[channelID]; !ok || !channel.IsAvailable() {
		return false
	}

	if isChannelIDInList(group2model2channels[group][modelName], channelID) {
		return true
	}
	normalized := ratio_setting.RoutingMatchModelName(modelName)
	if normalized != "" && normalized != modelName {
		return isChannelIDInList(group2model2channels[group][normalized], channelID)
	}
	return false
}

func IsChannelEnabledForAnyGroupModel(groups []string, modelName string, channelID int) bool {
	if len(groups) == 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForAnyGroupModelDB(groups, modelName, channelID)
	}
	for _, g := range groups {
		if IsChannelEnabledForGroupModel(g, modelName, channelID) {
			return true
		}
	}
	return false
}

func isChannelEnabledForGroupModelDB(group string, modelName string, channelID int) bool {
	return isChannelEnabledForAnyGroupModelDB([]string{group}, modelName, channelID)
}

func isChannelEnabledForAnyGroupModelDB(groups []string, modelName string, channelID int) bool {
	if len(groups) == 0 || modelName == "" || channelID <= 0 {
		return false
	}
	models := []string{modelName}
	normalized := ratio_setting.RoutingMatchModelName(modelName)
	if normalized != "" && normalized != modelName {
		models = append(models, normalized)
	}
	abilities := DB.Model(&Ability{}).Select("channel_id").
		Where(commonGroupCol+" IN ? and model IN ? and channel_id = ? and enabled = ?", groups, models, channelID, true)
	channel := &Channel{}
	err := DB.Select("id", "status", "setting").Where("id IN (?)", abilities).Limit(1).Find(channel).Error
	return err == nil && channel.Id > 0 && channel.IsAvailable()
}

func isChannelIDInList(list []int, channelID int) bool {
	for _, id := range list {
		if id == channelID {
			return true
		}
	}
	return false
}
