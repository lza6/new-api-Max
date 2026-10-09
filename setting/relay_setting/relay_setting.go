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

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/config"
)

// RelaySetting relay 层运行参数（热更新，注册名 "relay"）。
type RelaySetting struct {
	// StreamFallover B3-2 流式首包缓冲 fallover：缓冲 SSE 直到首个有效
	// data 块才向客户端提交响应头；首包超时判定本次渠道失败并走重试链。
	// 默认 on：流式首包缓冲 fallover 默认开启（B3-2 灰度通过）；
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

	// NonStreamFirstByteTimeout 非流式请求的上游首字节（完整响应头）超时秒数；
	// 0 = 关闭。超时返回 504 并提示使用流式（不 skip retry，可换渠道重试）。
	NonStreamFirstByteTimeout int `json:"non_stream_first_byte_timeout"`

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

	// RequestCompressionEnabled 出站请求体压缩总开关（nil=默认开启，沿用 env
	// RELAY_REQUEST_COMPRESSION_ENABLED）。置 false 时任何渠道都不压缩。管理员
	// 在「请求限制 → 请求体压缩」中可热更新。
	RequestCompressionEnabled *bool `json:"request_compression_enabled"`
	// RequestCompressionThresholdKB 触发压缩的最小请求体（KB，默认 50）。请求体
	// 小于该值时压缩收益不足以覆盖 CPU 开销，直接明文转发。管理员可热更新；
	// <=0 时回退 env 默认（RELAY_REQUEST_COMPRESSION_THRESHOLD_KB）。
	RequestCompressionThresholdKB int `json:"request_compression_threshold_kb"`
	// RequestCompressionLevel gzip 压缩级别（1=BestSpeed..9=BestCompression，
	// 默认 6=Default）。级别越高压缩率越好但 CPU 越多。<=0 或超范围时用默认 6。
	// 管理员可热更新（面板展示压缩率与压缩耗时供权衡）。
	RequestCompressionLevel int `json:"request_compression_level"`
	// RequestCompressionMaxMB 触发压缩的**最大**请求体（MB，默认 20）。请求体大于该
	// 值时不压缩——超大 body 压缩极耗 CPU（本站 2C/4C 小机尤甚），明文直发更快。
	// <=0 表示不设上限。管理员可热更新。
	RequestCompressionMaxMB int `json:"request_compression_max_mb"`

	// PolicyMaxPromptChars 统一策略中心的「提示词长度护栏」上限（字符）。0 = 不限制。
	// 仅在 POLICY_ENGINE_MODE=enforce 时真正拦截；shadow 模式只记录「本应拦截」。
	// 这是策略引擎三类判定的**唯一**新增判定源：预算与限流已分别由预扣费
	// （service.PreConsumeBilling）与每用户限速中间件（middleware.UserRateLimit）
	// 强制执行，策略引擎只读取它们的真实状态做统一观测，不重复实现。
	PolicyMaxPromptChars int `json:"policy_max_prompt_chars"`
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
	DefaultNonStreamFirstByteTimeout    = 300 // 非流式首字节超时默认 300s（0=关闭）
)

// DefaultRequestCompressionThresholdKB 出站请求体压缩阈值默认值（50KB）。
// 从 50KB 起压，覆盖更广的中大 prompt；管理员可在设置页用预设档位或自定义调整。
// 与 common.DefaultRequestCompressionThresholdKB 同源（common 无依赖，避免循环）。
const DefaultRequestCompressionThresholdKB = common.DefaultRequestCompressionThresholdKB

var relaySetting = RelaySetting{
	StreamFallover:            true,
	StreamFirstTokenTimeout:   DefaultStreamFirstTokenTimeout,
	UserBaseConcurrencyLimit:  DefaultUserBaseConcurrencyLimit,
	UserBaseRpmLimit:          DefaultUserBaseRpmLimit,
	NonStreamFirstByteTimeout: DefaultNonStreamFirstByteTimeout,
}

