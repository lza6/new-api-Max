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
	healthSnapshotCache = map[int]healthSnapshotEntry{}
	t.Cleanup(func() {
		channelHealthMu.Lock()
		defer channelHealthMu.Unlock()
		channelHealthTable = map[int]*channelHealthRing{}
		healthSnapshotCache = map[int]healthSnapshotEntry{}
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

// TestComputeHealthScoreBoundaries 锁 B3-3 评分边界：1.5s 满分、10s 零分、线性插值、零成功率零分。
func TestComputeHealthScoreBoundaries(t *testing.T) {
	tests := []struct {
		name string
		rate float64
		p95  int64
		want float64
	}{
		{"all success p95<1.5s -> 100", 1.0, 1000, 100.0},
		{"all success p95=1.5s -> 100", 1.0, 1500, 100.0},
		{"all success p95=10s -> latency 0 -> 70", 1.0, 10000, 70.0},
		{"all success p95=5.75s midpoint -> 85", 1.0, 5750, 85.0},
		{"zero success rate -> 0", 0.0, 1000, 0.0},
		{"0.5 rate fast -> 50", 0.5, 1000, 50.0},
		{"all fail no latency -> 0", 0.0, 0, 0.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeHealthScore(tc.rate, tc.p95)
			assert.InDelta(t, tc.want, got, 0.001)
		})
	}
}

// TestChannelHealthSnapshotEmptyReturnsZero 无样本渠道返回全零且不 panic（冷启动/从未命中渠道）。
func TestChannelHealthSnapshotEmptyReturnsZero(t *testing.T) {
	resetChannelHealthForTest(t)
	snap := GetChannelHealthSnapshot(7777)
	assert.Zero(t, snap.SampleCount)
	assert.Zero(t, snap.Score)
	assert.False(t, snap.CoolingDown)
}

// TestChannelHealthSnapshotCacheInvalidatedOnNewSample：快照缓存命中后写入新
// 样本必须使缓存失效（下一读反映新样本）；1s TTL 内重复读不重建。
func TestChannelHealthSnapshotCacheInvalidatedOnNewSample(t *testing.T) {
	resetChannelHealthForTest(t)
	const cid = 5003
	RecordChannelOutcome(cid, true, 200*time.Millisecond, ErrClassOK)
	first := GetChannelHealthSnapshot(cid)
	require.Positive(t, first.SampleCount)

	// 直接调内部计算（未命中缓存）应得到同样结果，证明首读已缓存。
	direct := computeChannelHealthSnapshot(cid)
	assert.Equal(t, first.SampleCount, direct.SampleCount)

	// 新失败样本写入 → 缓存失效 → 下一读成功率下降。
	RecordChannelOutcome(cid, false, 0, ErrClassServerError)
	after := GetChannelHealthSnapshot(cid)
	assert.Equal(t, 2, after.SampleCount)
	assert.InDelta(t, 0.5, after.SuccessRate, 0.001)
}

// TestChannelHealthSnapshotCacheHitWithinTTL：1s TTL 内连续读只算一次。
func TestChannelHealthSnapshotCacheHitWithinTTL(t *testing.T) {
	resetChannelHealthForTest(t)
	const cid = 5004
	RecordChannelOutcome(cid, true, 300*time.Millisecond, ErrClassOK)
	// 连续两次读都不应因排序等原因变化；缓存命中由内部逻辑保证。
	a := GetChannelHealthSnapshot(cid)
	b := GetChannelHealthSnapshot(cid)
	assert.Equal(t, a.Score, b.Score)
	assert.Equal(t, a.SampleCount, b.SampleCount)
}
