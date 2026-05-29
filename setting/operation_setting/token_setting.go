package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

const (
	DefaultMaxGroupCount = 50
	MinMaxGroupCount     = 1
	MaxMaxGroupCount     = 100
)

// TokenSetting 令牌相关配置
type TokenSetting struct {
	MaxUserTokens int `json:"max_user_tokens"` // 每用户最大令牌数量
	MaxGroupCount int `json:"max_group_count"` // 单个令牌最大分组数量
}

// 默认配置
var tokenSetting = TokenSetting{
	MaxUserTokens: 1000, // 默认每用户最多 1000 个令牌
	MaxGroupCount: DefaultMaxGroupCount,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("token_setting", &tokenSetting)
}

// GetTokenSetting 获取令牌配置
func GetTokenSetting() *TokenSetting {
	return &tokenSetting
}

// GetMaxUserTokens 获取每用户最大令牌数量
func GetMaxUserTokens() int {
	return GetTokenSetting().MaxUserTokens
}

func GetMaxGroupCount() int {
	if tokenSetting.MaxGroupCount < MinMaxGroupCount {
		return DefaultMaxGroupCount
	}
	if tokenSetting.MaxGroupCount > MaxMaxGroupCount {
		return MaxMaxGroupCount
	}
	return tokenSetting.MaxGroupCount
}

func SetMaxGroupCount(maxGroupCount int) {
	tokenSetting.MaxGroupCount = maxGroupCount
	NormalizeTokenSetting()
}

func NormalizeTokenSetting() {
	tokenSetting.MaxGroupCount = GetMaxGroupCount()
}
