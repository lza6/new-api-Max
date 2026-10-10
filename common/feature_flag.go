package common

import (
	"strconv"
	"sync/atomic"

	"github.com/lza6/new-api-Max/constant"
)

// 能力开关（Feature Switch）运行时存储 —— Batch-8 / G1 的底座。
//
// 背景：本项目有 13 个「能力已实现、默认关、且管理端无入口」的开关，值只在进程
// 启动时从 env 读取一次（`var x = GetEnvOrDefaultBool(...)`）。要把它们变成可灰度
// 的运营能力，必须让**管理端能改**且**改完立即生效**。
//
// 关键设计：**不引入「共享可变包级 bool 变量」**。那种写法（管理端写 while 热路径读）
// 在 Go 内存模型下是无同步并发读写，属未定义行为（可触发不可 recover 的 fatal，
// 本项目 4.2.2 已有同类教训）。这里改为 atomic.Pointer 发布**不可变快照**：
// 读侧 Load() 后只读，写侧构造新 map 再 Store() —— 无锁、无竞争、无 race。
//
// 分层：`common` 是最底层包，不能反向依赖 `setting/feature_switch`。故
//   - key 常量 + 覆盖存储（本文件）放 common；
//   - 元数据（标题/风险/依赖/度量）+ 持久化 + 管理端语义放 setting/feature_switch。
//
// 值来源优先级：**运行时覆盖（管理端热更新 / 测试） > 调用方传入的 fallback（env 默认）**。

// 能力开关 key —— 字面量即 env 变量名，属**稳定契约**，不得随版本改名
// （改名会让已持久化的管理端配置失效）。
const (
	FlagComplexityRouting       = "COMPLEXITY_ROUTING"
	FlagToolDrawerEnabled       = "TOOL_DRAWER_ENABLED"
	FlagRelayAuditEnabled       = "RELAY_AUDIT_ENABLED"
	FlagPolicyEngineMode        = "POLICY_ENGINE_MODE"
	FlagChannelHealthWeightedLB = "CHANNEL_HEALTH_WEIGHTED_LB"
	FlagDomainRouteEnabled      = "DOMAIN_ROUTE_ENABLED"
	FlagMemoryInjectionEnabled  = "MEMORY_INJECTION_ENABLED"
	FlagChannelCircuitBreaker   = "CHANNEL_CIRCUIT_BREAKER"
	FlagChannelKeyEncryption    = "CHANNEL_KEY_ENCRYPTION"
	FlagPasswordLoginEncryption = "PASSWORD_LOGIN_ENCRYPTION_ENABLED"
	FlagCatalogSyncTaskEnabled  = "CATALOG_SYNC_TASK_ENABLED"
	FlagErrorLogEnabled         = "ERROR_LOG_ENABLED"
	FlagGetMediaTokenNotStream  = "GET_MEDIA_TOKEN_NOT_STREAM"
)

// FeatureFlagKeys 返回全部受治理开关的 key（顺序不保证，调用方按需排序）。
func FeatureFlagKeys() []string {
	return []string{
		FlagComplexityRouting,
		FlagToolDrawerEnabled,
		FlagRelayAuditEnabled,
		FlagPolicyEngineMode,
		FlagChannelHealthWeightedLB,
		FlagDomainRouteEnabled,
		FlagMemoryInjectionEnabled,
		FlagChannelCircuitBreaker,
		FlagChannelKeyEncryption,
		FlagPasswordLoginEncryption,
		FlagCatalogSyncTaskEnabled,
		FlagErrorLogEnabled,
		FlagGetMediaTokenNotStream,
	}
}

// featureFlagOverrides 已发布的**不可变**覆盖快照。nil 表示无任何覆盖（全部走 fallback）。
//
// 存 string 而非 bool：`POLICY_ENGINE_MODE` 是枚举（off|shadow|enforce），
// 与布尔开关共用同一张表可以避免维护两套优先级规则。
var featureFlagOverrides atomic.Pointer[map[string]string]

// featureFlagLookup 从已发布快照中查一个 key。第二个返回值为「是否存在覆盖」。
func featureFlagLookup(key string) (string, bool) {
	p := featureFlagOverrides.Load()
	if p == nil {
		return "", false
	}
	v, ok := (*p)[key]
	return v, ok
}

