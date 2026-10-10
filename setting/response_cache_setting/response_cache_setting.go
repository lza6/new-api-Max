package response_cache_setting

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/config"
)

// 网关层响应缓存的参数配置（Batch-9 / G3）。
//
// 分工：
//   - **总开关**走能力开关注册表（`common.FlagResponseCacheEnabled`），这样它会出现在
//     管理端「实验功能」页，带风险徽章/回滚提示/效果度量；
//   - **参数**（TTL/容量/白黑名单/是否跨用户共享）放本模块，落在管理端设置里。
//
// 安全默认值（保守起步，指南 §5.2.1）：
//   - `AllowModels` 为空 = **全不缓存** —— 必须由管理员显式加白名单；
//   - `AllowCrossUser` 默认 false = **按 user 隔离**（跨用户泄漏是这项最严重的风险）；
//   - `ChargeOnHit` 默认 true = **照常计费**（不改变既有商业口径，成本节省体现在运营侧）。

// Default* 默认值。
const (
	DefaultTTLSeconds = 600   // 10 分钟
	DefaultMaxEntries = 1000  // 进程内最多 1000 条
	MaxTTLSeconds     = 86400 // 上限 24 小时（防管理员配出"永久缓存"）
	MaxMaxEntries     = 20000 // 上限 2 万条
)

// ResponseCacheSetting 响应缓存参数。
type ResponseCacheSetting struct {
	// TTLSeconds 缓存条目存活秒数；<=0 时回退 DefaultTTLSeconds。
	TTLSeconds int `json:"ttl_seconds"`
	// MaxEntries 进程内（memory 后端）最大条目数；<=0 时回退 DefaultMaxEntries。
	MaxEntries int `json:"max_entries"`
	// AllowModels 模型名 glob 白名单；**空表示全不缓存**。
	AllowModels []string `json:"allow_models"`
	// DenyModels 模型名 glob 黑名单，优先生效（先查 deny 再查 allow）。
	DenyModels []string `json:"deny_models"`
	// AllowCrossUser 为 true 时缓存键**不含 user_id**（跨用户共享）。
	// 默认 false：宁可命中率低，也不冒跨用户泄漏的风险。
	AllowCrossUser bool `json:"allow_cross_user"`
	// ChargeOnHit 命中缓存时是否照常计费。默认 true（不改变商业口径）。
	ChargeOnHit bool `json:"charge_on_hit"`
	// Backend 后端类型：memory | redis。默认 memory。
	Backend string `json:"backend"`

	// mu 保护上面的字段（热更新写入 vs 并发读取）。**不导出**，不参与
	// `setting/config` 的反射序列化。
	mu sync.RWMutex
}

// responseCacheConfig 是**无锁**的取值快照类型。
//
// 单独定义一个类型而不是直接返回 `ResponseCacheSetting`：后者含 `sync.RWMutex`，
// 拷贝/返回带锁的值会被 `go vet` 判定为 `copylocks`（真实隐患）。
type responseCacheConfig struct {
	TTLSeconds     int
	MaxEntries     int
	AllowModels    []string
	DenyModels     []string
	AllowCrossUser bool
	ChargeOnHit    bool
	Backend        string
}

var setting = ResponseCacheSetting{
	TTLSeconds:  DefaultTTLSeconds,
	MaxEntries:  DefaultMaxEntries,
	ChargeOnHit: true,
	Backend:     "memory",
}

// featureCacheEnabledDefault 总开关的 env 默认（RESPONSE_CACHE_ENABLED，默认 false）。
var featureCacheEnabledDefault = common.GetEnvOrDefaultBool(common.FlagResponseCacheEnabled, false)

func init() {
	config.GlobalConfig.Register("response_cache", &setting)
}

// BeforeConfigWrite / AfterConfigWrite / LockConfigRead / UnlockConfigRead 实现
// `setting/config` 的写入与读取钩子（与 relay_setting 同一范式）：热更新写入与并发读取
// 之间不能有 data race。
func (s *ResponseCacheSetting) BeforeConfigWrite() { s.mu.Lock() }

func (s *ResponseCacheSetting) AfterConfigWrite() { s.mu.Unlock() }

func (s *ResponseCacheSetting) LockConfigRead()   { s.mu.RLock() }
func (s *ResponseCacheSetting) UnlockConfigRead() { s.mu.RUnlock() }

// snapshot 读取当前参数的**无锁快照**（持读锁拷贝）。
func snapshot() responseCacheConfig {
	setting.mu.RLock()
	defer setting.mu.RUnlock()
	return responseCacheConfig{
		TTLSeconds:     setting.TTLSeconds,
		MaxEntries:     setting.MaxEntries,
		AllowModels:    append([]string(nil), setting.AllowModels...),
		DenyModels:     append([]string(nil), setting.DenyModels...),
		AllowCrossUser: setting.AllowCrossUser,
		ChargeOnHit:    setting.ChargeOnHit,
		Backend:        setting.Backend,
	}
}

// Enabled 报告响应缓存总开关是否生效（管理端「实验功能」页 > env，默认 false）。
func Enabled() bool {
	return common.FeatureFlagValue(common.FlagResponseCacheEnabled, featureCacheEnabledDefault)
}

