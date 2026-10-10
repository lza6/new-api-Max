package feature_switch

import (
	"sort"
	"strings"
	"sync"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/config"
)

// 能力开关注册表（Batch-8 / G1）。
//
// 解决的问题：本项目有 13 个「能力已实现、默认关、且管理端无入口」的开关 —— 能力
// 写完了却停在 `默认关`，既没人能打开，也没人知道打开后有没有用（已投入的开发成本
// 等于零）。本包把它们从「env-only」升级为「可持久化 + 可热更新 + 可见度量 + 可回滚」。
//
// 分工：
//   - 本包负责**元数据**（标题/描述/风险/依赖/度量/回滚提示）+ **持久化** + **管理端语义**；
//   - **运行时值的存储**在 `common.FeatureFlagValue`（atomic 快照，无锁无 race），
//     因为 `common` 是最底层包不能反向依赖本包；
//   - 各能力模块把自己的 getter 改为查 `common` 快照（见各模块的 `XxxEnabled()`）。
//
// 持久化：注册进 `setting/config` 框架，落库为单个选项 `feature_switch.values`。
// 该框架的 `configWriteHook` 在**启动装载**与**管理端保存**两条路径上都会触发，
// 所以「重启后生效」与「立即热更新」共用同一条链路，无需第二套机制。

// ModuleName 是 `setting/config` 注册名，同时也是选项前缀（`feature_switch.values`）。
const ModuleName = "feature_switch"

// ValuesFieldKey 是持久化选项的字段名（完整选项键为 `feature_switch.values`）。
const ValuesFieldKey = "values"

// Risk 描述打开该开关的爆炸半径。
type Risk string

const (
	RiskLow    Risk = "low"    // 只读观测或纯日志，零行为变化
	RiskMedium Risk = "medium" // 改变请求处理路径，但可即时回退
	RiskHigh   Risk = "high"   // 改变数据落库形态或鉴权协议，需二次确认
)

// Kind 描述取值类型。
type Kind string

const (
	KindBool Kind = "bool"
	KindEnum Kind = "enum" // 如 POLICY_ENGINE_MODE: off|shadow|enforce
)

// FeatureSwitch 一个受治理能力开关的元数据。
type FeatureSwitch struct {
	// Key 是 env 变量名，也是稳定契约（改名会让已持久化的管理端配置失效）。
	Key string
	// Kind 取值类型；默认 KindBool。
	Kind Kind
	// Options 是 KindEnum 时的合法取值集合。
	Options []string
	// Default 是默认值（bool 用 "true"/"false"）。
	Default string
	// TitleKey / DescriptionKey 是 i18n 键（英文源串）。
	TitleKey       string
	DescriptionKey string
	// Risk 打开该开关的爆炸半径。
	Risk Risk
	// DependsOn 是前置开关 key 列表：任一前置未开启时本开关**视为未生效**。
	DependsOn []string
	// AdminEditable 为 false 时管理端只读展示（例如必须先做数据迁移的开关）。
	AdminEditable bool
	// RequiresRestart 为 true 时，管理端保存的值**不会立即热更新**，需重启进程生效。
	// 当前 13 个开关全部可热更新（false）；保留该字段是因为后续接入「必须重启」
	// 的能力时，UI 需要如实标注，不能让管理员以为已经生效。
	RequiresRestart bool
	// MetricKeys 是可在 /metrics 中观察该开关效果的指标名（空 = 暂无接入指标）。
	MetricKeys []string
	// RollbackHint 是回滚提示（英文源串，走 i18n）。
	RollbackHint string
}

