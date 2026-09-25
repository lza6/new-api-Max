package operation_setting

import (
	"time"

	"github.com/lza6/new-api-Max/setting/config"
)

// ChannelHealthSetting 健康分聚合/路由策略参数（热更新，注册名 "channel_health"）。
// 默认值 = 历史代码常量，未配置管理端时行为零变化。
type ChannelHealthSetting struct {
	// WindowSeconds 健康分滑动窗口（秒），默认 3600（1h）。
	WindowSeconds int `json:"window_seconds"`
	// RingSize 每渠道样本环容量，默认 256；上限 4096 防内存膨胀。
	RingSize int `json:"ring_size"`
	// SuccessWeight 成功率权重（0-100），默认 70；延迟权重 = 100 - SuccessWeight。
	SuccessWeight int `json:"success_weight"`
	// LatencyBestMs 延迟满分阈值（ms），默认 1500。
	LatencyBestMs int64 `json:"latency_best_ms"`
	// LatencyWorstMs 延迟零分阈值（ms），默认 10000。
	LatencyWorstMs int64 `json:"latency_worst_ms"`
	// MinScore 路由过滤最低健康分；0 = 仅冷却剔除（沿用现有语义）。
	MinScore int `json:"min_score"`
}

var channelHealthSetting = ChannelHealthSetting{
	WindowSeconds:  3600,
	RingSize:       256,
	SuccessWeight:  70,
	LatencyBestMs:  1500,
	LatencyWorstMs: 10000,
	MinScore:       0,
}

func init() {
	config.GlobalConfig.Register("channel_health", &channelHealthSetting)
}

// GetChannelHealthSetting 返回当前健康分策略设置（指针，调用方只读）。
func GetChannelHealthSetting() *ChannelHealthSetting {
	return &channelHealthSetting
}

// GetChannelHealthWindowTTL 返回窗口 TTL（<=0 回退 1h）。
func GetChannelHealthWindowTTL() time.Duration {
	sec := channelHealthSetting.WindowSeconds
	if sec <= 0 {
		sec = 3600
	}
	return time.Duration(sec) * time.Second
}

// GetChannelHealthRingSize 返回样本环容量（<=0 回退 256；>4096 钳制 4096）。
func GetChannelHealthRingSize() int {
	if channelHealthSetting.RingSize <= 0 {
		return 256
	}
	if channelHealthSetting.RingSize > 4096 {
		return 4096
	}
	return channelHealthSetting.RingSize
}

// GetChannelHealthSuccessWeight 返回成功率权重（0-100，非法值回退 70）。
func GetChannelHealthSuccessWeight() int {
	w := channelHealthSetting.SuccessWeight
	if w <= 0 || w > 100 {
		return 70
	}
	return w
}

// GetChannelHealthLatencyBounds 返回（满分阈值，零分阈值）；非法值独立回退默认。
func GetChannelHealthLatencyBounds() (best, worst time.Duration) {
	bestMs := channelHealthSetting.LatencyBestMs
	if bestMs <= 0 {
		bestMs = 1500
	}
	worstMs := channelHealthSetting.LatencyWorstMs
	if worstMs <= 0 || worstMs <= bestMs {
		worstMs = 10000
	}
	return time.Duration(bestMs) * time.Millisecond, time.Duration(worstMs) * time.Millisecond
}

// GetChannelHealthMinScore 返回路由过滤最低分（<0 回退 0；>100 钳制 100）。
func GetChannelHealthMinScore() int {
	if channelHealthSetting.MinScore < 0 {
		return 0
	}
	if channelHealthSetting.MinScore > 100 {
		return 100
	}
	return channelHealthSetting.MinScore
}
