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

// §4.8.2 渠道级熔断器（per-provider circuit breaker）。
//
// 与既有冷却（channel_cooldown.go，按错误类定时冷却）正交：熔断器按
// 「连续失败次数」摘除渠道，进入 Open 后由冷却窗口 + 半开探测恢复，避免
// 持续故障的上游被反复选中（对标 nexus-llm-router per-provider breaker）。
//
// 复用既有健康快照做过滤点（model.FilterChannelHealth），零额外路由改动；
// 复用冷却窗口长度作为 Open 时长，到期自动进入半开探测。
//
// 行为开关：CHANNEL_CIRCUIT_BREAKER=on|true（默认 off = 零行为变化）。
package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// 熔断参数（可用 env 覆盖）。连续失败 N 次 → Open；Open 时长内不选；
// 时长到期 → 半开（放行一次探测），探测成功 → Closed，失败 → 重新 Open。
var (
	circuitBreakerEnabled   = common.GetEnvOrDefaultBool("CHANNEL_CIRCUIT_BREAKER", false)
	circuitFailureThreshold = common.GetEnvOrDefault("CHANNEL_CIRCUIT_FAILURE_THRESHOLD", 5)
	circuitOpenDuration     = time.Duration(common.GetEnvOrDefault("CHANNEL_CIRCUIT_OPEN_SECONDS", 120)) * time.Second
)

// CircuitState 熔断器状态。
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"    // 正常放行
	CircuitOpen     CircuitState = "open"      // 熔断中，摘除
	CircuitHalfOpen CircuitState = "half_open" // 半开，放行一次探测
)

type circuitEntry struct {
	consecutiveFailures int
	openedAt            time.Time
	probing             bool // 半开探测是否已放行（避免并发多探测）
}

var (
	circuitMu    sync.Mutex
	circuitTable = map[int]*circuitEntry{}
)

// SetCircuitBreakerEnabled 测试用覆盖；nil 恢复 env 默认。
func SetCircuitBreakerEnabled(v *bool) {
	if v == nil {
		circuitBreakerEnabled = common.GetEnvOrDefaultBool("CHANNEL_CIRCUIT_BREAKER", false)
		return
	}
	circuitBreakerEnabled = *v
}

// CircuitBreakerEnabled 报告熔断器开关状态。
func CircuitBreakerEnabled() bool { return circuitBreakerEnabled }

// RegisterChannelCircuitSuccess 记录一次渠道成功（探测或正常）：重置连续失败、
// 关闭熔断。Relay 成功路径调用（fail-open，nil 安全）。
func RegisterChannelCircuitSuccess(channelId int) {
	if !circuitBreakerEnabled || channelId <= 0 {
		return
	}
	circuitMu.Lock()
	if e, ok := circuitTable[channelId]; ok {
		if e.openedAt.IsZero() && e.consecutiveFailures == 0 && !e.probing {
			circuitMu.Unlock()
			return
		}
		delete(circuitTable, channelId)
		common.SysLog(fmt.Sprintf("circuit breaker: channel #%d closed after success", channelId))
	}
	circuitMu.Unlock()
}

// RegisterChannelCircuitFailure 记录一次「应触发熔断」的渠道失败（鉴权/限流/5xx/
// 超时类）。连续失败达阈值 → Open。返回是否刚进入 Open（供日志/观测）。
func RegisterChannelCircuitFailure(channelId int, class RelayErrorClass) bool {
	if !circuitBreakerEnabled || channelId <= 0 {
		return false
	}
	// 仅「上游/渠道不可用」类失败累计；参数错误（我方问题）不计。
	switch class {
	case ErrClassAuth, ErrClassRateLimited, ErrClassServerError, ErrClassTimeout:
	default:
		return false
	}

	circuitMu.Lock()
	defer circuitMu.Unlock()
	e := circuitTable[channelId]
	if e == nil {
		e = &circuitEntry{}
		circuitTable[channelId] = e
	}
	if e.probing {
		// 半开探测失败 → 重新 Open，重置窗口。
		e.probing = false
		e.openedAt = time.Now()
		common.SysLog(fmt.Sprintf("circuit breaker: channel #%d half-open probe failed, re-opened", channelId))
		return true
	}
	e.consecutiveFailures++
	if e.consecutiveFailures >= circuitFailureThreshold {
		if e.openedAt.IsZero() {
			e.openedAt = time.Now()
			common.SysLog(fmt.Sprintf("circuit breaker: channel #%d opened after %d consecutive failures (last class=%s)",
				channelId, e.consecutiveFailures, RelayErrorClassString(class)))
			return true
		}
	}
	return false
}

