/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// Grade 健康等级（0-100 分映射 A-F）。
type Grade string

const (
	GradeA Grade = "A"
	GradeB Grade = "B"
	GradeC Grade = "C"
	GradeD Grade = "D"
	GradeF Grade = "F"
)

// ProbeReport 一轮探测的完整报告（存渠道 probe_result 列）。
type ProbeReport struct {
	ProbedAt    int64        `json:"probed_at"`         // unix 秒
	Model       string       `json:"model"`             // 探测用模型
	Score       float64      `json:"score"`             // 0-100
	Grade       Grade        `json:"grade"`             // A-F
	TotalWeight float64      `json:"total_weight"`      // 参与计分的实际权重和
	Results     []CaseResult `json:"results"`           // 各用例明细
	DurationMs  int64        `json:"duration_ms"`       // 总耗时
	Skipped     string       `json:"skipped,omitempty"` // 整轮跳过原因（如不支持 OpenAI 格式）
}

// defaultCases 默认题库（权重合计 80：模型身份 25/参数 20/缓存 20/一致性 15）。
// 流完整性 10 与计费一致性 10 依赖流式/多计费路径，v1.1 先以 80 分制归一化。
func defaultCases() []ProbeCase {
	return []ProbeCase{
		ModelIDCase{},
		CapabilityCase{},
		CacheCase{},
		ConsistencyCase{},
	}
}

// ScoreGrade 分数 → 等级（相对满分归一化）。
func ScoreGrade(score, totalWeight float64) Grade {
	if totalWeight <= 0 {
		return GradeF
	}
	pct := score / totalWeight * 100
	switch {
	case pct >= 85:
		return GradeA
	case pct >= 70:
		return GradeB
	case pct >= 55:
		return GradeC
	case pct >= 35:
		return GradeD
	default:
		return GradeF
	}
}

// RunProbe 对目标渠道执行一轮完整探测。
// 调用方保证成本控制（每日 1 轮、单轮 ≤10 次调用——当前 4 用例 ≤7 次调用）。
func RunProbe(ctx context.Context, target *ProbeTarget) *ProbeReport {
	report := &ProbeReport{ProbedAt: time.Now().Unix(), Model: target.Model}
	start := time.Now()

	if target.Model == "" {
		report.Skipped = "channel has no probe model configured"
		return report
	}
	if target.TimeoutSecs <= 0 {
		target.TimeoutSecs = 60
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(target.TimeoutSecs)*time.Second)
	defer cancel()

	totalWeight := 0.0
	for _, c := range defaultCases() {
		result := c.Run(runCtx, target)
		report.Results = append(report.Results, result)
		report.Score += result.Score
		totalWeight += result.Weight
	}
	report.TotalWeight = totalWeight
	report.Grade = ScoreGrade(report.Score, totalWeight)
	report.DurationMs = time.Since(start).Milliseconds()

	common.SysLog(fmt.Sprintf("channel probe #%d (%s): score=%.1f grade=%s duration=%dms",
		target.ChannelID, target.Name, report.Score, report.Grade, report.DurationMs))
	return report
}
