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

// 渠道治理域公共错误分类器。
//
// 设计模型：将「是否可重试(eretryability)」与「是否需要冷却(cooldown)」
// 正交解耦，思想源自 aisix 网关的 cooldown 模型：
//
//	可重试 ≠ 就应立刻重试 —— 401/403 虽不可重试，但必须冷却，否则后续每个
//	请求都会重复撞同一次鉴权失败；429 可协调重试，但必须尊重服务端给出的
//	Retry-After 冷却窗口。
//
// 本文件是后续 Batch 的地基：B2-2（任务 unconfirmed 三态提交）、B3 渠道健康
// 治理、B4 提交重试队列都依赖这里的分类。签名一旦冻结不再修改。
//
// 错误分类语义：
//   - ErrClassAuth        : 401/403/407，key 失效或权限不足，不可重试，必须冷却
//   - ErrClassRateLimited : 429，可冷却后重试，尊重 Retry-After
//   - ErrClassServerError : 5xx（除 408），服务端故障，可有限重试，也要冷却
//   - ErrClassTimeout     : 网络层错误（status<=0）或 408，结果不可确认，
//     提交类请求走 unconfirmed 流程（见 B2-2）
//   - ErrClassBadRequest  : 4xx 参数问题（我方请求错误），不可重试不冷却
//   - ErrClassOK          : 2xx 成功，无错误语义
//   - ErrClassCapability  : 模型/上游不支持该请求能力（可由上层依据 content
//     提升，仅状态码无法判定）
//   - ErrClassUnknown     : 无法归类的状态码
package service

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RelayErrorClass 上游错误分类。
type RelayErrorClass int

const (
	ErrClassOK RelayErrorClass = iota
	ErrClassAuth
	ErrClassRateLimited
	ErrClassServerError
	ErrClassTimeout
	ErrClassBadRequest
	ErrClassCapability
	ErrClassUnknown
)

// RelayErrorClassString 返回错误类的稳定短标识，供渠道健康快照的
// last_cool_class 字段（B5-2 hover 原因）与日志使用。
func RelayErrorClassString(class RelayErrorClass) string {
	switch class {
	case ErrClassAuth:
		return "auth"
	case ErrClassRateLimited:
		return "rate_limited"
	case ErrClassServerError:
		return "server_error"
	case ErrClassTimeout:
		return "timeout"
	case ErrClassBadRequest:
		return "bad_request"
	case ErrClassCapability:
		return "capability"
	case ErrClassOK:
		return "ok"
	default:
		return "unknown"
	}
}

// DefaultCooldownCap 错误类冷却窗口上限：429/5xx/鉴权失败最多冷却 15 分钟，
// 避免上游持续降级时网关把自己"冷却"成不可用。
const DefaultCooldownCap = 15 * time.Minute

// ClassifyHTTPStatus 将上游 HTTP 状态码分类为 (可重试?, 需冷却?, 错误类)。
// status <= 0 表示网络层错误（连接超时/拒绝/重置等），归类为 ErrClassTimeout。
// retryAfter 仅供 429 场景使用；其余状态码下该参数被忽略。
func ClassifyHTTPStatus(status int, retryAfter string) (retryable bool, needCooldown bool, class RelayErrorClass) {
	// 网络层错误：没有 HTTP 响应，结果不可确认（远端可能已处理请求）。
	if status <= 0 {
		return false, true, ErrClassTimeout
	}
	switch {
	case status >= 200 && status < 300:
		return false, false, ErrClassOK
	case status == http.StatusUnauthorized, status == http.StatusForbidden, status == http.StatusProxyAuthRequired:
		// 不可重试（重试同样会撞鉴权失败），但必须冷却，避免热循环。
		return false, true, ErrClassAuth
	case status == http.StatusTooManyRequests:
		return true, true, ErrClassRateLimited
	case status == http.StatusRequestTimeout:
		return false, true, ErrClassTimeout
	case status >= 500:
		// 服务端故障可有限重试，但要冷却，防止雪崩。
		return true, true, ErrClassServerError
	default:
		return false, false, ErrClassBadRequest
	}
}

// ShouldBackoff429 429 有界退避判定：状态为 429 且已用退避次数未达上限。
// max429Retries < 0 表示不启用退避（兼容旧行为）。
func ShouldBackoff429(status int, used429Retries int, max429Retries int) bool {
	return status == http.StatusTooManyRequests && max429Retries > 0 && used429Retries < max429Retries
}

// RetryAfterCooldown 解析 HTTP Retry-After 头为冷却时长。
// 支持两种标准格式：秒数（"120"）与 HTTP-date。
// 非法值、0 和已过期的 date 返回 0；超过 cap 时钳制到 cap。
// maxCap <= 0 时使用 DefaultCooldownCap。
func RetryAfterCooldown(retryAfter string, maxCap time.Duration) time.Duration {
	if maxCap <= 0 {
		maxCap = DefaultCooldownCap
	}
	raw := strings.TrimSpace(retryAfter)
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs <= 0 {
			return 0
		}
		d := time.Duration(secs) * time.Second
		if d > maxCap {
			return maxCap
		}
		return d
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0
		}
		if d > maxCap {
			return maxCap
		}
		return d
	}
	return 0
}
