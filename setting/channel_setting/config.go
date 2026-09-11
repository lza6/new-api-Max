package channel_setting

import "github.com/lza6/new-api-Max/setting/config"

// ChannelSetting 渠道治理设置（热更新，注册名 "channel"）。
type ChannelSetting struct {
	// CooldownV2 启用 B3-1 cooldown/retryable 解耦模型：错误路径统一经过
	// service.DecideCooldown 决策点，按错误类（B2-1 分类器）冷却渠道并在
	// 到期后自动恢复。关闭时保持旧行为（仅 AutomaticDisable 关键词/状态码
	// 触发硬禁用，无时长冷却）。
	CooldownV2 bool `json:"cooldown_v2"`
	// ProbeScheduleEnabled 启用 B4-2 每日定时验真探测（默认 off；
	// 手动「立即探测」不受此开关限制）。
	ProbeScheduleEnabled bool `json:"probe_schedule_enabled"`
}

var channelSetting = ChannelSetting{
	CooldownV2: false,
}

func init() {
	config.GlobalConfig.Register("channel", &channelSetting)
}

func GetChannelSetting() *ChannelSetting {
	return &channelSetting
}

// IsCooldownV2Enabled 报告 cooldown v2 模型是否开启（默认 off = 旧行为）。
func IsCooldownV2Enabled() bool {
	return channelSetting.CooldownV2
}
