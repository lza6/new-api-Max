package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestChannelCompressionEnabledThreeState 渠道压缩三态语义：
// nil=默认开启、true=开启、false=显式关闭。
func TestChannelCompressionEnabledThreeState(t *testing.T) {
	ResetCompressionBreakerForTest()
	tr := true
	fa := false

	// nil（缺省）→ 默认开启。
	assert.True(t, channelCompressionEnabled(0, nil), "缺省应为开启")
	// 显式 true → 开启。
	assert.True(t, channelCompressionEnabled(0, &tr))
	// 显式 false → 关闭（优先于默认）。
	assert.False(t, channelCompressionEnabled(0, &fa), "显式关闭应生效")
	// 熔断中：显式 true 也被禁用（隔离不兼容渠道）；显式 false 仍关闭。
	TripChannelCompressionBreaker(5, "test")
	assert.False(t, channelCompressionEnabled(5, &fa))
	assert.False(t, channelCompressionEnabled(5, &tr), "熔断中即使显式 true 也禁用")
}

// TestChannelCompressionBreakerTrip 熔断：触发后本渠道禁用压缩，其他渠道不受影响。
func TestChannelCompressionBreakerTrip(t *testing.T) {
	ResetCompressionBreakerForTest()
	tr := true

	// 触发前：渠道 7 默认开启。
	assert.True(t, channelCompressionEnabled(7, &tr))
	// 触发熔断。
	TripChannelCompressionBreaker(7, "upstream 400")
	assert.True(t, IsChannelCompressionTrippedForTest(7), "渠道 7 应处于熔断期")
	assert.False(t, channelCompressionEnabled(7, &tr), "熔断中应禁用压缩")
	// 其他渠道不受影响。
	assert.True(t, channelCompressionEnabled(8, &tr), "其他渠道不受影响")

	// 重置后恢复。
	ResetCompressionBreakerForTest()
	assert.False(t, IsChannelCompressionTrippedForTest(7))
	assert.True(t, channelCompressionEnabled(7, &tr))
}

// TestStreamEmptyOnCompressedRequestTripsBreaker 回归：空壳流（零 data 事件）在
// **请求体确实被压缩过**时也必须触发压缩熔断。
//
// 真实故障：渠道 #59（hebox.net）对 gzip 请求体返回 HTTP 200 +
// application/json（非流式），scanner 读不到任何 data 事件。既有熔断只认
// 400/415/422，这类 200 形态永远不触发 → 该渠道持续 500 两天、145 次，
// 且上游侧无任何失败记录。这条用例锁死「200 + 空壳 + 用过压缩 → 熔断」。
func TestStreamEmptyOnCompressedRequestTripsBreaker(t *testing.T) {
	ResetCompressionBreakerForTest()
	assert.False(t, IsChannelCompressionTrippedForTest(59))

	// 未用过压缩（RequestOriginalBytes 为 0）不得触发：避免把与压缩无关的
	// 上游空流问题错误归因到压缩上。
	assert.False(t, HasUsedOutboundCompression(&RelayInfo{}))
	assert.False(t, HasUsedOutboundCompression(nil))

	compressed := &RelayInfo{}
	compressed.RequestOriginalBytes = 62070
	assert.True(t, HasUsedOutboundCompression(compressed),
		"RequestOriginalBytes>0 表示本次请求体确实走了压缩")

	TripChannelCompressionBreaker(59, "upstream returned an empty stream for a gzip-compressed request body")
	assert.True(t, IsChannelCompressionTrippedForTest(59), "渠道 59 应被隔离出压缩路径")
	tr := true
	assert.False(t, channelCompressionEnabled(59, &tr), "熔断中该渠道不再压缩")
	assert.True(t, channelCompressionEnabled(58, &tr), "其他渠道不受影响")
}

