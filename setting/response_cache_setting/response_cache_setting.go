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

	// 磁盘后端默认值。
	//
	// 为什么默认走磁盘：生产是 4 核 4G / 容器 900MB 上限的机器，而磁盘有 78G 富余；
	// 缓存条目是**整个上游响应体**（可达数百 KB），放进程内是与内存抢资源，
	// 放磁盘才是「合理利用现有资源」。
	DefaultDiskMaxBytes       int64 = 512 << 20 // 缓存目录总量上限 512 MB
	MaxDiskMaxBytes           int64 = 8 << 30   // 上限 8 GB（防管理员配出"吃掉整块盘"）
	DefaultDiskMaxEntryBytes  int64 = 1 << 20   // 单条上限 1 MB
	MaxDiskMaxEntryBytes      int64 = 64 << 20  // 单条上限 64 MB
	DefaultMinFreeDiskBytes   int64 = 2 << 30   // 宿主剩余空间低于 2 GB 就停止写入
	MinMinFreeDiskBytes       int64 = 64 << 20  // 该下限最小可配 64 MB
	DefaultDiskCleanupSeconds       = 300       // 磁盘清理周期 5 分钟
	MinDiskCleanupSeconds           = 30
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
	// Backend 后端类型：memory | disk | redis。默认 **disk**（见上面的默认值说明）。
	Backend string `json:"backend"`
	// DiskDir 磁盘后端的根目录；留空用 `<工作目录>/data/response-cache`。
	DiskDir string `json:"disk_dir"`
	// DiskMaxBytes 磁盘缓存目录的**总量上限**；<=0 回退 DefaultDiskMaxBytes。
	// 超出时按「最旧优先」自动淘汰 —— 这是"不允许无限增长"的硬保证。
	DiskMaxBytes int64 `json:"disk_max_bytes"`
	// DiskMaxEntryBytes **单条**响应体上限；超过则不缓存该条（大响应落盘也不值得）。
	DiskMaxEntryBytes int64 `json:"disk_max_entry_bytes"`
	// MinFreeDiskBytes 宿主文件系统**剩余空间下限**：低于它立即停止写入并清理。
	// 这是"不要让磁盘爆满"的兜底 —— 缓存永远不能把宿主盘写满。
	MinFreeDiskBytes int64 `json:"min_free_disk_bytes"`
	// DiskCleanupSeconds 磁盘清理 loop 的周期（秒）。
	DiskCleanupSeconds int `json:"disk_cleanup_seconds"`

	// SemanticAllowModels 语义缓存白名单（模型名）。空 = 不启用语义匹配（默认）。
	// 必须显式列模型：语义命中返回的是「另一个问题的答案」，不能默认对所有模型放开。
	SemanticAllowModels []string `json:"semantic_allow_models"`
	// SemanticSimilarity 语义命中阈值（余弦相似度 0-1，默认 0.97）。
	// 阈值越低越容易命中、也越容易答非所问；超出 (0,1] 时回退默认值。
	SemanticSimilarity float64 `json:"semantic_similarity"`

	// mu 保护上面的字段（热更新写入 vs 并发读取）。**不导出**，不参与
	// `setting/config` 的反射序列化。
	mu sync.RWMutex
}

// responseCacheConfig 是**无锁**的取值快照类型。
//
// 单独定义一个类型而不是直接返回 `ResponseCacheSetting`：后者含 `sync.RWMutex`，
// 拷贝/返回带锁的值会被 `go vet` 判定为 `copylocks`（真实隐患）。
type responseCacheConfig struct {
	TTLSeconds          int
	MaxEntries          int
	AllowModels         []string
	DenyModels          []string
	AllowCrossUser      bool
	ChargeOnHit         bool
	Backend             string
	DiskDir             string
	DiskMaxBytes        int64
	DiskMaxEntryBytes   int64
	MinFreeDiskBytes    int64
	DiskCleanupSeconds  int
	SemanticAllowModels []string
	SemanticSimilarity  float64
}

var setting = ResponseCacheSetting{
	TTLSeconds:         DefaultTTLSeconds,
	MaxEntries:         DefaultMaxEntries,
	ChargeOnHit:        true,
	Backend:            "disk",
	DiskMaxBytes:       DefaultDiskMaxBytes,
	DiskMaxEntryBytes:  DefaultDiskMaxEntryBytes,
	MinFreeDiskBytes:   DefaultMinFreeDiskBytes,
	DiskCleanupSeconds: DefaultDiskCleanupSeconds,
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
		TTLSeconds:          setting.TTLSeconds,
		MaxEntries:          setting.MaxEntries,
		AllowModels:         append([]string(nil), setting.AllowModels...),
		DenyModels:          append([]string(nil), setting.DenyModels...),
		AllowCrossUser:      setting.AllowCrossUser,
		ChargeOnHit:         setting.ChargeOnHit,
		Backend:             setting.Backend,
		DiskDir:             setting.DiskDir,
		DiskMaxBytes:        setting.DiskMaxBytes,
		DiskMaxEntryBytes:   setting.DiskMaxEntryBytes,
		MinFreeDiskBytes:    setting.MinFreeDiskBytes,
		DiskCleanupSeconds:  setting.DiskCleanupSeconds,
		SemanticAllowModels: append([]string(nil), setting.SemanticAllowModels...),
		SemanticSimilarity:  setting.SemanticSimilarity,
	}
}

