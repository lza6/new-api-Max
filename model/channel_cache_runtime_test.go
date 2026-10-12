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

// TestInitChannelCacheHandlesGroupWithoutAbilityRows 回归：渠道声明的分组
// 在 abilities 表里还没有任何行时，InitChannelCache 不得 panic。
//
// 真实故障（本地 E2E 实测复现）：内层 map 只按 abilities 的组预置，随后按
// channel.Group 写入时命中 nil map → `assignment to entry in nil map`，
// 直接把 SyncChannelCache goroutine（main 每 60s 拉起）打死，进程反复崩溃。
// 「渠道已配分组、但该组还没有 ability 行」是新加分组的常见中间状态。
func TestInitChannelCacheHandlesGroupWithoutAbilityRows(t *testing.T) {
	setupChannelRuntimeTest(t)

	// 关键：只建渠道，**不**调用 AddAbilities —— abilities 表保持为空。
	ch := &Channel{
		Name:   "group-without-abilities",
		Key:    "no-ability-key",
		Status: common.ChannelStatusEnabled,
		Group:  "超低价备用渠道",
		Models: "deepseek-v4.1-flash",
	}
	ch.SetSetting(dto.ChannelSettings{})
	ch.SetOtherSettings(dto.ChannelOtherSettings{})
	require.NoError(t, DB.Create(ch).Error)

	var abilityCount int64
	require.NoError(t, DB.Model(&Ability{}).Count(&abilityCount).Error)
	require.Zero(t, abilityCount, "前置条件：abilities 必须为空")

	require.NotPanics(t, func() { InitChannelCache() },
		"渠道分组没有 ability 行时不得 panic（nil map 赋值）")

	// 索引必须真的建起来，而不是靠 recover 掩盖。
	channelSyncLock.RLock()
	ids := group2model2channels["超低价备用渠道"]["deepseek-v4.1-flash"]
	channelSyncLock.RUnlock()
	require.Contains(t, ids, ch.Id, "该渠道必须出现在 group2model2channels 索引中")
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
