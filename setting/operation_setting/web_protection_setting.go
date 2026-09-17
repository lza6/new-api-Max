package operation_setting

import "github.com/lza6/new-api-Max/setting/config"

// WebProtectionSetting Web 层防刷/限流/封禁配置（热更新，注册名 "web_protection"）。
// 仅作用于非 /v1 前缀的请求（Web 管理 API、静态资源、SPA 页面）；
// /v1 模型中继 API 完全不受影响，继续走既有 token 级限流。
type WebProtectionSetting struct {
	// Enabled 总开关（限流+日志+自动封禁）。
	Enabled bool `json:"enabled"`
	// LimitPerSecond 每 IP 每秒允许请求数（档位基准）。
	LimitPerSecond int `json:"limit_per_second"`
	// Burst 突发容忍（令牌桶容量），首屏多资源并行加载需要突发。
	Burst int `json:"burst"`
	// AutoBan 自动封禁开关。
	AutoBan bool `json:"auto_ban"`
	// AutoBanThresholdPerMinute 同一 60s 窗口内被 429 拒绝达到该次数即自动封禁。
	AutoBanThresholdPerMinute int `json:"auto_ban_threshold_per_minute"`
	// AutoBanMinutes 自动封禁时长（分钟），0 表示永久。
	AutoBanMinutes int64 `json:"auto_ban_minutes"`
	// LogEnabled Web 请求日志（按 IP 聚合）开关。
	LogEnabled bool `json:"log_enabled"`
	// WindowSeconds 聚合窗口（秒），默认 60。
	WindowSeconds int64 `json:"window_seconds"`
}

var webProtectionSetting = WebProtectionSetting{
	Enabled:                   false,
	LimitPerSecond:            10,
	Burst:                     60,
	AutoBan:                   true,
	AutoBanThresholdPerMinute: 20,
	AutoBanMinutes:            1440,
	LogEnabled:                true,
	WindowSeconds:             60,
}

func init() {
	config.GlobalConfig.Register("web_protection", &webProtectionSetting)
}

func GetWebProtectionSetting() *WebProtectionSetting {
	return &webProtectionSetting
}

// IsWebProtectionEnabled Web 防刷总开关。
func IsWebProtectionEnabled() bool {
	return webProtectionSetting.Enabled
}

// GetWebProtectionLimit 返回（每秒允许数，突发容量，聚合窗口秒数），非法值回退默认。
func GetWebProtectionLimit() (int, int, int64) {
	perSec := webProtectionSetting.LimitPerSecond
	if perSec <= 0 {
		perSec = 10
	}
	burst := webProtectionSetting.Burst
	if burst < perSec {
		burst = perSec
	}
	windowSec := webProtectionSetting.WindowSeconds
	if windowSec <= 0 {
		windowSec = 60
	}
	return perSec, burst, windowSec
}

// IsAutoBanEnabled 自动封禁开关。
func IsAutoBanEnabled() bool {
	return webProtectionSetting.AutoBan
}

// GetAutoBanThreshold 窗口内 429 触发自动封禁的阈值（<=0 回退 20）。
func GetAutoBanThreshold() int {
	if webProtectionSetting.AutoBanThresholdPerMinute > 0 {
		return webProtectionSetting.AutoBanThresholdPerMinute
	}
	return 20
}

// GetAutoBanMinutes 自动封禁时长（分钟，<=0 回退 1440）。
func GetAutoBanMinutes() int64 {
	if webProtectionSetting.AutoBanMinutes > 0 {
		return webProtectionSetting.AutoBanMinutes
	}
	return 1440
}

// IsWebLogEnabled Web 请求日志开关。
func IsWebLogEnabled() bool {
	return webProtectionSetting.LogEnabled
}