// DefaultSemanticSimilarity 语义缓存默认阈值（保守：宁可少命中，不可答非所问）。
const DefaultSemanticSimilarity = 0.97

// SemanticCacheActive 报告语义缓存是否应生效：精确缓存总开关 + 语义开关都开，
// 且白名单里至少有一个模型。任一条不满足即返回 false（默认全不满足）。
func SemanticCacheActive() bool {
	if !Enabled() {
		return false
	}
	if !common.FeatureFlagValue(common.FlagSemanticResponseCache, false) {
		return false
	}
	return len(snapshot().SemanticAllowModels) > 0
}

// SemanticModels 返回语义缓存白名单（副本）。
func SemanticModels() []string { return snapshot().SemanticAllowModels }

// SemanticThreshold 返回生效阈值；未配置或超出 (0,1] 时回退默认值。
func SemanticThreshold() float64 {
	v := snapshot().SemanticSimilarity
	if v <= 0 || v > 1 {
		return DefaultSemanticSimilarity
	}
	return v
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

// Backend 返回缓存后端名（normalize 后只可能是 memory / disk / redis）。
//
// 默认 **disk**：生产内存紧张（容器 900MB）而磁盘富余（78G），缓存条目又是整个
// 上游响应体（可达数百 KB）—— 放进程内是与内存抢资源。
func Backend() string {
	v := strings.ToLower(strings.TrimSpace(snapshot().Backend))
	switch v {
	case "redis":
		return "redis"
	case "memory":
		return "memory"
	default:
		return "disk"
	}
}

// DiskDir 返回磁盘缓存根目录（留空由调用方回退到默认路径）。
func DiskDir() string { return strings.TrimSpace(snapshot().DiskDir) }

// DiskMaxBytes 返回磁盘缓存**总量上限**（带上限保护，防配出"吃掉整块盘"）。
func DiskMaxBytes() int64 {
	v := snapshot().DiskMaxBytes
	if v <= 0 {
		return DefaultDiskMaxBytes
	}
	if v > MaxDiskMaxBytes {
		return MaxDiskMaxBytes
	}
	return v
}

// DiskMaxEntryBytes 返回**单条**响应体上限（超限不缓存该条）。
func DiskMaxEntryBytes() int64 {
	v := snapshot().DiskMaxEntryBytes
	if v <= 0 {
		return DefaultDiskMaxEntryBytes
	}
	if v > MaxDiskMaxEntryBytes {
		return MaxDiskMaxEntryBytes
	}
	return v
}

// MinFreeDiskBytes 返回宿主剩余空间下限（低于它停止写入）。
func MinFreeDiskBytes() int64 {
	v := snapshot().MinFreeDiskBytes
	if v <= 0 {
		return DefaultMinFreeDiskBytes
	}
	if v < MinMinFreeDiskBytes {
		return MinMinFreeDiskBytes
	}
	return v
}

// DiskCleanupSeconds 返回磁盘清理 loop 周期（秒），带下限保护。
func DiskCleanupSeconds() int {
	v := snapshot().DiskCleanupSeconds
	if v < MinDiskCleanupSeconds {
		return DefaultDiskCleanupSeconds
	}
	return v
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

// SetBackendForTest 覆盖后端类型（仅测试使用）。
//
// 之所以需要它：生产默认后端是 **disk**，而进程内 LRU 的契约测试必须显式指定
// memory —— 否则会「测错对象」（用例看起来通过，实际测的是另一条路径）。
func SetBackendForTest(backend string) {
	setting.mu.Lock()
	setting.Backend = backend
	setting.mu.Unlock()
}

// SetDiskDirForTest 覆盖磁盘目录（仅测试使用，测试应传 t.TempDir()）。
func SetDiskDirForTest(dir string) {
	setting.mu.Lock()
	setting.DiskDir = dir
	setting.mu.Unlock()
}

// SetDiskLimitsForTest 覆盖磁盘限额（仅测试使用；<=0 表示不改该项）。
func SetDiskLimitsForTest(maxBytes, maxEntryBytes, minFreeBytes int64) {
	setting.mu.Lock()
	defer setting.mu.Unlock()
	if maxBytes > 0 {
		setting.DiskMaxBytes = maxBytes
	}
	if maxEntryBytes > 0 {
		setting.DiskMaxEntryBytes = maxEntryBytes
	}
	if minFreeBytes > 0 {
		setting.MinFreeDiskBytes = minFreeBytes
	}
}
