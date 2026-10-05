package system_setting

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/setting/config"
)

type FetchSetting struct {
	EnableSSRFProtection   bool     `json:"enable_ssrf_protection"` // 是否启用SSRF防护
	AllowPrivateIp         bool     `json:"allow_private_ip"`
	DomainFilterMode       bool     `json:"domain_filter_mode"`         // 域名过滤模式，true: 白名单模式，false: 黑名单模式
	IpFilterMode           bool     `json:"ip_filter_mode"`             // IP过滤模式，true: 白名单模式，false: 黑名单模式
	DomainList             []string `json:"domain_list"`                // domain format, e.g. example.com, *.example.com
	IpList                 []string `json:"ip_list"`                    // CIDR format
	AllowedPorts           []string `json:"allowed_ports"`              // port range format, e.g. 80, 443, 8000-9000
	ApplyIPFilterForDomain bool     `json:"apply_ip_filter_for_domain"` // 对域名启用IP过滤（实验性）
}

var defaultFetchSetting = FetchSetting{
	EnableSSRFProtection:   true, // 默认开启SSRF防护
	AllowPrivateIp:         false,
	DomainFilterMode:       false,
	IpFilterMode:           false,
	DomainList:             []string{},
	IpList:                 []string{},
	AllowedPorts:           []string{"80", "443", "8080", "8443"},
	ApplyIPFilterForDomain: true,
}

// fetchSettingMu 保护 defaultFetchSetting 主副本（三个 []string）。
var fetchSettingMu sync.RWMutex

// fetchSettingSnapshot 已发布的不可变快照。SSRF 防护判定（每次外部 fetch）只读快照，
// 避免与周期热更新（反射就地写 slice）竞争。
var fetchSettingSnapshot atomic.Pointer[FetchSetting]

// publishFetchSettingSnapshotLocked 在持 fetchSettingMu 前提下深拷贝主副本并发布。
func publishFetchSettingSnapshotLocked() {
	snap := defaultFetchSetting
	snap.DomainList = slices.Clone(defaultFetchSetting.DomainList)
	snap.IpList = slices.Clone(defaultFetchSetting.IpList)
	snap.AllowedPorts = slices.Clone(defaultFetchSetting.AllowedPorts)
	fetchSettingSnapshot.Store(&snap)
}

// loadFetchSetting 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadFetchSetting() *FetchSetting {
	if s := fetchSettingSnapshot.Load(); s != nil {
		return s
	}
	return &defaultFetchSetting
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook。
func (f *FetchSetting) BeforeConfigWrite() { fetchSettingMu.Lock() }
func (f *FetchSetting) AfterConfigWrite() {
	publishFetchSettingSnapshotLocked()
	fetchSettingMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (f *FetchSetting) LockConfigRead()   { fetchSettingMu.RLock() }
func (f *FetchSetting) UnlockConfigRead() { fetchSettingMu.RUnlock() }

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("fetch_setting", &defaultFetchSetting)
	fetchSettingMu.Lock()
	publishFetchSettingSnapshotLocked()
	fetchSettingMu.Unlock()
}

// GetFetchSetting 返回当前不可变快照。只读，勿直接改写返回对象。
func GetFetchSetting() *FetchSetting {
	return loadFetchSetting()
}

// UpdateFetchSetting 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateFetchSetting(fn func(*FetchSetting)) {
	fetchSettingMu.Lock()
	defer fetchSettingMu.Unlock()
	fn(&defaultFetchSetting)
	publishFetchSettingSnapshotLocked()
}