// registry 是 13 个受治理开关的元数据表。
//
// 刻意**未纳入**的开关：TLS_INSECURE_SKIP_VERIFY、SMTP_STARTTLS_ENABLE、
// SMTP_INSECURE_SKIP_VERIFY、SKIP_64BIT_QUOTA_SCHEMA_CHECK、GENERATE_DEFAULT_TOKEN
// —— 它们是传输/校验类 escape hatch，默认关本就是正确姿势，不该有管理端一键入口。
var registry = []FeatureSwitch{
	{
		Key: common.FlagComplexityRouting, Kind: KindBool, Default: "false",
		TitleKey:       "Complexity Routing",
		DescriptionKey: "Scores each request on seven local dimensions (length, code, math, reasoning, tools, multimodal, multi-turn) and records the tier in the request log. Currently observation-only: it does not change channel selection.",
		Risk:           RiskLow, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turn off to stop scoring; no request behavior changes either way.",
	},
	{
		Key: common.FlagToolDrawerEnabled, Kind: KindBool, Default: "false",
		TitleKey:       "Tool Schema Deduplication",
		DescriptionKey: "Removes duplicated tool definitions from requests to save prompt tokens. Semantically equivalent, but clients that rely on the exact submitted schema layout may be affected.",
		Risk:           RiskMedium, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turn off to send tool definitions through untouched.",
	},
	{
		Key: common.FlagRelayAuditEnabled, Kind: KindBool, Default: "false",
		TitleKey:       "Relay Consistency Audit",
		DescriptionKey: "Read-only checks after each relay (SSE event whitelist, usage monotonicity, upstream error leakage, model fingerprint). Findings are recorded on the log entry.",
		Risk:           RiskLow, AdminEditable: true,
		MetricKeys:   []string{"relay_audit_findings_total"},
		RollbackHint: "Turn off for zero allocations and zero overhead.",
	},
	{
		Key: common.FlagPolicyEngineMode, Kind: KindEnum,
		Options: []string{"off", "shadow", "enforce"}, Default: "off",
		TitleKey:       "Policy Engine Mode",
		DescriptionKey: "Unified policy center over guardrails, budget and rate limits. shadow records what would be blocked without blocking; enforce rejects with an explainable error naming the matched rule.",
		Risk:           RiskMedium, AdminEditable: true,
		MetricKeys:   []string{"policy_engine_eval_total", "policy_decision_total"},
		RollbackHint: "Set back to off. shadow is always safe; only enforce changes request outcomes.",
	},
	{
		Key: common.FlagChannelHealthWeightedLB, Kind: KindBool, Default: "false",
		TitleKey:       "Health-Weighted Load Balancing",
		DescriptionKey: "Selects channels by base weight multiplied by a health factor so healthy channels are picked more often.",
		Risk:           RiskMedium, AdminEditable: true,
		MetricKeys:   []string{"channel_health_score_avg", "channels_tracked"},
		RollbackHint: "Turn off to fall back to plain weighted random selection.",
	},
	{
		Key: common.FlagDomainRouteEnabled, Kind: KindBool, Default: "false",
		TitleKey:       "Domain-Aware Routing",
		DescriptionKey: "Routes requests carrying an X-Route-Tag header to a mapped group. The mapping table is still configured by the DOMAIN_ROUTE_MAP environment variable; targets outside the caller's usable groups are ignored.",
		Risk:           RiskMedium, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turn off to ignore X-Route-Tag entirely.",
	},
	{
		Key: common.FlagMemoryInjectionEnabled, Kind: KindBool, Default: "false",
		TitleKey:       "User Memory and Skill Injection",
		DescriptionKey: "Injects each user's own memory and skill snippets into the system prompt. The snippet is sent to the upstream provider, so enable only where users understand this.",
		Risk:           RiskMedium, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turn off to stop injecting; user profile content stays stored but unused.",
	},
	{
		Key: common.FlagChannelCircuitBreaker, Kind: KindBool, Default: "false",
		TitleKey:       "Channel Circuit Breaker",
		DescriptionKey: "Opens a channel after consecutive failures, skips it while open, then half-opens to probe recovery.",
		Risk:           RiskMedium, AdminEditable: true,
		MetricKeys:   []string{"channel_circuit_open_total", "channels_tracked"},
		RollbackHint: "Turn off to return to the previous retry and failover behavior.",
	},
	{
		Key: common.FlagChannelKeyEncryption, Kind: KindBool, Default: "false",
		TitleKey:       "Channel Key Encryption",
		DescriptionKey: "Stores channel keys with AES-256-GCM. Enabling also encrypts existing plaintext keys in place. Requires an explicitly configured CRYPTO_SECRET: without it a restart can make all stored keys undecryptable and every channel will fail authentication.",
		Risk:           RiskHigh, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turning this off is NOT a safe rollback once keys are ciphertext: the ciphertext would be sent upstream and every channel would fail with 401. Decrypt the keys back to plaintext first.",
	},
	{
		Key: common.FlagPasswordLoginEncryption, Kind: KindBool, Default: "false",
		TitleKey:       "Password Login Transport Encryption",
		DescriptionKey: "Encrypts the password field of the login request with an asymmetric key. Enabling loads or provisions the login encryption key; clients that cannot encrypt fall back to plaintext submission.",
		Risk:           RiskHigh, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turn off to accept plaintext password submission again.",
	},
	{
		Key: common.FlagCatalogSyncTaskEnabled, Kind: KindBool, Default: "false",
		TitleKey:       "Scheduled Model Catalog Sync",
		DescriptionKey: "Periodically syncs the upstream model catalog (entries, descriptions, icons, tags, endpoints) into the local model and vendor tables. Only rows flagged as officially synced are touched.",
		Risk:           RiskLow, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turn off to stop the scheduled sync; already synced rows are left as they are.",
	},
	{
		Key: common.FlagErrorLogEnabled, Kind: KindBool, Default: "false",
		TitleKey:       "Persist Error Logs",
		DescriptionKey: "Writes relay errors that currently only reach the process log into the log database so they appear in the admin log view.",
		Risk:           RiskLow, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turn off to stop persisting error rows. Confirm log database capacity and rotation first.",
	},
	{
		Key: common.FlagGetMediaTokenNotStream, Kind: KindBool, Default: "false",
		TitleKey:       "Non-Streaming Media Token Response",
		DescriptionKey: "Requests media token usage as a single non-streaming response instead of a stream. Only enable for upstream providers that actually support it.",
		Risk:           RiskMedium, AdminEditable: true,
		MetricKeys:   []string{},
		RollbackHint: "Turn off to return to streaming responses.",
	},
	{
		Key: common.FlagResponseCacheEnabled, Kind: KindBool, Default: "false",
		TitleKey:       "Response Cache",
		DescriptionKey: "Serves byte-identical non-streaming requests from an in-process (or Redis) cache instead of calling the upstream again. Caching is limited to the models you list in the response cache settings; entries are isolated per user unless you explicitly allow sharing.",
		Risk:           RiskMedium, AdminEditable: true,
		MetricKeys:   []string{"response_cache_hits_total", "response_cache_misses_total", "response_cache_live_entries"},
		RollbackHint: "Turn off to bypass the cache entirely; upstream calls resume immediately.",
	},
}