// TTLSeconds 返回生效的 TTL（秒），带上限保护。
func TTLSeconds() int {
	v := snapshot().TTLSeconds
	if v <= 0 {
		return DefaultTTLSeconds
	}
	if v > MaxTTLSeconds {
		return MaxTTLSeconds
	}
	return v
}

// MaxEntries 返回生效的容量上限，带上限保护。
func MaxEntries() int {
	v := snapshot().MaxEntries
	if v <= 0 {
		return DefaultMaxEntries
	}
	if v > MaxMaxEntries {
		return MaxMaxEntries
	}
	return v
}

// AllowCrossUser 报告是否允许跨用户共享缓存（默认 false）。
func AllowCrossUser() bool { return snapshot().AllowCrossUser }

// ChargeOnHit 报告命中缓存时是否照常计费（默认 true）。
func ChargeOnHit() bool { return snapshot().ChargeOnHit }

// Backend 返回缓存后端名（normalize 后只可能是 memory / redis）。
func Backend() string {
	v := strings.ToLower(strings.TrimSpace(snapshot().Backend))
	switch v {
	case "redis":
		return "redis"
	default:
		return "memory"
	}
}

// ModelCacheable 判定某个模型是否允许缓存。
// 规则：**先查 deny 再查 allow**；allow 为空 = 全不缓存（保守起步）。
// 两者都是大小写不敏感的 glob（`*` 通配）。
func ModelCacheable(model string) bool {
	if strings.TrimSpace(model) == "" {
		return false
	}
	s := snapshot()
	for _, pattern := range s.DenyModels {
		if globMatch(pattern, model) {
			return false
		}
	}
	if len(s.AllowModels) == 0 {
		return false
	}
	for _, pattern := range s.AllowModels {
		if globMatch(pattern, model) {
			return true
		}
	}
	return false
}

// globMatch 极简 glob：只支持 `*`（匹配任意长度）与字面量，大小写不敏感。
// 刻意不引入完整 glob 库 —— 模型名匹配不需要 `?`/`[]`/`{}` 语义。
func globMatch(pattern, value string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	value = strings.ToLower(strings.TrimSpace(value))
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	parts := strings.Split(pattern, "*")
	// 无通配符：精确匹配
	if len(parts) == 1 {
		return parts[0] == value
	}
	// 前缀必须匹配
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	rest := value[len(parts[0]):]
	// 中间片段顺序匹配
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			continue
		}
		idx := strings.Index(rest, part)
		if idx < 0 {
			return false
		}
		rest = rest[idx+len(part):]
	}
	// 后缀必须匹配
	tail := parts[len(parts)-1]
	return tail == "" || strings.HasSuffix(rest, tail)
}

// cacheHits / cacheMisses / cacheStores 进程内计数（供 /metrics 与效果度量）。
var (
	cacheHits         atomic.Int64
	cacheMisses       atomic.Int64
	cacheStores       atomic.Int64
	cacheEvicts       atomic.Int64
	cacheServedTokens atomic.Int64
)

// RecordCacheHit 记录一次命中（n 为命中时回填的 token 数）。
func RecordCacheHit(tokens int64) {
	cacheHits.Add(1)
	if tokens > 0 {
		cacheServedTokens.Add(tokens)
	}
}

// RecordCacheMiss 记录一次未命中。
func RecordCacheMiss() { cacheMisses.Add(1) }

// RecordCacheStore 记录一次写入。
func RecordCacheStore() { cacheStores.Add(1) }

// RecordCacheEvict 记录一次淘汰（容量或 TTL）。
func RecordCacheEvict(n int64) { cacheEvicts.Add(n) }

// CacheStats 是给 /metrics 与「实验功能」页效果度量用的快照。
type CacheStats struct {
	Hits         int64 `json:"hits"`
	Misses       int64 `json:"misses"`
	Stores       int64 `json:"stores"`
	Evictions    int64 `json:"evictions"`
	ServedTokens int64 `json:"served_tokens"`
	LiveEntries  int   `json:"live_entries"`
}

// Stats 返回当前统计快照（LiveEntries 由调用方注入 —— 见 service/response_cache.go）。
func Stats() CacheStats {
	return CacheStats{
		Hits:         cacheHits.Load(),
		Misses:       cacheMisses.Load(),
		Stores:       cacheStores.Load(),
		Evictions:    cacheEvicts.Load(),
		ServedTokens: cacheServedTokens.Load(),
	}
}

// ResetStatsForTest 清空计数（仅测试使用）。
func ResetStatsForTest() {
	cacheHits.Store(0)
	cacheMisses.Store(0)
	cacheStores.Store(0)
	cacheEvicts.Store(0)
	cacheServedTokens.Store(0)
}

// SetForTest 直接覆盖参数（测试用；nil 字段表示不改）。
func SetForTest(ttl, maxEntries int, allow, deny []string, crossUser *bool) {
	setting.mu.Lock()
	defer setting.mu.Unlock()
	if ttl > 0 {
		setting.TTLSeconds = ttl
	}
	if maxEntries > 0 {
		setting.MaxEntries = maxEntries
	}
	if allow != nil {
		setting.AllowModels = allow
	}
	if deny != nil {
		setting.DenyModels = deny
	}
	if crossUser != nil {
		setting.AllowCrossUser = *crossUser
	}
}
