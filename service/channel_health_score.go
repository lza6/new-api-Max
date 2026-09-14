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

// B3-3 渠道健康分聚合：进程内滑动窗口统计每渠道近 1h 的成功率、P50/P95
// 延迟与冷却次数，输出 0-100 健康分（公式对齐 NewAPI-Gateway model_route
// 已验证实现：成功率 70% + 延迟线性分 30%，1.5s~10s 映射）。
// 数据源：RecordChannelOutcome（成功/失败统一入口）+ B3-1 冷却表。
// 消费方：管理端 API（B4-3 前端徽章）+ Batch-4 探测结果合并。

package service

import (
	"sort"
	"sync"
	"time"
)

// channelHealthRing 每渠道滑动窗口固定 256 条，防多渠道内存膨胀。
const (
	channelHealthRingSize  = 256
	channelHealthWindowTTL = time.Hour
	// 延迟分线性映射区间：1.5s 计满分，10s 计零分。
	healthLatencyBest  = 1500 * time.Millisecond
	healthLatencyWorst = 10 * time.Second
)

type channelHealthSample struct {
	at        time.Time
	success   bool
	latencyMs int64
	class     RelayErrorClass // 失败时的错误类；成功时 ErrClassOK
}

type channelHealthRing struct {
	samples   []channelHealthSample
	idx       int
	coolCount int // 近 1h 冷却次数（累计计数 + 时间裁剪）
	lastCool  time.Time
	// lastCoolClass 最近一次冷却的错误类（B2-1），用于渠道状态徽章 hover 展示
	// "连续 N 次 <类>，冷却至 <时间>"（B5-2）。零值 ErrClassOK 表示未知。
	lastCoolClass RelayErrorClass
}

type channelHealthState struct {
	ring *channelHealthRing
}

var (
	channelHealthMu    sync.RWMutex
	channelHealthTable = map[int]*channelHealthRing{}
)

func getHealthRing(channelId int) *channelHealthRing {
	channelHealthMu.Lock()
	defer channelHealthMu.Unlock()
	r, ok := channelHealthTable[channelId]
	if !ok {
		r = &channelHealthRing{}
		channelHealthTable[channelId] = r
	}
	return r
}

// RecordChannelOutcome 渠道请求结果统一记录入口（成功/失败都走这里）。
// latency 为本次上游调用耗时；失败时 class 为 B2-1 错误类，成功传 ErrClassOK。
func RecordChannelOutcome(channelId int, success bool, latency time.Duration, class RelayErrorClass) {
	r := getHealthRing(channelId)
	channelHealthMu.Lock()
	defer channelHealthMu.Unlock()
	if r.samples == nil {
		r.samples = make([]channelHealthSample, channelHealthRingSize)
	}
	r.samples[r.idx] = channelHealthSample{
		at:        time.Now(),
		success:   success,
		latencyMs: latency.Milliseconds(),
		class:     class,
	}
	r.idx = (r.idx + 1) % channelHealthRingSize
}

// RecordChannelCooldownMatch 记录一次冷却事件（B3-1 调用），供健康分聚合。
// 最近一次冷却的错误类随事件记录，供 B5-2 渠道状态徽章 hover 展示原因。
// 未提供具体类（旧调用方）时保持已有类，避免用 unknown 覆盖已知原因。
func RecordChannelCooldownMatch(channelId int) {
	RecordChannelCooldownMatchWithClass(channelId, ErrClassUnknown)
}

// RecordChannelCooldownMatchWithClass 记录冷却事件并携带错误类（B5-2）。
// 调用方（controller.processChannelError）在 DecideCooldown 判定后传入分类结果。
func RecordChannelCooldownMatchWithClass(channelId int, class RelayErrorClass) {
	r := getHealthRing(channelId)
	channelHealthMu.Lock()
	r.coolCount++
	r.lastCool = time.Now()
	if class != ErrClassUnknown {
		r.lastCoolClass = class
	}
	channelHealthMu.Unlock()
}

// ChannelHealthSnapshot 渠道健康聚合结果。
type ChannelHealthSnapshot struct {
	Score        float64 `json:"score"`        // 0-100
	SuccessRate  float64 `json:"success_rate"` // 0-1
	P50LatencyMs int64   `json:"p50_latency_ms"`
	P95LatencyMs int64   `json:"p95_latency_ms"`
	CoolCount    int     `json:"cool_count"`
	SampleCount  int     `json:"sample_count"`
	CoolingDown  bool    `json:"cooling_down"`
	CoolUntil    int64   `json:"cool_until,omitempty"` // unix 秒，0 = 未冷却
	// LastCoolClass 最近一次冷却的错误类（B2-1），供前端 hover 展示原因。
	LastCoolClass string `json:"last_cool_class,omitempty"`
}

// GetChannelHealthSnapshot 计算渠道健康快照（近 1h 窗口）。
func GetChannelHealthSnapshot(channelId int) ChannelHealthSnapshot {
	channelHealthMu.RLock()
	defer channelHealthMu.RUnlock()
	snap := ChannelHealthSnapshot{}
	r, ok := channelHealthTable[channelId]
	if !ok || r.samples == nil {
		return snap
	}
	cutoff := time.Now().Add(-channelHealthWindowTTL)
	var latencies []int64
	successes, total := 0, 0
	for i := 0; i < channelHealthRingSize; i++ {
		s := r.samples[i]
		if s.at.IsZero() || s.at.Before(cutoff) {
			continue
		}
		total++
		if s.success {
			successes++
			latencies = append(latencies, s.latencyMs)
		}
	}
	snap.SampleCount = total
	snap.CoolCount = r.coolCount
	if total > 0 {
		snap.SuccessRate = float64(successes) / float64(total)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	if len(latencies) > 0 {
		snap.P50LatencyMs = percentileOf(latencies, 0.50)
		snap.P95LatencyMs = percentileOf(latencies, 0.95)
	}
	snap.Score = computeHealthScore(snap.SuccessRate, snap.P95LatencyMs)
	until := GetChannelCooldownUntil(channelId)
	snap.CoolingDown = !until.IsZero() && time.Now().Before(until)
	if snap.CoolingDown {
		snap.CoolUntil = until.Unix()
	}
	if r.lastCoolClass != ErrClassOK {
		snap.LastCoolClass = RelayErrorClassString(r.lastCoolClass)
	}
	return snap
}

// computeHealthScore 健康分 = 成功率 × (70 + 延迟分)。
// 延迟分基于成功请求的 P95（1.5s 满分 30、10s 零分）；无成功样本 → 0 分。
// 乘以成功率保证：全部失败时延迟分不虚高（公式对齐 NewAPI-Gateway 实现）。
func computeHealthScore(successRate float64, p95LatencyMs int64) float64 {
	latencyScore := 0.0
	p95 := float64(p95LatencyMs)
	bestMs := float64(healthLatencyBest / time.Millisecond)
	worstMs := float64(healthLatencyWorst / time.Millisecond)
	if worstMs > bestMs {
		switch {
		case p95 <= bestMs:
			latencyScore = 30
		case p95 >= worstMs:
			latencyScore = 0
		default:
			latencyScore = (worstMs - p95) / (worstMs - bestMs) * 30
		}
	}
	return successRate * (70 + latencyScore)
}

func percentileOf(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}
