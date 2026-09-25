package service

// P1-4 渠道多因子凸组合路由。
//
// 设计（对标 OmniRoute 因子裁剪）：候选渠道按一组 0-1 归一化因子打凸组合分，
// 得分最高者当选。与固定 weighted 策略互补：新增 combo.Strategy = "factor"。
//
// 因子（0-1，越大越好）：
//   - health    : ChannelHealthSnapshot.Score / 100
//   - latency   : 线性映射 p50（1.5s 满分 → 10s 零分），无样本 → 0.5 中性
//   - quality   : 1 - 错误率（滑动窗失败率），无样本 → 0.5
//   - consistency / cacheAffinity / cost : 暂取 1.0 中性项（保留扩展点，
//     待每请求成本/缓存命中数据源接入后替换；不伪造数据）。
//
// 权重表可配置（env）：ROUTE_W_HEALTH/LATENCY/QUALITY/COST/CACHE/CONSIST，
// 默认凸组合 health=0.45 latency=0.2 quality=0.25 cost=0.05 cache=0.05 consist=0。
// 归一化保证权重和 = 1（凸组合）；非法/全零输入 fail-open 回退固定权重。
//
// 决策可解释：ScoreChannelFactors 返回各因子与总分，路由日志记录
// 「combo factor -> channel#N score=X health=Y latency=Z quality=W」。

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/setting/operation_setting"
)

// FactorWeights 凸组合权重（和恒为 1）。
type FactorWeights struct {
	Health        float64
	Latency       float64
	Quality       float64
	Cost          float64
	CacheAffinity float64
	Consistency   float64
}

// defaultFactorWeights 默认梯度：健康优先，其次质量，再延迟。
func defaultFactorWeights() FactorWeights {
	return FactorWeights{Health: 0.45, Latency: 0.20, Quality: 0.25, Cost: 0.05, CacheAffinity: 0.05}
}

// routeFactorOverride 管理员显式覆盖权重（env 一次读取；0 表示用默认）。
func routeFactorOverride() FactorWeights {
	w := defaultFactorWeights()
	read := func(env string, dst *float64) {
		raw := strings.TrimSpace(os.Getenv(env))
		if raw == "" {
			return
		}
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v >= 0 {
			*dst = v
		}
	}
	read("ROUTE_W_HEALTH", &w.Health)
	read("ROUTE_W_LATENCY", &w.Latency)
	read("ROUTE_W_QUALITY", &w.Quality)
	read("ROUTE_W_COST", &w.Cost)
	read("ROUTE_W_CACHE", &w.CacheAffinity)
	read("ROUTE_W_CONSIST", &w.Consistency)
	return w
}

// ChannelFactorInput 候选渠道的因子原始输入（可选零值=未知）。
type ChannelFactorInput struct {
	HealthScore  float64 // 0-100（快照 Score）
	P50LatencyMs int64   // 成功样本 p50；0 = 无样本
	ErrorRate    float64 // 0-1 滑动窗失败率；-1 = 无样本
	HasSamples   bool    // 是否有真实样本
}

// ChannelFactorScores 归一化因子与凸组合总分（决策可解释）。
type ChannelFactorScores struct {
	Score       float64 `json:"score"`
	Health      float64 `json:"health"`
	Latency     float64 `json:"latency"`
	Quality     float64 `json:"quality"`
	Cost        float64 `json:"cost"`
	Cache       float64 `json:"cache"`
	Consistency float64 `json:"consistency"`
}

// latencyFactor p50 线性映射：<=latency_best 满分，>=latency_worst 零分。
// 边界与健康分公式同源（operation_setting.channel_health），避免双口径。
func latencyFactor(p50Ms int64) float64 {
	bestD, worstD := operation_setting.GetChannelHealthLatencyBounds()
	best := float64(bestD.Milliseconds())
	worst := float64(worstD.Milliseconds())
	// 与 computeHealthScore 守卫对齐：best>=worst 是非法/退化配置，
	// 返回中性 0.5 避免 0/0 -> NaN 把 factor 候选静默丢弃。
	if worst <= best {
		return 0.5
	}
	v := (worst - float64(p50Ms)) / (worst - best)
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return v
}

// ScoreChannelFactors 将原始输入归一化并按凸组合加权，返回可解释分数。
// 输入未知（无样本）时取中性 0.5，避免惩罚新渠道（fail-open 原则）。
func ScoreChannelFactors(in ChannelFactorInput) (float64, ChannelFactorScores) {
	w := routeFactorOverride()
	health := 0.5
	latency := 0.5
	quality := 0.5
	if in.HasSamples {
		health = clamp01(in.HealthScore / 100.0)
		if in.P50LatencyMs > 0 {
			latency = latencyFactor(in.P50LatencyMs)
		}
		if in.ErrorRate >= 0 {
			quality = clamp01(1 - in.ErrorRate)
		}
	}
	const cost = 1.0
	const cache = 1.0
	const consist = 1.0
	score := w.Health*health + w.Latency*latency + w.Quality*quality +
		w.Cost*cost + w.CacheAffinity*cache + w.Consistency*consist
	return score, ChannelFactorScores{
		Score: score, Health: health, Latency: latency, Quality: quality,
		Cost: cost, Cache: cache, Consistency: consist,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ExplainFactorDecision 输出决策日志一行（可读理由）。
func ExplainFactorDecision(comboName string, channelID int, s ChannelFactorScores) string {
	return fmt.Sprintf("combo %q factor -> channel #%d score=%.3f health=%.2f latency=%.2f quality=%.2f",
		comboName, channelID, s.Score, s.Health, s.Latency, s.Quality)
}

// factorSelectionDeadline 单次因子评估的时间上限，超时 fail-open（防止快照
// 查询阻塞路由热路径；记录告警）。保守 50ms（纯内存读取应远小于此）。
const factorSelectionDeadline = 50 * time.Millisecond