// TestCompressionUnsupportedUsesLongIsolation 回归：确证「上游不支持 gzip」时必须用
// **长隔离**（24h），而不是普通熔断的 10 分钟。
//
// 为什么这条值得单独锁：普通 TTL 是为「上游那一刻不高兴」设计的（400/415/422）。
// 但「明文流式、gzip 答非流式」是**实现缺陷**，不会自己好。生产渠道 #59 实测：
// 用 10 分钟 TTL 时该渠道每 10 分钟再撞一次墙，用户按概率持续吃到 500 —— 也就是
// 「熔断了但等于没熔断」。长隔离才真正实现用户要的「不支持就自动退回」。
func TestCompressionUnsupportedUsesLongIsolation(t *testing.T) {
	ResetCompressionBreakerForTest()

	// 普通熔断（疑似暂时性）：短 TTL。
	TripChannelCompressionBreaker(100, CompressionBreakerReasonUpstreamStatus)
	// 确证不支持：长 TTL。
	TripChannelCompressionUnsupported(59, CompressionBreakerReasonNonStreamDowngrade)

	assert.Less(t, CompressionBreakerTTLForTest(CompressionBreakerReasonUpstreamStatus),
		CompressionBreakerTTLForTest(CompressionBreakerReasonNonStreamDowngrade),
		"确证不支持的隔离时长必须显著长于普通熔断")
	assert.GreaterOrEqual(t, CompressionBreakerTTLForTest(CompressionBreakerReasonNonStreamDowngrade),
		12*time.Hour, "确证不支持至少隔离半天，否则会反复踩同一个坑")

	// 记录原因，供排查直接看出「为什么不压缩了」。
	assert.Equal(t, CompressionBreakerReasonNonStreamDowngrade,
		ChannelCompressionBreakerReason(59))
	assert.Equal(t, CompressionBreakerReasonUpstreamStatus,
		ChannelCompressionBreakerReason(100))

	// 两种熔断都真的禁用了压缩。
	tr := true
	assert.False(t, channelCompressionEnabled(59, &tr))
	assert.False(t, channelCompressionEnabled(100, &tr))
	// 未熔断的渠道不受影响（不能把「个别不支持」放大成全站禁用）。
	assert.True(t, channelCompressionEnabled(61, &tr))

	// 过期后原因也要清掉，避免管理端读到陈旧原因。
	ResetCompressionBreakerForTest()
	assert.Empty(t, ChannelCompressionBreakerReason(59))
}

// TestCompressionBreakerReasonClearedOnLazyExpiry 惰性过期时原因必须一并清理，
// 否则熔断已解除但管理端仍显示「因为不支持 gzip 而禁用」，属于误导性状态。
func TestCompressionBreakerReasonClearedOnLazyExpiry(t *testing.T) {
	ResetCompressionBreakerForTest()

	// 直接塞一个已过期的时间点，模拟 TTL 到期。
	compressionBreaker.Lock()
	compressionBreaker.until[77] = time.Now().Add(-time.Second)
	compressionBreaker.reason[77] = CompressionBreakerReasonNonStreamDowngrade
	compressionBreaker.Unlock()

	tr := true
	assert.True(t, channelCompressionEnabled(77, &tr), "已过期应恢复压缩")
	assert.Empty(t, ChannelCompressionBreakerReason(77), "过期后原因必须一并清理")
}

// TestCompressionUnsupportedDoesNotAffectOtherChannels 长隔离只作用于命中的渠道，
// 绝不放大成全局禁用——否则一个坏上游会拖慢全站（压缩是首字延迟的关键优化）。
func TestCompressionUnsupportedDoesNotAffectOtherChannels(t *testing.T) {
	ResetCompressionBreakerForTest()
	TripChannelCompressionUnsupported(59, CompressionBreakerReasonNonStreamDowngrade)

	tr := true
	for _, other := range []int{49, 50, 62, 63} {
		assert.True(t, channelCompressionEnabled(other, &tr),
			"渠道 %d 未熔断，必须继续享受压缩", other)
	}
}