// ChannelCircuitAllows 是**纯判定**（无副作用）：渠道当前是否应作为**候选**保留。
//   - 未启用 / Closed / 未知：true（保留）。
//   - Open 且未到期：false（从候选中剔除）。
//   - Open 且已到期（半开候选）：true（保留为候选，是否真正放行由 AcquireCircuitProbe
//     在发请求前决定）。
//
// §审查 C1：本函数被 ChannelHealthProbe→channelMatchesFilter 在**候选过滤**中对每个
// 候选、每轮选择（affinity 预判 + 主选择）多次调用，**必须幂等、无副作用**——绝不能
// 在此消费探测令牌，否则未最终选中/被过滤的候选会静默烧掉令牌，使渠道在 Open 窗口内
// 永不恢复。令牌消费在 AcquireCircuitProbe（真正发请求前）完成。
func ChannelCircuitAllows(channelId int) bool {
	if !circuitBreakerEnabled || channelId <= 0 {
		return true
	}
	circuitMu.Lock()
	defer circuitMu.Unlock()
	e := circuitTable[channelId]
	if e == nil || e.openedAt.IsZero() {
		return true
	}
	// Open 未到期 → 剔除；已到期（半开候选）→ 保留（不消费令牌）。
	return time.Since(e.openedAt) >= circuitOpenDuration
}

// AcquireCircuitProbe 在**真正要向上游发起请求**前调用，原子地获取半开探测令牌。
// 返回 true = 允许本次请求打到该渠道（若为半开，本次即探测请求）；false = 拒绝。
//   - Closed / 未知：true。
//   - Open 未到期：false。
//   - Open 已到期：首个调用者置 probing 并返回 true（唯一探测），其余返回 false
//     （避免并发洪峰在探测期间全量打到未确认渠道）。
//
// 关键：与 ChannelCircuitAllows 分离，保证过滤阶段的多次纯判定不消耗令牌。
func AcquireCircuitProbe(channelId int) bool {
	if !circuitBreakerEnabled || channelId <= 0 {
		return true
	}
	circuitMu.Lock()
	defer circuitMu.Unlock()
	e := circuitTable[channelId]
	if e == nil || e.openedAt.IsZero() {
		return true
	}
	if time.Since(e.openedAt) < circuitOpenDuration {
		return false
	}
	// 半开：只放行一次探测。
	if e.probing {
		return false
	}
	e.probing = true
	common.SysLog(fmt.Sprintf("circuit breaker: channel #%d half-open, allowing one probe", channelId))
	return true
}

// GetChannelCircuitState 返回渠道熔断状态（观测/管理 API 用）。
func GetChannelCircuitState(channelId int) (CircuitState, int) {
	circuitMu.Lock()
	defer circuitMu.Unlock()
	e := circuitTable[channelId]
	if e == nil || (e.openedAt.IsZero() && e.consecutiveFailures == 0) {
		return CircuitClosed, 0
	}
	if e.openedAt.IsZero() {
		return CircuitClosed, e.consecutiveFailures
	}
	if time.Since(e.openedAt) >= circuitOpenDuration {
		return CircuitHalfOpen, e.consecutiveFailures
	}
	return CircuitOpen, e.consecutiveFailures
}

// resetCircuitTableForTest 清空熔断表（测试用）。
func resetCircuitTableForTest() {
	circuitMu.Lock()
	circuitTable = map[int]*circuitEntry{}
	circuitMu.Unlock()
}
