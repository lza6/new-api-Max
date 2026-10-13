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
	// 适用于「疑似暂时性」失败（400/415/422）——上游可能只是那一刻不高兴。
	channelCompressionBreakerTTL = 10 * time.Minute

	// channelCompressionUnsupportedTTL 是「已确证上游不支持 gzip 请求体」的隔离时长。
	//
	// 为什么比普通熔断长得多（24h 而非 10min）：这类失败**不是暂时的**。生产实测
	// 渠道 #59（hebox.net）对带 `Content-Encoding: gzip` 的请求体恒返回非流式
	// JSON（明文则恒返回 SSE），是上游网关的实现缺陷，不会自己好。
	// 若沿用 10 分钟，该渠道每 10 分钟就会再撞一次墙——用户按概率持续吃到 500。
	// 24h 让「不支持」的渠道基本不再踩坑，同时仍保留每日一次的自愈机会
	// （上游修好后无需人工干预，最多等一天自动恢复）。
	channelCompressionUnsupportedTTL = 24 * time.Hour
)

// 熔断原因（稳定字符串，供日志与排查；不要随意改字面量）。
const (
	// CompressionBreakerReasonUpstreamStatus 上游对压缩请求返回 400/415/422。
	CompressionBreakerReasonUpstreamStatus = "upstream_status_rejected"
	// CompressionBreakerReasonNonStreamDowngrade 上游对压缩请求把成功响应降级成
	// 非流式（HTTP 200 但非 text/event-stream），即「接受了 gzip 但答错了形状」。
	// 这是**确证**的不兼容（明文同请求是 SSE），用长 TTL。
	CompressionBreakerReasonNonStreamDowngrade = "non_stream_downgrade"
)

var compressionBreaker = struct {
	sync.RWMutex
	until  map[int]time.Time // channelId -> 熔断截止时刻
	reason map[int]string    // channelId -> 最近一次熔断原因
}{until: make(map[int]time.Time), reason: make(map[int]string)}

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
		delete(compressionBreaker.reason, channelId)
		compressionBreaker.Unlock()
	}
	return true
}

// HasUsedOutboundCompression 报告本次请求的出站体是否**确实被网关压缩过**。
//
// 判据是 RequestOriginalBytes>0 —— 该字段仅由 MaybeCompressOutboundBody 在压缩
// 成功时写入；未达阈值、压缩无收益、存储失败等回退明文的情形都保持 0。
//
// 为什么需要这个判据：压缩熔断必须只对「与压缩有关的失败」生效，否则会把与该
// 渠道压缩能力无关的普通错误（如上游整体宕机导致的空流）也归因成「不接受 gzip」，
// 白白关掉一个其实支持压缩的渠道的优化。
func HasUsedOutboundCompression(info *RelayInfo) bool {
	return info != nil && info.RequestOriginalBytes > 0
}

// TripChannelCompressionBreaker 当某渠道的压缩请求收到疑似"上游不接受 gzip"的
// 错误时调用：临时禁用本渠道压缩。安全幂等（重复调用只刷新截止时刻）。
//
// [修复防御] 4.2.6：把已证明不兼容的渠道隔离出压缩路径，避免持续 400；到期自动
// 恢复重试，保证上游修复后无需人工干预。
func TripChannelCompressionBreaker(channelId int, reason string) {
	tripChannelCompressionBreaker(channelId, reason, channelCompressionBreakerTTL)
}

// TripChannelCompressionUnsupported 用于**已确证**「上游不支持 gzip 请求体」的场景：
// 上游对压缩请求把成功响应答成了非流式（明文同请求却是流式）。
//
// 与普通熔断的区别只在 TTL：这类不兼容不会自己好，用 24h 隔离，避免该渠道每
// 10 分钟就再撞一次墙、用户按概率持续吃到 500（生产渠道 #59 的实测形态）。
// 仍保留自动恢复：上游若修好，最多一天后自动重新启用压缩。
func TripChannelCompressionUnsupported(channelId int, reason string) {
	tripChannelCompressionBreaker(channelId, reason, channelCompressionUnsupportedTTL)
}

func tripChannelCompressionBreaker(channelId int, reason string, ttl time.Duration) {
	if channelId <= 0 {
		return
	}
	until := time.Now().Add(ttl)
	compressionBreaker.Lock()
	compressionBreaker.until[channelId] = until
	compressionBreaker.reason[channelId] = reason
	compressionBreaker.Unlock()
	logger.LogWarn(nil, fmt.Sprintf(
		"channel #%d compression breaker tripped for %s until %s: %s",
		channelId, ttl, until.Format("2006-01-02 15:04:05"), reason))
}

// ChannelCompressionBreakerReason 返回渠道最近一次被熔断的原因（无则空串）。
// 供管理端/排查读取：能直接看出「为什么不压缩了」。
func ChannelCompressionBreakerReason(channelId int) string {
	compressionBreaker.RLock()
	defer compressionBreaker.RUnlock()
	return compressionBreaker.reason[channelId]
}

// IsChannelCompressionTrippedForTest 报告渠道是否处于压缩熔断期（仅测试用）。
func IsChannelCompressionTrippedForTest(channelId int) bool {
	compressionBreaker.RLock()
	defer compressionBreaker.RUnlock()
	until, ok := compressionBreaker.until[channelId]
	return ok && time.Now().Before(until)
}

// CompressionBreakerTTLForTest 暴露两种 TTL，供用例断言「确证不支持用长隔离」。
func CompressionBreakerTTLForTest(kind string) time.Duration {
	if kind == CompressionBreakerReasonNonStreamDowngrade {
		return channelCompressionUnsupportedTTL
	}
	return channelCompressionBreakerTTL
}

// ResetCompressionBreakerForTest 清空熔断状态（仅测试用）。
func ResetCompressionBreakerForTest() {
	compressionBreaker.Lock()
	compressionBreaker.until = make(map[int]time.Time)
	compressionBreaker.reason = make(map[int]string)
	compressionBreaker.Unlock()
}
