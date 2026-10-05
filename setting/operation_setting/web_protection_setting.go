package operation_setting

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/setting/config"
)

// WebProtectionSetting Web 层防刷/限流/封禁配置（热更新，注册名 "web_protection"）。
// 仅作用于非 /v1 前缀的请求（Web 管理 API、静态资源、SPA 页面）；
// /v1 模型中继 API 完全不受影响，继续走既有 token 级限流。
type WebProtectionSetting struct {
	// Enabled 总开关（限流+日志+自动封禁）。
	Enabled bool `json:"enabled"`
	// LimitPerSecond 每 IP 每秒允许请求数（档位基准）。
	LimitPerSecond int `json:"limit_per_second"`
	// Burst 突发容忍（令牌桶容量），首屏多资源并行加载需要突发。
	Burst int `json:"burst"`
	// AutoBan 自动封禁开关。
	AutoBan bool `json:"auto_ban"`
	// AutoBanThresholdPerMinute 同一 60s 窗口内被 429 拒绝达到该次数即自动封禁。
	AutoBanThresholdPerMinute int `json:"auto_ban_threshold_per_minute"`
	// AutoBanMinutes 自动封禁时长（分钟），0 表示永久。
	AutoBanMinutes int64 `json:"auto_ban_minutes"`
	// LogEnabled Web 请求日志（按 IP 聚合）开关。
	LogEnabled bool `json:"log_enabled"`
	// WindowSeconds 聚合窗口（秒），默认 60。
	WindowSeconds int64 `json:"window_seconds"`
	// AllowedPaths 路径白名单（glob 或前缀匹配，空=全部放行）。
	AllowedPaths []string `json:"allowed_paths"`
	// BlockedPaths 路径黑名单（前缀匹配，空=不拦截）。
	BlockedPaths []string `json:"blocked_paths"`
	// UAAllowlist User-Agent 白名单（子串匹配，大小写不敏感，空=不启用）。
	UAAllowlist []string `json:"ua_allowlist"`
	// IPAllowlist 内部/信任来源 IP 或 CIDR 白名单（空=不启用）。
	// 命中白名单（或环回/链路本地/私有网段）的请求完全豁免限流与自动封禁，
	// 仅计数聚合——Web 防护只防外部恶意攻击，内网自身流量不应被误封。
	IPAllowlist []string `json:"ip_allowlist"`
}

var webProtectionSetting = WebProtectionSetting{
	Enabled:                   false,
	LimitPerSecond:            10,
	Burst:                     60,
	AutoBan:                   true,
	AutoBanThresholdPerMinute: 20,
	AutoBanMinutes:            1440,
	LogEnabled:                true,
	WindowSeconds:             60,
}

// webProtectionSettingMu 保护 webProtectionSetting 主副本（四个 []string）。
var webProtectionSettingMu sync.RWMutex

// webProtectionSettingSnapshot 已发布的不可变快照。Web 防护判定（每非 /v1 请求）
// 只读快照，避免与周期热更新（反射就地写 slice）竞争。
var webProtectionSettingSnapshot atomic.Pointer[WebProtectionSetting]

// publishWebProtectionSettingSnapshotLocked 在持 webProtectionSettingMu 前提下深拷贝
// 主副本并发布。
func publishWebProtectionSettingSnapshotLocked() {
	snap := webProtectionSetting
	snap.AllowedPaths = slices.Clone(webProtectionSetting.AllowedPaths)
	snap.BlockedPaths = slices.Clone(webProtectionSetting.BlockedPaths)
	snap.UAAllowlist = slices.Clone(webProtectionSetting.UAAllowlist)
	snap.IPAllowlist = slices.Clone(webProtectionSetting.IPAllowlist)
	webProtectionSettingSnapshot.Store(&snap)
}

// loadWebProtectionSetting 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadWebProtectionSetting() *WebProtectionSetting {
	if s := webProtectionSettingSnapshot.Load(); s != nil {
		return s
	}
	return &webProtectionSetting
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook。
func (w *WebProtectionSetting) BeforeConfigWrite() { webProtectionSettingMu.Lock() }
func (w *WebProtectionSetting) AfterConfigWrite() {
	publishWebProtectionSettingSnapshotLocked()
	webProtectionSettingMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (w *WebProtectionSetting) LockConfigRead()   { webProtectionSettingMu.RLock() }
func (w *WebProtectionSetting) UnlockConfigRead() { webProtectionSettingMu.RUnlock() }

func init() {
	config.GlobalConfig.Register("web_protection", &webProtectionSetting)
	webProtectionSettingMu.Lock()
	publishWebProtectionSettingSnapshotLocked()
	webProtectionSettingMu.Unlock()
}

// GetWebProtectionSetting 返回当前不可变快照。只读，勿直接改写返回对象。
func GetWebProtectionSetting() *WebProtectionSetting {
	return loadWebProtectionSetting()
}

// UpdateWebProtectionSetting 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateWebProtectionSetting(fn func(*WebProtectionSetting)) {
	webProtectionSettingMu.Lock()
	defer webProtectionSettingMu.Unlock()
	fn(&webProtectionSetting)
	publishWebProtectionSettingSnapshotLocked()
}

// IsWebProtectionEnabled Web 防刷总开关。
func IsWebProtectionEnabled() bool {
	return loadWebProtectionSetting().Enabled
}

// GetWebProtectionLimit 返回（每秒允许数，突发容量，聚合窗口秒数），非法值回退默认。
func GetWebProtectionLimit() (int, int, int64) {
	s := loadWebProtectionSetting()
	perSec := s.LimitPerSecond
	if perSec <= 0 {
		perSec = 10
	}
	burst := s.Burst
	if burst < perSec {
		burst = perSec
	}
	windowSec := s.WindowSeconds
	if windowSec <= 0 {
		windowSec = 60
	}
	return perSec, burst, windowSec
}

// IsAutoBanEnabled 自动封禁开关。
func IsAutoBanEnabled() bool {
	return loadWebProtectionSetting().AutoBan
}

// GetAutoBanThreshold 窗口内 429 触发自动封禁的阈值（<=0 回退 20）。
func GetAutoBanThreshold() int {
	if v := loadWebProtectionSetting().AutoBanThresholdPerMinute; v > 0 {
		return v
	}
	return 20
}

// GetAutoBanMinutes 自动封禁时长（分钟，<=0 回退 1440）。
func GetAutoBanMinutes() int64 {
	if v := loadWebProtectionSetting().AutoBanMinutes; v > 0 {
		return v
	}
	return 1440
}

// IsWebLogEnabled Web 请求日志开关。
func IsWebLogEnabled() bool {
	return loadWebProtectionSetting().LogEnabled
}

// GetWebProtectionPathPolicy 返回（允许路径, 拦截路径）策略。
// 空白条目清洗后丢弃；空列表表示对应维度不启用（非法值回退空）。
func GetWebProtectionPathPolicy() (allowed, blocked []string) {
	s := loadWebProtectionSetting()
	return cleanStringList(s.AllowedPaths), cleanStringList(s.BlockedPaths)
}

// GetWebProtectionUAAllowlist 返回 UA 白名单（子串匹配，大小写不敏感）。
// 空列表表示不启用。
func GetWebProtectionUAAllowlist() []string {
	return cleanStringList(loadWebProtectionSetting().UAAllowlist)
}

// GetWebProtectionIPAllowlist 返回内部/信任来源 IP/CIDR 白名单（空=不启用）。
func GetWebProtectionIPAllowlist() []string {
	return cleanStringList(loadWebProtectionSetting().IPAllowlist)
}

func cleanStringList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" {
			out = append(out, it)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
