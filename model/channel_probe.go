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

package model

import (
	"time"

	"github.com/lza6/new-api-Max/common"
)

// SaveChannelProbeResult 持久化一轮探测报告（json 到 probe_result 列）。
// 历史对比：保留最近 5 次报告（新报告前插，旧的截断），「两次分歧=后端被换」
// 的告警逻辑由前端/运维对比 grade 完成。
func SaveChannelProbeResult(channelId int, reportJSON string) error {
	channel, err := GetChannelById(channelId, true)
	if err != nil {
		return err
	}
	history := "[]"
	if channel.ProbeResult != nil && *channel.ProbeResult != "" {
		history = *channel.ProbeResult
	}
	merged, err := mergeProbeHistory(history, reportJSON)
	if err != nil {
		// 历史解析失败时以新报告整体替换，探测功能不被坏数据卡死。
		merged = "[" + reportJSON + "]"
	}
	return DB.Model(channel).Update("probe_result", merged).Error
}

// mergeProbeHistory 把新报告前插进历史数组并截断到最近 5 次。
func mergeProbeHistory(existingJSON, reportJSON string) (string, error) {
	var existing []map[string]any
	if err := common.Unmarshal([]byte(existingJSON), &existing); err != nil {
		return "", err
	}
	var report map[string]any
	if err := common.Unmarshal([]byte(reportJSON), &report); err != nil {
		return "", err
	}
	merged := append([]map[string]any{report}, existing...)
	if len(merged) > 5 {
		merged = merged[:5]
	}
	b, err := common.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// GetChannelProbeHistory 读取渠道探测历史（nil = 从未探测）。
func GetChannelProbeHistory(channelId int) (*string, error) {
	channel, err := GetChannelById(channelId, false)
	if err != nil {
		return nil, err
	}
	return channel.ProbeResult, nil
}

// probeHistoryMaxAge 每日 1 轮的成本控制判断：距上次探测不足 24h 则跳过
// （手动触发不受限——调用方传 force=true）。
func probeHistoryMaxAge() time.Duration { return 24 * time.Hour }

// ChannelProbeDue 报告渠道是否到了每日探测窗口（从未探测 = due）。
func ChannelProbeDue(channelId int) bool {
	history, err := GetChannelProbeHistory(channelId)
	if err != nil || history == nil || *history == "" {
		return true
	}
	var reports []struct {
		ProbedAt int64 `json:"probed_at"`
	}
	if err := common.Unmarshal([]byte(*history), &reports); err != nil || len(reports) == 0 {
		return true
	}
	latest := time.Unix(reports[0].ProbedAt, 0)
	return time.Since(latest) >= probeHistoryMaxAge()
}
