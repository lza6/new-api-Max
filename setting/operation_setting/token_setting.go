package operation_setting

import "github.com/lza6/new-api-Max/setting/config"

// TokenSetting 令牌相关配置
type TokenSetting struct {
	MaxUserTokens int `json:"max_user_tokens"` // 每用户最大令牌数量
	// RequireVerificationToReadOwnKey 查看「自己」的密钥是否仍要求二次验证。
	// 默认 false：用户已通过 session 登录，后端 GetTokenByIds(id, userId) 已保证
	// 归属，二次验证属重复校验，徒增操作步骤（生产曾收到「复制密钥被要求验证」
	// 投诉）。设为 true 可恢复旧的 step-up 行为。读取「他人」密钥始终要求验证，
	// 不受此开关影响——该开关只放宽自己的密钥。
	RequireVerificationToReadOwnKey bool `json:"require_verification_to_read_own_key"`
	// RequireVerificationToTestChannelKey 测试/查看渠道密钥是否要求二次验证。
	// 默认 false：管理端 key 运维（查看/测试/批量测试）已由 AdminAuth +
	// RootAuth + ChannelSensitiveWrite 权限把关，重复 step-up 拖慢运维。
	// 设为 true 可恢复旧行为。
	RequireVerificationToReadChannelKey bool `json:"require_verification_to_read_channel_key"`
}

// 默认配置
var tokenSetting = TokenSetting{
	MaxUserTokens:                       1000, // 默认每用户最多 1000 个令牌
	RequireVerificationToReadOwnKey:     false,
	RequireVerificationToReadChannelKey: false,
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

// IsOwnKeyReadVerificationRequired 查看自己的密钥是否需要二次验证（默认否）。
func IsOwnKeyReadVerificationRequired() bool {
	return GetTokenSetting().RequireVerificationToReadOwnKey
}

// IsChannelKeyReadVerificationRequired 渠道密钥查看/测试是否需要二次验证（默认否）。
func IsChannelKeyReadVerificationRequired() bool {
	return GetTokenSetting().RequireVerificationToReadChannelKey
}