// featureSwitchConfig 是注册进 config 框架的持久化载体。
//
// Values 只保存**管理员显式设置过**的开关（键为 env 名，值为字符串）；未出现的 key
// 走 env 默认 —— 这样「未配置」与「显式设为默认值」语义可区分，UI 才能如实展示
// 「已配置 / 使用默认」。
type featureSwitchConfig struct {
	Values map[string]string `json:"values"`

	mu sync.RWMutex
}

var setting = featureSwitchConfig{Values: map[string]string{}}

func init() {
	config.GlobalConfig.Register(ModuleName, &setting)
}

// BeforeConfigWrite / AfterConfigWrite / LockConfigRead / UnlockConfigRead 实现
// `setting/config` 的写入钩子与读锁钩子（与 relay_setting 同一范式）：
// 写侧持锁并在写完后把新值发布到 common 的原子快照，读侧只读快照 —— 消除热更新
// 与热路径读取之间的 data race。
func (c *featureSwitchConfig) BeforeConfigWrite() { c.mu.Lock() }

func (c *featureSwitchConfig) AfterConfigWrite() {
	publishLocked()
	c.mu.Unlock()
}

func (c *featureSwitchConfig) LockConfigRead()   { c.mu.RLock() }
func (c *featureSwitchConfig) UnlockConfigRead() { c.mu.RUnlock() }

// publishLocked 把当前 Values 发布为 common 的运行时覆盖快照。调用方必须持写锁。
// 未知 key（注册表里没有的）一律丢弃 —— 防止被污染的持久化配置影响任意开关。
func publishLocked() {
	snapshot := make(map[string]string, len(setting.Values))
	for key, value := range setting.Values {
		meta, ok := Meta(key)
		if !ok {
			continue
		}
		normalized, ok := normalize(meta, value)
		if !ok {
			continue
		}
		snapshot[key] = normalized
	}
	common.ReplaceFeatureFlagOverrides(snapshot)
}

