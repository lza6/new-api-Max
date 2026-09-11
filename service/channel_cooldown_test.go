package service

import (
	"testing"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/channel_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setCooldownV2ForTest(t *testing.T, enabled bool) {
	t.Helper()
	previous := channel_setting.GetChannelSetting().CooldownV2
	channel_setting.GetChannelSetting().CooldownV2 = enabled
	t.Cleanup(func() { channel_setting.GetChannelSetting().CooldownV2 = previous })
}

// TestDecideCooldownOff 开关 off 时永远不冷却（旧行为零变化）。
func TestDecideCooldownOff(t *testing.T) {
	setCooldownV2ForTest(t, false)
	cool, until := DecideCooldown(901, ErrClassAuth, 0)
	assert.False(t, cool)
	assert.True(t, until.IsZero())
	assert.False(t, IsChannelCoolingDown(901))
}

// TestDecideCooldownClasses 开关 on 时按 B2-1 错误类决策冷却。
func TestDecideCooldownClasses(t *testing.T) {
	setCooldownV2ForTest(t, true)
	t.Cleanup(func() {
		channelCooldownMu.Lock()
		channelCooldownTable = map[int]channelCooldownEntry{}
		channelCooldownMu.Unlock()
	})

	// 401：不可重试但必须冷却（aisix 教训），默认 5min。
	cool, until := DecideCooldown(911, ErrClassAuth, 0)
	require.True(t, cool)
	require.True(t, IsChannelCoolingDown(911))
	assert.WithinDuration(t, time.Now().Add(DefaultAuthCooldown), until, 2*time.Second)

	// 429 + Retry-After=2min：尊重上游（< 15min 上限）。
	cool, until = DecideCooldown(912, ErrClassRateLimited, 2*time.Minute)
	require.True(t, cool)
	assert.WithinDuration(t, time.Now().Add(2*time.Minute), until, 2*time.Second)

	// 429 + Retry-After=1h：钳制到 15min 上限。
	cool, until = DecideCooldown(913, ErrClassRateLimited, time.Hour)
	require.True(t, cool)
	assert.WithinDuration(t, time.Now().Add(DefaultCooldownCap), until, 2*time.Second)

	// 429 + 无 Retry-After：默认 60s。
	cool, until = DecideCooldown(914, ErrClassRateLimited, 0)
	require.True(t, cool)
	assert.WithinDuration(t, time.Now().Add(DefaultRateLimitedCooldown), until, 2*time.Second)

	// 5xx：冷却 1min（重试限次由重试链负责）。
	cool, _ = DecideCooldown(915, ErrClassServerError, 0)
	require.True(t, cool)
	assert.True(t, IsChannelCoolingDown(915))

	// 网络超时：冷却 30s。
	cool, _ = DecideCooldown(916, ErrClassTimeout, 0)
	require.True(t, cool)

	// 4xx 参数问题 / 能力不支持 / 成功：不冷却。
	for _, class := range []RelayErrorClass{ErrClassBadRequest, ErrClassCapability, ErrClassOK} {
		cool, until = DecideCooldown(917, class, 0)
		assert.False(t, cool, "class %v must not cool down", class)
		assert.True(t, until.IsZero(), "class %v must return zero until", class)
	}
}

// TestCooldownRecovery 冷却到期后 sweep 清理条目；AutoDisabled 渠道被恢复。
func TestCooldownRecovery(t *testing.T) {
	truncate(t)
	setCooldownV2ForTest(t, true)
	seedChannel(t, 921)

	// 渠道已被自动禁用后进入冷却。
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 921).
		Update("status", common.ChannelStatusAutoDisabled).Error)
	DecideCooldown(921, ErrClassServerError, 0)
	require.True(t, IsChannelCoolingDown(921))
	require.True(t, HasCoolingDownChannels())

	// 手动把冷却条目改为已过期 → sweep 清理并恢复渠道。
	channelCooldownMu.Lock()
	channelCooldownTable[921] = channelCooldownEntry{until: time.Now().Add(-time.Second)}
	channelCooldownMu.Unlock()

	CooldownRecoverySweep(nil)

	assert.False(t, IsChannelCoolingDown(921), "过期条目必须被清除")
	ch, err := model.GetChannelById(921, false)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, ch.Status, "AutoDisabled 渠道到期后自动恢复")
}

// TestChannelHealthScoreFormula B3-3 健康分公式：成功率 × (70 + 延迟分)。
func TestChannelHealthScoreFormula(t *testing.T) {
	// 成功率 1.0、延迟极快 → 满分 100。
	assert.InDelta(t, 100.0, computeHealthScore(1.0, 0), 0.001)
	// 成功率 0 → 0 分（延迟再快也不加分）。
	assert.InDelta(t, 0.0, computeHealthScore(0.0, 100), 0.001)
	// 成功率 1.0、P95=1.5s → 满分 100（延迟分满）。
	assert.InDelta(t, 100.0, computeHealthScore(1.0, 1500), 0.001)
	// 成功率 1.0、P95=10s → 延迟分 0 → 70。
	assert.InDelta(t, 70.0, computeHealthScore(1.0, 10000), 0.001)
	// 成功率 0.9、P95=5.75s（区间中点 → 延迟分 15）→ 0.9×85=76.5。
	assert.InDelta(t, 76.5, computeHealthScore(0.9, 5750), 0.01)
}

// TestChannelHealthRingAggregation 健康分 ring 聚合：成功率/延迟/冷却计数。
func TestChannelHealthRingAggregation(t *testing.T) {
	const channelId = 931
	// 重置 ring。
	channelHealthMu.Lock()
	delete(channelHealthTable, channelId)
	channelHealthMu.Unlock()
	t.Cleanup(func() {
		channelHealthMu.Lock()
		delete(channelHealthTable, channelId)
		channelHealthMu.Unlock()
	})

	RecordChannelOutcome(channelId, true, 100*time.Millisecond, ErrClassOK)
	RecordChannelOutcome(channelId, true, 200*time.Millisecond, ErrClassOK)
	RecordChannelOutcome(channelId, false, 3*time.Second, ErrClassServerError)
	RecordChannelCooldownMatch(channelId)

	snap := GetChannelHealthSnapshot(channelId)
	assert.Equal(t, 3, snap.SampleCount)
	assert.InDelta(t, 2.0/3.0, snap.SuccessRate, 0.001)
	// 延迟仅统计成功样本：[100, 200] → P50=P95=200。
	assert.Equal(t, int64(200), snap.P50LatencyMs)
	assert.Equal(t, int64(200), snap.P95LatencyMs)
	assert.Equal(t, 1, snap.CoolCount)
	assert.False(t, snap.CoolingDown)

	// 无数据渠道返回零值快照。
	empty := GetChannelHealthSnapshot(99999)
	assert.Equal(t, 0, empty.SampleCount)
	assert.InDelta(t, 0.0, empty.Score, 0.001)
}
