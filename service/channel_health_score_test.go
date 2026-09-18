package service

import (
	"testing"
	"time"

	"github.com/lza6/new-api-Max/setting/channel_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetChannelHealthForTest 清空健康分表，保证测试隔离。
func resetChannelHealthForTest(t *testing.T) {
	t.Helper()
	channelHealthMu.Lock()
	defer channelHealthMu.Unlock()
	channelHealthTable = map[int]*channelHealthRing{}
	t.Cleanup(func() {
		channelHealthMu.Lock()
		defer channelHealthMu.Unlock()
		channelHealthTable = map[int]*channelHealthRing{}
	})
}

// TestChannelHealthSnapshotRealData B3-3/B1-2：打真实样本后快照返回真实
// 成功率/P50/P95/评分（验证健康分 API 数据源，非 mock）。
func TestChannelHealthSnapshotRealData(t *testing.T) {
	resetChannelHealthForTest(t)
	const cid = 5001
	// 8 次成功（延迟 100-800ms）+ 2 次失败 → 成功率 0.8。
	for i := range 8 {
		RecordChannelOutcome(cid, true, time.Duration(100+i*100)*time.Millisecond, ErrClassOK)
	}
	for range 2 {
		RecordChannelOutcome(cid, false, 0, ErrClassServerError)
	}

	snap := GetChannelHealthSnapshot(cid)
	require.Positive(t, snap.SampleCount)
	assert.Equal(t, 10, snap.SampleCount)
	assert.InDelta(t, 0.8, snap.SuccessRate, 0.001)
	assert.GreaterOrEqual(t, snap.P50LatencyMs, int64(100))
	assert.GreaterOrEqual(t, snap.P95LatencyMs, snap.P50LatencyMs)
	assert.InDelta(t, computeHealthScore(0.8, snap.P95LatencyMs), snap.Score, 0.001)
}

// TestChannelHealthSnapshotCoolingReflectsCooldown B3-1/B1-2：冷却联动后
// 快照反映 cooling_down 与最近冷却错误类（hover-why 数据源）。
func TestChannelHealthSnapshotCoolingReflectsCooldown(t *testing.T) {
	resetChannelHealthForTest(t)
	const cid = 5002
	prev := channel_setting.GetChannelSetting().CooldownV2
	channel_setting.GetChannelSetting().CooldownV2 = true
	t.Cleanup(func() { channel_setting.GetChannelSetting().CooldownV2 = prev })

	// 记录一次冷却事件（rate_limited 类）→ 快照冷却计数 +1。
	RecordChannelCooldownMatchWithClass(cid, ErrClassRateLimited)
	snap := GetChannelHealthSnapshot(cid)
	assert.GreaterOrEqual(t, snap.CoolCount, 1)
	assert.Equal(t, "rate_limited", snap.LastCoolClass)

	// 冷却中（DecideCooldown 写入冷却表）→ 快照 cooling_down=true。
	cool, until := DecideCooldown(cid, ErrClassRateLimited, 0)
	require.True(t, cool)
	snap2 := GetChannelHealthSnapshot(cid)
	assert.True(t, snap2.CoolingDown)
	assert.Equal(t, until.Unix(), snap2.CoolUntil)
}
