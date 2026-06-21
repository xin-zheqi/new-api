package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type GroupStatusSetting struct {
	EnabledGroups map[string]bool `json:"enabled_groups"`
}

var groupStatusSetting = GroupStatusSetting{
	EnabledGroups: map[string]bool{},
}

func init() {
	config.GlobalConfig.Register("group_status_setting", &groupStatusSetting)
}

func GetGroupStatusSetting() *GroupStatusSetting {
	return &groupStatusSetting
}

func IsGroupStatusEnabled(group string) bool {
	if group == "" {
		return false
	}
	return groupStatusSetting.EnabledGroups[group]
}