// FeatureFlagValue 返回布尔型开关当前值。
// 有运行时覆盖时以覆盖为准；否则返回 fallback（通常是该模块的 env 默认）。
// 覆盖值无法解析为布尔时回退 fallback（配置损坏不得让热路径 panic）。
func FeatureFlagValue(key string, fallback bool) bool {
	raw, ok := featureFlagLookup(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

// FeatureFlagString 返回枚举/字符串型开关当前值。
// 有运行时覆盖时以覆盖为准；否则返回 fallback。空字符串覆盖视为未设置。
func FeatureFlagString(key string, fallback string) string {
	raw, ok := featureFlagLookup(key)
	if !ok || raw == "" {
		return fallback
	}
	return raw
}

// SetFeatureFlagOverride 设置或清除单个开关的运行时覆盖。
// v 为 nil 表示清除（回退 fallback，即 env 默认）——测试用它做还原。
//
// 热更新路径：`setting/feature_switch.SetEnabled` 在持久化成功后调用本函数，
// 使改动立即对热路径生效。
func SetFeatureFlagOverride(key string, v *string) {
	cur := featureFlagOverrides.Load()
	next := make(map[string]string, len(derefFeatureFlagMap(cur))+1)
	for k, val := range derefFeatureFlagMap(cur) {
		next[k] = val
	}
	if v == nil {
		delete(next, key)
	} else {
		next[key] = *v
	}
	if len(next) == 0 {
		// 显式发布 nil：回到「无覆盖」态，避免空 map 与 nil 语义分叉。
		featureFlagOverrides.Store(nil)
		return
	}
	featureFlagOverrides.Store(&next)
}

// ReplaceFeatureFlagOverrides 用给定集合**整体替换**覆盖快照。
// 启动时从持久化配置装载（`setting/feature_switch` 的 configWriteHook 会调用）。
// 传入空集合等价于清空全部覆盖。
func ReplaceFeatureFlagOverrides(values map[string]string) {
	if len(values) == 0 {
		featureFlagOverrides.Store(nil)
		return
	}
	next := make(map[string]string, len(values))
	for k, v := range values {
		if k == "" {
			continue
		}
		next[k] = v
	}
	if len(next) == 0 {
		featureFlagOverrides.Store(nil)
		return
	}
	featureFlagOverrides.Store(&next)
}

// FeatureFlagOverrides 返回当前全部覆盖的**副本**（供持久化与排障；不含 fallback 值）。
func FeatureFlagOverrides() map[string]string {
	cur := derefFeatureFlagMap(featureFlagOverrides.Load())
	out := make(map[string]string, len(cur))
	for k, v := range cur {
		out[k] = v
	}
	return out
}

// ClearFeatureFlagOverrides 清空全部覆盖（仅测试与「恢复出厂」使用）。
func ClearFeatureFlagOverrides() { featureFlagOverrides.Store(nil) }

// BoolFeatureFlagOverride 把各模块既有的 `Set*Enabled(v *bool)` 测试入参
// （nil = 恢复 env 默认）转换为存储层需要的 `*string`。nil 原样透传。
func BoolFeatureFlagOverride(v *bool) *string {
	if v == nil {
		return nil
	}
	s := strconv.FormatBool(*v)
	return &s
}

// 以下四个 getter 服务于「值由 common / constant 包持有导出变量」的开关
// （它们在 env 初始化时被赋值，且既有测试会**直接给变量赋值**）。
//
// 处理方式：**保留导出变量不动**（测试写法零改动），只把**生产读点**改为调用
// getter。getter 优先取管理端覆盖，未设置时回退到该变量 —— 既让管理端能热更新，
// 又避免把变量改成 atomic 而破坏测试的直接赋值写法。
//
// 注意：这四处**不做运行时写回变量**（写回会重新引入无同步并发读写）。因此管理端
// 修改后对**已读取过旧值的代码路径**要等下一次读取，行为是"下一次读取即生效"，
// 与热更新语义一致。

// ChannelKeyEncryptionEnabledValue 返回渠道密钥加密的**生效值**。
func ChannelKeyEncryptionEnabledValue() bool {
	return FeatureFlagValue(FlagChannelKeyEncryption, ChannelKeyEncryptionEnabled)
}

// PasswordLoginEncryptionEnabledValue 返回密码登录传输加密的**生效值**。
func PasswordLoginEncryptionEnabledValue() bool {
	return FeatureFlagValue(FlagPasswordLoginEncryption, PasswordLoginEncryptionEnabled)
}

// ErrorLogEnabledValue 返回错误日志落库总闸的**生效值**。
func ErrorLogEnabledValue() bool {
	return FeatureFlagValue(FlagErrorLogEnabled, constant.ErrorLogEnabled)
}

// MediaTokenNotStreamValue 返回媒体 token 非流式返回的**生效值**。
func MediaTokenNotStreamValue() bool {
	return FeatureFlagValue(FlagGetMediaTokenNotStream, constant.GetMediaTokenNotStream)
}

// derefFeatureFlagMap 把可能为 nil 的快照指针解成可安全 range 的 map。
// 分离出来是为了让上面每个写路径都写一次 nil 判断而不是各自重复。
func derefFeatureFlagMap(p *map[string]string) map[string]string {
	if p == nil {
		return nil
	}
	return *p
}
