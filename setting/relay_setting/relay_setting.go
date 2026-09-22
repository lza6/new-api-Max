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
package relay_setting

import "github.com/lza6/new-api-Max/setting/config"

// RelaySetting relay 层运行参数（热更新，注册名 "relay"）。
type RelaySetting struct {
	// StreamFallover B3-2 流式首包缓冲 fallover：缓冲 SSE 直到首个有效
	// data 块才向客户端提交响应头；首包超时判定本次渠道失败并走重试链。
	// 默认 off（不缓冲，行为与现状完全一致）。
	StreamFallover bool `json:"stream_fallover"`
	// StreamFirstTokenTimeout 首包超时（秒），默认 15；<=0 表示禁用首包超时
	// （仅缓冲不判超时）。仅在 StreamFallover 开启时生效。
	StreamFirstTokenTimeout int `json:"stream_first_token_timeout"`

	// GlobalConcurrencyEnabled T6 全局真实并发桶：开启后限制整个网关同时
	// 处理中的模型请求数；超出的请求进入有界排队，等待并发释放而不是直接
	// 拒绝。默认 off（行为与现状完全一致）。
	GlobalConcurrencyEnabled bool `json:"global_concurrency_enabled"`
	// GlobalConcurrencyLimit 全局并发上限（同时处理中的请求数）。<=0 表示
	// 不限制（仅在 Enabled 时生效）。
	GlobalConcurrencyLimit int `json:"global_concurrency_limit"`
	// GlobalConcurrencyQueue 排队容量（等待中的请求数上限）。超过则直接 429。
	// <=0 表示不允许排队（满即 429）。
	GlobalConcurrencyQueue int `json:"global_concurrency_queue"`
	// GlobalConcurrencyWaitTimeout 排队最长等待秒数；到期仍未获得并发则 429。
	// <=0 默认 30 秒。
	GlobalConcurrencyWaitTimeout int `json:"global_concurrency_wait_timeout"`

	// UserBaseRateLimitEnabled T7 每用户基础限速总开关（nil=默认开启）。
	// 开启后所有用户按 UserBaseConcurrencyLimit（默认 3，并发/秒）与
	// UserBaseRpmLimit（默认 120，RPM）限流，超限 429；管理员可关闭或调参。
	UserBaseRateLimitEnabled *bool `json:"user_base_rate_limit_enabled"`
	UserBaseConcurrencyLimit int   `json:"user_base_concurrency_limit"`
	UserBaseRpmLimit         int   `json:"user_base_rpm_limit"`

	// UserRateLimitExemptModels 限流豁免模型清单：命中这些模型的请求跳过
	// 每用户/每密钥/订阅档位的并发与 RPM 限速（如免费翻译模型 google-translate，
	// 对所有用户一律不限并发/不限速率）。空 = 不豁免任何模型。
	UserRateLimitExemptModels []string `json:"user_rate_limit_exempt_models"`

	// GroupRateLimitOverrides 分组限速覆盖：group -> 档位（0=该项不限）。
	// UserRateLimitOverrides 用户/部分用户限速覆盖：userId -> 档位。
	// 生效优先级：用户覆盖 > 分组覆盖 > 基础默认。热更新，无需 schema 变更。
	GroupRateLimitOverrides map[string]RateLimitTier `json:"group_rate_limit_overrides"`
	UserRateLimitOverrides  map[int]RateLimitTier    `json:"user_rate_limit_overrides"`

	// SubscriptionRequiredGroups 需订阅才能使用的分组清单：未订阅用户选择这些
	// 分组（自动分组或手动指定）时被拒（403）。空 = 不启用该门禁。
	SubscriptionRequiredGroups []string `json:"subscription_required_groups"`
}

// RateLimitTier 限速档位（并发 + RPM）。
type RateLimitTier struct {
	Concurrency int `json:"concurrency"`
	Rpm         int `json:"rpm"`
}

// DefaultStreamFirstTokenTimeout 首包超时默认 15 秒。
const DefaultStreamFirstTokenTimeout = 15

// 全局并发桶默认值（T6）。
const (
	DefaultGlobalConcurrencyLimit       = 0 // 0 = 不限制
	DefaultGlobalConcurrencyQueue       = 1000
	DefaultGlobalConcurrencyWaitTimeout = 30
	DefaultUserBaseConcurrencyLimit     = 3   // 每用户基础并发（请求/秒）
	DefaultUserBaseRpmLimit             = 120 // 每用户基础 RPM
)

var relaySetting = RelaySetting{
	StreamFirstTokenTimeout:  DefaultStreamFirstTokenTimeout,
	UserBaseConcurrencyLimit: DefaultUserBaseConcurrencyLimit,
	UserBaseRpmLimit:         DefaultUserBaseRpmLimit,
}

