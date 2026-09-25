package model

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupChannelRuntimeTest 建独立渠道数据并全量重建内存缓存。
func setupChannelRuntimeTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	original := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = original
		truncateTables(t)
	})
}

// TestChannelRuntimeSnapshotParsedOnce：预计算快照与 Channel 方法解析结果一致，
// 且快照命中时不重新解析（快照 map 非空即可证明已预计算）。
func TestChannelRuntimeSnapshotParsedOnce(t *testing.T) {
	setupChannelRuntimeTest(t)
	ch := &Channel{
		Name:   "snapshot-channel",
		Key:    "snap-key",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-4o,bench-model",
	}
	ch.SetSetting(dto.ChannelSettings{})
	ch.SetOtherSettings(dto.ChannelOtherSettings{})
	require.NoError(t, DB.Create(ch).Error)
	require.NoError(t, ch.AddAbilities(nil))

	InitChannelCache()
	snap := CacheGetChannelRuntimeSnapshot(ch.Id)
	require.NotNil(t, snap, "预计算快照必须存在")
	assert.Equal(t, ch.GetModelMapping(), snap.ModelMapping)
	assert.Equal(t, ch.GetStatusCodeMapping(), snap.StatusCodeMapping)
	assert.Equal(t, ch.GetAutoBan(), snap.AutoBan)
	assert.Equal(t, ch.GetBaseURL(), snap.BaseURL)
	assert.NotNil(t, snap.ParamOverride)
	assert.NotNil(t, snap.HeaderOverride)
	assert.Equal(t, dto.ChannelSettings{}, snap.Settings)
}

// TestCacheUpdateChannelStatusReenableRestoresIndex：冷却恢复启用后渠道必须
// 重新出现在 group2model2channels 索引（防“恢复后选不到”）。
func TestCacheUpdateChannelStatusReenableRestoresIndex(t *testing.T) {
	setupChannelRuntimeTest(t)
	ch := &Channel{
		Name:   "cooldown-recover",
		Key:    "cd-key",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "bench-model",
	}
	require.NoError(t, DB.Create(ch).Error)
	require.NoError(t, ch.AddAbilities(nil))
	InitChannelCache()

	// 禁用 → 索引移除（key 可残留空 slice，语义=无候选）
	CacheUpdateChannelStatus(ch.Id, common.ChannelStatusAutoDisabled)
	channelSyncLock.RLock()
	idsAfterDisable := group2model2channels["default"]["bench-model"]
	channelSyncLock.RUnlock()
	require.NotContains(t, idsAfterDisable, ch.Id, "禁用后索引必须移除该渠道")

	// 恢复启用 → 索引重建（回归：旧实现不回加）
	CacheUpdateChannelStatus(ch.Id, common.ChannelStatusEnabled)
	channelSyncLock.RLock()
	ids := group2model2channels["default"]["bench-model"]
	channelSyncLock.RUnlock()
	require.Contains(t, ids, ch.Id, "恢复启用后必须重新出现在索引")
	// 并保证非空（不是残留空 key）
	require.NotEmpty(t, ids)
}
