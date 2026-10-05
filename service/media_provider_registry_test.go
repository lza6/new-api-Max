package service

import (
	"testing"

	"github.com/lza6/new-api-Max/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §4.6.1 媒体能力注册表：查询/门控/快照。
func TestMediaProviderRegistry(t *testing.T) {
	// 初始登记：kling 支持文生视频 + 图生视频。
	require.True(t, SupportsMediaCapability(constant.ChannelTypeKling, MediaCapabilityTextToVideo))
	require.True(t, SupportsMediaCapability(constant.ChannelTypeKling, MediaCapabilityImageToVideo))
	// midjourney 只登记图像生成，不支持文生视频。
	assert.False(t, SupportsMediaCapability(constant.ChannelTypeMidjourney, MediaCapabilityTextToVideo))
	assert.True(t, SupportsMediaCapability(constant.ChannelTypeMidjourney, MediaCapabilityImageGen))

	vids := MediaProvidersFor(MediaCapabilityTextToVideo)
	assert.NotEmpty(t, vids)
	// 稳定排序。
	for i := 1; i < len(vids); i++ {
		assert.LessOrEqual(t, vids[i-1].Name, vids[i].Name)
	}
	names := map[string]bool{}
	for _, p := range vids {
		names[p.Name] = true
	}
	assert.True(t, names["kling"] && names["sora"])
}

// 注册幂等（同名覆盖）+ 未知能力返回空。
func TestMediaProviderRegistryRegisterAndUnknown(t *testing.T) {
	resetMediaProviderRegistryForTest()
	t.Cleanup(func() { resetMediaProviderRegistryForTest() })

	RegisterMediaProvider(MediaProvider{Name: "x", ChannelType: 999, Capabilities: []MediaCapability{MediaCapabilityTTS}})
	RegisterMediaProvider(MediaProvider{Name: "x", ChannelType: 999, Capabilities: []MediaCapability{MediaCapabilityTTS}}) // 覆盖
	assert.Len(t, AllMediaProviders(), 1, "same-name registration must be idempotent")
	assert.True(t, SupportsMediaCapability(999, MediaCapabilityTTS))
	assert.Empty(t, MediaProvidersFor(MediaCapabilityASR))
}