// settingMu 保护 relaySetting（主副本）。所有写入（配置热更新反射写入、Set* 方法）
// 必须持写锁，并在写完后发布新快照。反射式读取主副本（ExportAllConfigs/SaveToDB）
// 持读锁，与写入互斥 —— 后者不走快照，故必须与写同锁串行（4.2.2 补）。
var settingMu sync.RWMutex

// relaySettingSnapshot 已发布的**不可变**配置快照。读侧只 Load() 后只读访问，
// 永不触碰正被写入的主副本 —— 消除「热路径读 map」与「热更新就地写 struct/map」
// 的 data race（4.2.2）。
var relaySettingSnapshot atomic.Pointer[RelaySetting]

// publishSettingSnapshotLocked 在**持有 settingMu** 的前提下，把 relaySetting 深拷贝
// 为一份不可变快照并发布。map/slice/指针字段必须深拷贝，否则快照与主副本共享底层
// 存储，读侧仍会与后续写入竞争（旧值被就地改写）。
func publishSettingSnapshotLocked() {
	snap := relaySetting // 值拷贝：标量字段独立
	snap.UserRateLimitOverrides = maps.Clone(relaySetting.UserRateLimitOverrides)
	snap.GroupRateLimitOverrides = maps.Clone(relaySetting.GroupRateLimitOverrides)
	snap.UserRateLimitExemptModels = slices.Clone(relaySetting.UserRateLimitExemptModels)
	snap.SubscriptionRequiredGroups = slices.Clone(relaySetting.SubscriptionRequiredGroups)
	if relaySetting.UserBaseRateLimitEnabled != nil {
		enabled := *relaySetting.UserBaseRateLimitEnabled
		snap.UserBaseRateLimitEnabled = &enabled
	}
	relaySettingSnapshot.Store(&snap)
}

// loadSetting 返回当前已发布的不可变快照（无锁读）。初始化后快照必非 nil；
// 极端早期（init 前）兜底返回主副本指针以保持旧行为。
func loadSetting() *RelaySetting {
	if s := relaySettingSnapshot.Load(); s != nil {
		return s
	}
	return &relaySetting
}

