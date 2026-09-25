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

// B3-1 cooldown/retryable 解耦 + 决策单点化（N4 aisix 证据）。
// 根因：冷却决策散落各 relay 路径 → 新增 relay 格式静默漏冷却（真实审计级
// bug 模式）。本文件是唯一冷却决策点：所有渠道错误路径必须经过
// DecideCooldown；冷却与可重试正交（可重试 ≠ 立刻重试；不可重试 ≠ 不冷却）。
//
// 行为开关：channel.cooldown_v2（默认 off = 旧行为，零变化）。
// on 时：按 B2-1 错误类决策冷却，到期由 cooldownRecoverySweep 自动恢复。
// 冷却表为进程内语义；多节点各自冷却（硬禁用/恢复仍走 DB 权威）。

package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/channel_setting"
)

// 冷却时长默认值（Retry-After 缺失/非法时的兜底；上限复用 DefaultCooldownCap）。
const (
	DefaultAuthCooldown        = 5 * time.Minute
	DefaultRateLimitedCooldown = time.Minute
	DefaultServerErrorCooldown = time.Minute
	DefaultTimeoutCooldown     = 30 * time.Second
)

type channelCooldownEntry struct {
	until     time.Time
	class     RelayErrorClass
	reason    string
	createdAt time.Time
}

var (
	channelCooldownMu    sync.RWMutex
	channelCooldownTable = map[int]channelCooldownEntry{}
)

// DecideCooldown 是唯一冷却决策点：所有 relay 格式的错误路径必须经过。
// 输入：渠道 id、B2-1 分类结果、上游 Retry-After（仅 429 有效）。
// 返回：是否冷却、冷却截止时间。开关 off 时永远返回 (false, zero)。
func DecideCooldown(channelId int, class RelayErrorClass, retryAfter time.Duration) (bool, time.Time) {
	if !channel_setting.IsCooldownV2Enabled() {
		return false, time.Time{}
	}
	var duration time.Duration
	switch class {
	case ErrClassAuth:
		// aisix 教训：401/403 不可重试但必须冷却，否则后续每个请求都重复撞。
		duration = DefaultAuthCooldown
	case ErrClassRateLimited:
		// 尊重上游 Retry-After（超长钳制到 DefaultCooldownCap）；缺失/非法 → 默认 60s。
		switch {
		case retryAfter <= 0:
			duration = DefaultRateLimitedCooldown
		case retryAfter > DefaultCooldownCap:
			duration = DefaultCooldownCap
		default:
			duration = retryAfter
		}
	case ErrClassServerError:
		duration = DefaultServerErrorCooldown
	case ErrClassTimeout:
		duration = DefaultTimeoutCooldown
	default:
		// BadRequest/Capability/OK/Unknown：我方问题或可确认终态，不冷却。
		return false, time.Time{}
	}
	until := time.Now().Add(duration)
	recordChannelCooldown(channelId, until, class)
	return true, until
}

func recordChannelCooldown(channelId int, until time.Time, class RelayErrorClass) {
	channelCooldownMu.Lock()
	channelCooldownTable[channelId] = channelCooldownEntry{
		until:     until,
		class:     class,
		createdAt: time.Now(),
	}
	channelCooldownMu.Unlock()
	// 冷却状态变化会反映到健康分快照（CoolingDown/CoolUntil），必须置空该渠道
	// 快照缓存，避免 1s TTL 内读到陈旧冷却状态。先释放 cooldown 锁再取 health 锁，
	// 保持与 GetChannelHealthSnapshot(仅 health) / GetChannelCooldownUntil(仅 cooldown)
	// 一致的锁序，避免嵌套死锁。
	channelHealthMu.Lock()
	delete(healthSnapshotCache, channelId)
	channelHealthMu.Unlock()
}

// GetChannelCooldownUntil 返回渠道当前冷却截止时间（零值 = 未冷却）。
func GetChannelCooldownUntil(channelId int) time.Time {
	channelCooldownMu.RLock()
	defer channelCooldownMu.RUnlock()
	entry, ok := channelCooldownTable[channelId]
	if !ok {
		return time.Time{}
	}
	return entry.until
}

// IsChannelCoolingDown 报告渠道是否处于冷却期。
func IsChannelCoolingDown(channelId int) bool {
	until := GetChannelCooldownUntil(channelId)
	return !until.IsZero() && time.Now().Before(until)
}

// clearExpiredChannelCooldowns 清除过期条目并返回本周期到期的渠道 id。
func clearExpiredChannelCooldowns() []int {
	now := time.Now()
	channelCooldownMu.Lock()
	recovered := make([]int, 0, 4)
	for channelId, entry := range channelCooldownTable {
		if now.Before(entry.until) {
			continue
		}
		delete(channelCooldownTable, channelId)
		recovered = append(recovered, channelId)
	}
	channelCooldownMu.Unlock()
	if len(recovered) > 0 {
		channelHealthMu.Lock()
		for _, channelId := range recovered {
			delete(healthSnapshotCache, channelId)
		}
		channelHealthMu.Unlock()
	}
	return recovered
}

// HasCoolingDownChannels 报告当前是否存在冷却中的渠道（供调度器省行）。
func HasCoolingDownChannels() bool {
	if !channel_setting.IsCooldownV2Enabled() {
		return false
	}
	channelCooldownMu.RLock()
	defer channelCooldownMu.RUnlock()
	now := time.Now()
	for _, entry := range channelCooldownTable {
		if now.Before(entry.until) {
			return true
		}
	}
	return false
}

// CooldownRecoverySweep 每轮清理过期冷却并把到期渠道从「自动禁用」恢复。
// 仅在 cooldown_v2 开启时执行；ctx 取消时尽快返回（多节点租约）。
func CooldownRecoverySweep(ctx context.Context) {
	if !channel_setting.IsCooldownV2Enabled() {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	for _, channelId := range clearExpiredChannelCooldowns() {
		ch, err := model.GetChannelById(channelId, false)
		if err != nil {
			continue
		}
		if ch.Status != common.ChannelStatusAutoDisabled {
			continue
		}
		EnableChannel(channelId, "", ch.Name)
		common.SysLog(fmt.Sprintf("channel cooldown expired, channel recovered: #%d", channelId))
	}
}