// Meta 按 key 查询开关元数据。
func Meta(key string) (FeatureSwitch, bool) {
	for _, s := range registry {
		if s.Key == key {
			return s, true
		}
	}
	return FeatureSwitch{}, false
}

// normalize 把外部传入的取值归一化为合法值；不合法返回 false。
func normalize(meta FeatureSwitch, value string) (string, bool) {
	switch meta.Kind {
	case KindEnum:
		v := strings.ToLower(strings.TrimSpace(value))
		for _, opt := range meta.Options {
			if v == opt {
				return v, true
			}
		}
		return "", false
	default: // KindBool
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "on", "yes":
			return "true", true
		case "false", "0", "off", "no":
			return "false", true
		default:
			return "", false
		}
	}
}

// IsEnabled 报告某个开关**当前是否生效**（含依赖判定与运行时覆盖）。
// 未知 key 返回 false（fail-closed：不认识的开关不该被当作已启用）。
func IsEnabled(key string) bool {
	meta, ok := Meta(key)
	if !ok {
		return false
	}
	return isEnabledRec(meta, map[string]bool{})
}

func isEnabledRec(meta FeatureSwitch, visiting map[string]bool) bool {
	if visiting[meta.Key] {
		return false // 依赖成环：判定为未生效，绝不死循环
	}
	visiting[meta.Key] = true
	for _, depKey := range meta.DependsOn {
		depMeta, ok := Meta(depKey)
		if !ok || !isEnabledRec(depMeta, visiting) {
			return false
		}
	}
	return common.FeatureFlagValue(meta.Key, envDefaultBool(meta))
}

// envDefaultBool 解析开关的 env 默认值（未设置 env 时用注册表里的 Default）。
func envDefaultBool(meta FeatureSwitch) bool {
	if meta.Kind == KindEnum {
		return strings.ToLower(common.GetEnvOrDefaultString(meta.Key, meta.Default)) != "off"
	}
	return common.GetEnvOrDefaultBool(meta.Key, meta.Default == "true")
}

// CurrentValue 返回某个开关的**当前生效值**（字符串形式），用于审计记录
// 「从 X 改到 Y」中的 X。未知 key 返回 ("", false)。
func CurrentValue(key string) (string, bool) {
	meta, ok := Meta(key)
	if !ok {
		return "", false
	}
	return snapshotOf(meta).Value, true
}

// ConfiguredValue 返回管理员显式设置过的值；第二个返回值表示是否设置过。
func ConfiguredValue(key string) (string, bool) {
	setting.mu.RLock()
	defer setting.mu.RUnlock()
	v, ok := setting.Values[key]
	return v, ok
}

// Set 校验并写入一个开关，随后**立即发布**到运行时快照（热更新）。
// 返回错误时不做任何改动（调用方据此回滚前置动作）。
func Set(key, value string) error {
	meta, ok := Meta(key)
	if !ok {
		return &UnknownSwitchError{Key: key}
	}
	if !meta.AdminEditable {
		return &NotEditableError{Key: key}
	}
	normalized, ok := normalize(meta, value)
	if !ok {
		return &InvalidValueError{Key: key, Value: value, Allowed: allowedValues(meta)}
	}
	// 依赖判定：开启时所有前置必须已生效，否则拒绝（避免"打开了但静默不工作"）。
	if normalized != "false" && normalized != "off" {
		for _, depKey := range meta.DependsOn {
			if !IsEnabled(depKey) {
				return &DependencyError{Key: key, Missing: depKey}
			}
		}
	}

	setting.mu.Lock()
	next := make(map[string]string, len(setting.Values)+1)
	for k, v := range setting.Values {
		next[k] = v
	}
	next[key] = normalized
	setting.Values = next
	publishLocked()
	setting.mu.Unlock()
	return nil
}