func init() {
	config.GlobalConfig.Register("relay", &relaySetting)
}

func GetRelaySetting() *RelaySetting {
	return &relaySetting
}

// GetStreamFirstTokenTimeout 返回首包超时（秒）；未配置时用默认 15。
func GetStreamFirstTokenTimeout() int {
	if s := GetRelaySetting(); s != nil && s.StreamFirstTokenTimeout > 0 {
		return s.StreamFirstTokenTimeout
	}
	return DefaultStreamFirstTokenTimeout
}

// GlobalConcurrencyGate 全局并发桶运行参数快照，供中间件读取。
type GlobalConcurrencyGate struct {
	Enabled     bool
	Limit       int
	Queue       int
	WaitTimeout int
}

func GetGlobalConcurrencyGate() GlobalConcurrencyGate {
	g := GlobalConcurrencyGate{}
	if s := GetRelaySetting(); s != nil {
		g.Enabled = s.GlobalConcurrencyEnabled
		g.Limit = s.GlobalConcurrencyLimit
		g.Queue = s.GlobalConcurrencyQueue
		g.WaitTimeout = s.GlobalConcurrencyWaitTimeout
	}
	if g.Limit <= 0 {
		g.Limit = DefaultGlobalConcurrencyLimit
	}
	if g.Queue <= 0 {
		g.Queue = DefaultGlobalConcurrencyQueue
	}
	if g.WaitTimeout <= 0 {
		g.WaitTimeout = DefaultGlobalConcurrencyWaitTimeout
	}
	return g
}

// GetUserRateLimitExemptModels 返回限流豁免模型清单（空 = 不豁免任何模型）。
func GetUserRateLimitExemptModels() []string {
	if s := GetRelaySetting(); s != nil {
		return s.UserRateLimitExemptModels
	}
	return nil
}

// GetUserRateLimitTier 解析用户生效限速档位（并发/RPM）：
// 用户覆盖 > 分组覆盖 > 基础默认；返回 0 表示该项不限（沿用既有其它限流）。
func GetUserRateLimitTier(userId int, group string) (concurrency, rpm int) {
	s := GetRelaySetting()
	if s == nil {
		return DefaultUserBaseConcurrencyLimit, DefaultUserBaseRpmLimit
	}
	if tier, ok := s.UserRateLimitOverrides[userId]; ok {
		return tier.Concurrency, tier.Rpm
	}
	if tier, ok := s.GroupRateLimitOverrides[group]; ok {
		return tier.Concurrency, tier.Rpm
	}
	enabled := true
	if s.UserBaseRateLimitEnabled != nil {
		enabled = *s.UserBaseRateLimitEnabled
	}
	if !enabled {
		return 0, 0
	}
	concurrency = s.UserBaseConcurrencyLimit
	if concurrency <= 0 {
		concurrency = DefaultUserBaseConcurrencyLimit
	}
	rpm = s.UserBaseRpmLimit
	if rpm <= 0 {
		rpm = DefaultUserBaseRpmLimit
	}
	return concurrency, rpm
}

// SetUserRateLimitOverride 设置/移除用户限速覆盖（0,0=移除；其余值整体替换档位）。
func SetUserRateLimitOverride(userId int, tier RateLimitTier) {
	s := GetRelaySetting()
	if s.UserRateLimitOverrides == nil {
		s.UserRateLimitOverrides = make(map[int]RateLimitTier)
	}
	if tier.Concurrency <= 0 && tier.Rpm <= 0 {
		delete(s.UserRateLimitOverrides, userId)
		return
	}
	s.UserRateLimitOverrides[userId] = tier
}

// SetGroupRateLimitOverride 设置/移除分组限速覆盖（0,0=移除）。
func SetGroupRateLimitOverride(group string, tier RateLimitTier) {
	s := GetRelaySetting()
	if s.GroupRateLimitOverrides == nil {
		s.GroupRateLimitOverrides = make(map[string]RateLimitTier)
	}
	if tier.Concurrency <= 0 && tier.Rpm <= 0 {
		delete(s.GroupRateLimitOverrides, group)
		return
	}
	s.GroupRateLimitOverrides[group] = tier
}

// IsSubscriptionRequiredGroup 判断分组是否被标记为"需订阅才能使用"。
func IsSubscriptionRequiredGroup(group string) bool {
	s := GetRelaySetting()
	if s == nil || group == "" || len(s.SubscriptionRequiredGroups) == 0 {
		return false
	}
	for _, g := range s.SubscriptionRequiredGroups {
		if g == group {
			return true
		}
	}
	return false
}
