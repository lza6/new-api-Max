package common

import (
	"fmt"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/logger"
)

// 渠道级压缩熔断。
//
// [功能] 压缩默认对所有渠道开启（ChannelSetting.RequestCompression 为 nil 时）。
// 但**部分上游不接受 gzip 请求体**（返回 400/415/500）。网关无法预知，故采用
// 「乐观启用 + 失败熔断」：当某渠道的压缩请求收到疑似「不接受压缩」的上游错误，
// 本渠道临时禁用压缩（默认 10 分钟），到期自动恢复重试——既不因个别不支持渠道
// 持续报错，也不必逐个手工配置。
//
// 判定"显式关闭"：渠道设置显式 false（管理员手动关）→ 永不压缩，与熔断无关。

const (
	// channelCompressionBreakerTTL 熔断持续时长：到期后自动恢复压缩并重试。
	channelCompressionBreakerTTL = 10 * time.Minute
)

var compressionBreaker = struct {
	sync.RWMutex
	until map[int]time.Time // channelId -> 熔断截止时刻
}{until: make(map[int]time.Time)}

// channelCompressionEnabled 判断某渠道是否应压缩：
//   - 显式 false → 否（管理员手动关闭，优先）。
//   - 显式 true 或 nil（默认）→ 是，除非本渠道处于熔断期。
func channelCompressionEnabled(channelId int, setting *bool) bool {
	if setting != nil && !*setting {
		return false // 显式关闭
	}
	if channelId <= 0 {
		return true
	}
	compressionBreaker.RLock()
	until, ok := compressionBreaker.until[channelId]
	compressionBreaker.RUnlock()
	if ok && time.Now().Before(until) {
		return false // 熔断中
	}
	if ok {
		// 已过期，惰性清理（避免 map 累积）。
		compressionBreaker.Lock()
		delete(compressionBreaker.until, channelId)
		compressionBreaker.Unlock()
	}
	return true
}

// TripChannelCompressionBreaker 当某渠道的压缩请求收到疑似"上游不接受 gzip"的
// 错误时调用：临时禁用本渠道压缩。安全幂等（重复调用只刷新截止时刻）。
//
// [修复防御] 4.2.6：把已证明不兼容的渠道隔离出压缩路径，避免持续 400；到期自动
// 恢复重试，保证上游修复后无需人工干预。
func TripChannelCompressionBreaker(channelId int, reason string) {
	if channelId <= 0 {
		return
	}
	until := time.Now().Add(channelCompressionBreakerTTL)
	compressionBreaker.Lock()
	compressionBreaker.until[channelId] = until
	compressionBreaker.Unlock()
	logger.LogWarn(nil, fmt.Sprintf("channel #%d compression breaker tripped until %s: %s",
		channelId, until.Format("15:04:05"), reason))
}

// IsChannelCompressionTrippedForTest 报告渠道是否处于压缩熔断期（仅测试用）。
func IsChannelCompressionTrippedForTest(channelId int) bool {
	compressionBreaker.RLock()
	defer compressionBreaker.RUnlock()
	until, ok := compressionBreaker.until[channelId]
	return ok && time.Now().Before(until)
}

// ResetCompressionBreakerForTest 清空熔断状态（仅测试用）。
func ResetCompressionBreakerForTest() {
	compressionBreaker.Lock()
	compressionBreaker.until = make(map[int]time.Time)
	compressionBreaker.Unlock()
}
