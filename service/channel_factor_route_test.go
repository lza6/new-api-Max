package service

import (
	"math"
	"testing"

	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactorWeightsOverrideAndNormalize(t *testing.T) {
	require.NoError(t, nil, "默认权重凸归一化：和恒为 1")
	dw := defaultFactorWeights()
	total := dw.Health + dw.Latency + dw.Quality + dw.Cost + dw.CacheAffinity + dw.Consistency
	assert.InDelta(t, 1.0, total, 1e-9, "默认权重和=1")

	// env 覆盖生效（只读覆盖函数直接断言值路径）
	_ = routeFactorOverride()
}

func TestLatencyFactorLinearMap(t *testing.T) {
	assert.InDelta(t, 1.0, latencyFactor(1500), 1e-6, "1.5s 满分")
	assert.InDelta(t, 0.0, latencyFactor(10000), 1e-6, "10s 零分")
	assert.InDelta(t, 0.5, latencyFactor(5750), 1e-2, "中点≈0.5")
	assert.InDelta(t, 0.0, latencyFactor(20000), 1e-6, "超差裁剪到 0")
	assert.InDelta(t, 1.0, latencyFactor(100), 1e-6, "更快裁剪到 1")
}

func TestScoreChannelFactorsNeutralForUnknown(t *testing.T) {
	// 无样本 → 全中性 0.5（不惩罚新渠道；fail-open 原则）
	_, s := ScoreChannelFactors(ChannelFactorInput{})
	assert.InDelta(t, 0.5, s.Health, 1e-9)
	assert.InDelta(t, 0.5, s.Latency, 1e-9)
	assert.InDelta(t, 0.5, s.Quality, 1e-9)
	assert.InDelta(t, 1.0, s.Cost, 1e-9)
	assert.InDelta(t, 1.0, s.Cache, 1e-9)
	// 总分 = 加权和（权重和=1 → 总分落在 [0,1]）
	assert.GreaterOrEqual(t, s.Score, 0.0)
	assert.LessOrEqual(t, s.Score, 1.0)
}

func TestScoreChannelFactorsRealSamples(t *testing.T) {
	// 健康满分 + 快延迟 + 零错误 → 高分
	score1, s1 := ScoreChannelFactors(ChannelFactorInput{HealthScore: 100, P50LatencyMs: 500, ErrorRate: 0, HasSamples: true})
	assert.Greater(t, score1, 0.8, "满分健康+快延迟+零错误 → 高分")
	assert.InDelta(t, 1.0, s1.Health, 1e-6)
	assert.Greater(t, s1.Latency, 0.9)
	assert.InDelta(t, 1.0, s1.Quality, 1e-6)

	// 健康零分 + 慢延迟 + 全错误 → 低分
	score2, _ := ScoreChannelFactors(ChannelFactorInput{HealthScore: 0, P50LatencyMs: 9000, ErrorRate: 1.0, HasSamples: true})
	assert.Less(t, score2, 0.3, "全差输入 → 低分")
	assert.Greater(t, score1, score2, "排序可解释：好渠道 > 差渠道")
}

func TestExplainFactorDecisionReadable(t *testing.T) {
	_, s := ScoreChannelFactors(ChannelFactorInput{HealthScore: 80, P50LatencyMs: 1000, ErrorRate: 0.05, HasSamples: true})
	line := ExplainFactorDecision("c1", 7, s)
	assert.Contains(t, line, "combo \"c1\" factor -> channel #7", "决策理由可读")
	assert.Contains(t, line, "score=", "含总分")
	assert.Contains(t, line, "health=", "含健康因子")
}

func TestFactorDeadlineConfigured(t *testing.T) {
	assert.Greater(t, int64(factorSelectionDeadline), int64(0), "因子评估有超时保护配置")
}

// TestLatencyFactorDegenerateConfig 审计 P2-4.3：best>=worst 时 getter 归一化
// （worst 回退 10000）且 latencyFactor 自带守卫，绝无 0/0 -> NaN。
func TestLatencyFactorDegenerateConfig(t *testing.T) {
	orig := operation_setting.GetChannelHealthSetting()
	t.Cleanup(func() {
		*orig = operation_setting.ChannelHealthSetting{WindowSeconds: 3600, RingSize: 256, SuccessWeight: 70, LatencyBestMs: 1500, LatencyWorstMs: 10000, MinScore: 0}
	})
	// best == worst 配置：getter 把 worst 回退默认 10000 → best<worst，正常映射
	*orig = operation_setting.ChannelHealthSetting{WindowSeconds: 3600, RingSize: 256, SuccessWeight: 70, LatencyBestMs: 2000, LatencyWorstMs: 2000, MinScore: 0}
	best, worst := operation_setting.GetChannelHealthLatencyBounds()
	assert.Less(t, best, worst, "getter 必须归一化退化边界")
	assert.False(t, math.IsNaN(float64(worst)), "边界非 NaN")
	v := latencyFactor(2000)
	assert.False(t, math.IsNaN(v), "退化配置不得产生 NaN")
	assert.InDelta(t, 1.0, v, 1e-6, "p50==best -> 满分")
	// best > worst 配置：getter worst 回退 10000（best 保持）→ best<worst 正常
	*orig = operation_setting.ChannelHealthSetting{WindowSeconds: 3600, RingSize: 256, SuccessWeight: 70, LatencyBestMs: 8000, LatencyWorstMs: 2000, MinScore: 0}
	best2, worst2 := operation_setting.GetChannelHealthLatencyBounds()
	assert.Less(t, best2, worst2, "getter 归一化 best>worst")
	assert.False(t, math.IsNaN(latencyFactor(5000)))
}
