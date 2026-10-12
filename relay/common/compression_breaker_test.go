package common

import (
	"testing"

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