// UpdateRelaySetting 在写锁内修改配置主副本并发布新快照（供运行时变更与测试使用）。
// 读侧（各 getter）始终读快照，因此本函数返回后新值即对所有读者可见且无竞争。
func UpdateRelaySetting(fn func(*RelaySetting)) {
	settingMu.Lock()
	defer settingMu.Unlock()
	fn(&relaySetting)
	publishSettingSnapshotLocked()
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook：配置热更新
// （updateConfigFromMap 反射就地写 &relaySetting）期间持有写锁，写完后发布新快照。
func (r *RelaySetting) BeforeConfigWrite() { settingMu.Lock() }
func (r *RelaySetting) AfterConfigWrite() {
	publishSettingSnapshotLocked()
	settingMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard：反射式读取主副本
// （configToMap，供 ExportAllConfigs/SaveToDB 使用）期间持读锁，与热更新写入互斥。
func (r *RelaySetting) LockConfigRead()   { settingMu.RLock() }
func (r *RelaySetting) UnlockConfigRead() { settingMu.RUnlock() }

func init() {
	config.GlobalConfig.Register("relay", &relaySetting)
	// 发布初始快照，使所有 getter 从启动起就只读不可变数据。
	settingMu.Lock()
	publishSettingSnapshotLocked()
	settingMu.Unlock()
}

// GetRelaySetting 返回当前配置的**不可变快照**。只读；修改请用 UpdateRelaySetting。
func GetRelaySetting() *RelaySetting {
	return loadSetting()
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

// GetNonStreamFirstByteTimeout 返回非流式请求上游首字节超时秒数（0=关闭）。
func GetNonStreamFirstByteTimeout() int {
	if s := GetRelaySetting(); s != nil {
		return s.NonStreamFirstByteTimeout
	}
	return DefaultNonStreamFirstByteTimeout
}

// GetRequestCompressionEnabled 返回出站请求体压缩是否启用（nil=默认 true）。
func GetRequestCompressionEnabled() bool {
	if s := GetRelaySetting(); s != nil && s.RequestCompressionEnabled != nil {
		return *s.RequestCompressionEnabled
	}
	return true
}

// GetRequestCompressionThresholdKB 返回出站请求体压缩阈值（KB）。
// 管理员配置 >0 时生效；否则回退 env 默认（common.RelayRequestCompressionThresholdKB）。
func GetRequestCompressionThresholdKB() int {
	if s := GetRelaySetting(); s != nil && s.RequestCompressionThresholdKB > 0 {
		return s.RequestCompressionThresholdKB
	}
	return DefaultRequestCompressionThresholdKB
}

// DefaultRequestCompressionLevel gzip 压缩级别默认值（6=DefaultCompression）。
// 6 在压缩率与 CPU 间平衡：对可压内容比 BestSpeed(1) 明显更优，大 body 压缩仍
// 仅百 ms 级，适合 2C2G。范围为 gzip.HuffmanOnly(-2)..gzip.BestCompression(9)。
const DefaultRequestCompressionLevel = 6

// GetRequestCompressionLevel 返回 gzip 压缩级别。管理员配置在有效范围 [1,9] 内
// 时生效；否则用默认 6。
func GetRequestCompressionLevel() int {
	if s := GetRelaySetting(); s != nil && s.RequestCompressionLevel >= 1 && s.RequestCompressionLevel <= 9 {
		return s.RequestCompressionLevel
	}
	return DefaultRequestCompressionLevel
}

// DefaultRequestCompressionMaxMB 触发压缩的最大请求体（MB）。超过则不压缩——超大
// body 压缩极耗 CPU（小机尤甚）。20MB 覆盖绝大多数 prompt，超出者明文直发更快。
const DefaultRequestCompressionMaxMB = 20

// GetRequestCompressionMaxMB 返回触发压缩的最大请求体（MB）。
// 管理员配置 >0 时生效；否则默认 20；<=0（未配置）与显式 0 的区分：未配置回退默认，
// 管理员若想要「不设上限」需显式设一个很大的值（不建议）。
func GetRequestCompressionMaxMB() int {
	if s := GetRelaySetting(); s != nil && s.RequestCompressionMaxMB > 0 {
		return s.RequestCompressionMaxMB
	}
	return DefaultRequestCompressionMaxMB
}

// GetPolicyMaxPromptChars 返回统一策略中心的提示词长度护栏上限（字符）。
// 0 = 不限制（默认，零行为变化）。
func GetPolicyMaxPromptChars() int {
	if s := GetRelaySetting(); s != nil {
		return s.PolicyMaxPromptChars
	}
	return 0
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
// 在写锁内改主副本并发布新快照，读侧无需加锁即可见。
func SetUserRateLimitOverride(userId int, tier RateLimitTier) {
	UpdateRelaySetting(func(s *RelaySetting) {
		if tier.Concurrency <= 0 && tier.Rpm <= 0 {
			delete(s.UserRateLimitOverrides, userId)
			return
		}
		if s.UserRateLimitOverrides == nil {
			s.UserRateLimitOverrides = make(map[int]RateLimitTier)
		}
		s.UserRateLimitOverrides[userId] = tier
	})
}

// SetGroupRateLimitOverride 设置/移除分组限速覆盖（0,0=移除）。
func SetGroupRateLimitOverride(group string, tier RateLimitTier) {
	UpdateRelaySetting(func(s *RelaySetting) {
		if tier.Concurrency <= 0 && tier.Rpm <= 0 {
			delete(s.GroupRateLimitOverrides, group)
			return
		}
		if s.GroupRateLimitOverrides == nil {
			s.GroupRateLimitOverrides = make(map[string]RateLimitTier)
		}
		s.GroupRateLimitOverrides[group] = tier
	})
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
