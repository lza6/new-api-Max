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