// Reset 清除某个开关的管理员配置，使其回退 env 默认值。
func Reset(key string) error {
	if _, ok := Meta(key); !ok {
		return &UnknownSwitchError{Key: key}
	}
	setting.mu.Lock()
	next := make(map[string]string, len(setting.Values))
	for k, v := range setting.Values {
		if k == key {
			continue
		}
		next[k] = v
	}
	setting.Values = next
	publishLocked()
	setting.mu.Unlock()
	return nil
}

// SerializedValues 返回可直接落库的 `feature_switch.values` 选项值。
// 控制层用它走既有的 `model.UpdateOption` 持久化（从而复用配置框架的派发链路）。
func SerializedValues() (string, error) {
	setting.mu.RLock()
	snapshot := make(map[string]string, len(setting.Values))
	for k, v := range setting.Values {
		snapshot[k] = v
	}
	setting.mu.RUnlock()

	encoded, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// allowedValues 返回某开关的合法取值（用于错误信息与 UI 提示）。
func allowedValues(meta FeatureSwitch) []string {
	if meta.Kind == KindEnum {
		return meta.Options
	}
	return []string{"true", "false"}
}

// Snapshot 是给管理端展示的一条开关状态（只读视图）。
type Snapshot struct {
	Key             string   `json:"key"`
	Kind            string   `json:"kind"`
	Options         []string `json:"options,omitempty"`
	Value           string   `json:"value"`                // 当前生效值的字符串形式
	EnvDefault      string   `json:"env_default"`          // env 默认值
	Configured      bool     `json:"configured"`           // 管理员是否显式设置过
	Effective       bool     `json:"effective"`            // 布尔语义下是否生效（含依赖）
	TitleKey        string   `json:"title_key"`            // i18n 键
	DescriptionKey  string   `json:"description_key"`      // i18n 键
	Risk            string   `json:"risk"`                 // low | medium | high
	DependsOn       []string `json:"depends_on,omitempty"` // 前置开关
	UnsatisfiedDeps []string `json:"unsatisfied_deps,omitempty"`
	AdminEditable   bool     `json:"admin_editable"`
	RequiresRestart bool     `json:"requires_restart"`
	MetricKeys      []string `json:"metric_keys,omitempty"`
	RollbackHint    string   `json:"rollback_hint"` // i18n 键
}

// List 返回全部开关的当前状态，按 key 升序（稳定输出，便于前端 diff 与测试断言）。
func List() []Snapshot {
	out := make([]Snapshot, 0, len(registry))
	for _, meta := range registry {
		out = append(out, snapshotOf(meta))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func snapshotOf(meta FeatureSwitch) Snapshot {
	configured, hasConfigured := ConfiguredValue(meta.Key)
	envDefault := meta.Default
	if meta.Kind == KindEnum {
		envDefault = strings.ToLower(common.GetEnvOrDefaultString(meta.Key, meta.Default))
	} else if common.GetEnvOrDefaultBool(meta.Key, meta.Default == "true") {
		envDefault = "true"
	} else {
		envDefault = "false"
	}

	effective := IsEnabled(meta.Key)
	value := "false"
	if meta.Kind == KindEnum {
		value = strings.ToLower(common.FeatureFlagString(meta.Key, envDefault))
	} else if effective || common.FeatureFlagValue(meta.Key, envDefault == "true") {
		value = "true"
	}

	var unsatisfied []string
	for _, depKey := range meta.DependsOn {
		if !IsEnabled(depKey) {
			unsatisfied = append(unsatisfied, depKey)
		}
	}

	return Snapshot{
		Key:             meta.Key,
		Kind:            string(kindOf(meta)),
		Options:         meta.Options,
		Value:           value,
		EnvDefault:      envDefault,
		Configured:      hasConfigured && configured != "",
		Effective:       effective,
		TitleKey:        meta.TitleKey,
		DescriptionKey:  meta.DescriptionKey,
		Risk:            string(meta.Risk),
		DependsOn:       meta.DependsOn,
		UnsatisfiedDeps: unsatisfied,
		AdminEditable:   meta.AdminEditable,
		RequiresRestart: meta.RequiresRestart,
		MetricKeys:      meta.MetricKeys,
		RollbackHint:    meta.RollbackHint,
	}
}

func kindOf(meta FeatureSwitch) Kind {
	if meta.Kind == "" {
		return KindBool
	}
	return meta.Kind
}
