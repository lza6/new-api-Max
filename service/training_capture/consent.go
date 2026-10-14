package training_capture

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// 授权状态与总开关。
//
// ## 为什么授权判定必须**服务端强制**（宪法 III）
//
// 采集是不可逆的（数据一旦上传就离开本站）。前端的开关只是 UI；
// 真正的判据必须在投递点由服务端读 DB 决定——否则任何人构造一个请求
// 就能让未授权的数据被采集。
//
// ## 为什么要有缓存
//
// 投递发生在**请求路径附近**，不能每请求查一次 DB（宪法 V 禁热路径 DB 操作）。
// 因此用 TTL 缓存授权状态。
//
// 缓存带来的风险是「撤回后仍被采集一小段时间」。为此：
//   - TTL 取**很短**（默认 30 秒）；
//   - **撤回时主动失效缓存**（不依赖 TTL 过期），使撤回立即生效。
//
// 这个组合让「撤回立即停止采集」在正常路径上成立，同时不牺牲热路径性能。

const (
	// consentCacheTTL 授权状态缓存时长。
	//
	// 短 TTL 是刻意的：它是「撤回后仍可能被采集」的最坏窗口。
	// 30 秒 × 单用户请求量，最坏情况多采几十条——可接受，
	// 而撤回路径本身会主动失效缓存，正常情况窗口接近 0。
	consentCacheTTL = 30 * time.Second

	// consentCacheMaxEntries 缓存条目上限（防内存无界增长）。
	consentCacheMaxEntries = 20_000

	// CurrentPolicyVersion 当前免责声明版本。
	//
	// **声明内容实质性变更时必须递增它**，使旧同意失效并重新征求同意。
	// 见 `model.ListGrantedTrainingDataUserIds` 的版本过滤。
	CurrentPolicyVersion = "v1"
)

// ErrGatewayDisabled 管理员关闭了采集总开关。
var ErrGatewayDisabled = errors.New("training data capture: disabled by administrator")

// consentEntry 一条缓存的授权状态。
type consentEntry struct {
	granted bool
	// policyVersion 该用户同意时的声明版本；与当前版本不符时视为未授权。
	policyVersion string
	expiresAt     time.Time
}

var (
	consentCacheMu sync.RWMutex
	consentCache   = map[int]consentEntry{}

	// gatewayEnabled 管理员总开关（默认 **true**——用户要求「默认打开」）。
	//
	// 但**能被打开的前提是云盘已配置**（见 `CanEnableGateway`）：
	// 没配云盘就「开启采集」只会让数据堆在内存然后被丢弃，
	// 那比明确关闭更糟（管理员以为在采集）。
	gatewayEnabled atomic.Bool
)

func init() {
	gatewayEnabled.Store(true)
}

// ---------- 总开关 ----------

// IsGatewayEnabled 报告管理员总开关状态。
func IsGatewayEnabled() bool {
	return gatewayEnabled.Load()
}

// SetGatewayEnabled 设置总开关并清空授权缓存。
//
// 清缓存是必要的：开关状态参与 `IsGranted` 的判定，
// 不清缓存会让「刚关闭」在 TTL 内仍被判为可采集。
func SetGatewayEnabled(enabled bool) {
	gatewayEnabled.Store(enabled)
	invalidateConsentCache()
}

// SetGatewayEnabledForTest 测试用覆盖，返回恢复函数。
func SetGatewayEnabledForTest(enabled bool) func() {
	previous := gatewayEnabled.Load()
	gatewayEnabled.Store(enabled)
	invalidateConsentCache()
	return func() {
		gatewayEnabled.Store(previous)
		invalidateConsentCache()
	}
}

// CanEnableGateway 报告总开关**是否允许被打开**。
//
// 条件是云盘已配置：未配置时打开采集没有意义（数据无处可去）。
// 管理端据此禁用开关并显示原因，而不是允许打开后静默丢数据。
func CanEnableGateway() bool {
	return IsSinkConfigured()
}

// GatewayStatus 供管理端展示的总开关状态（含能否开启与原因）。
type GatewayStatus struct {
	Enabled bool `json:"enabled"`
	// CanEnable 为 false 时 Enabled 必然为 false；前端据此禁用开关。
	CanEnable bool `json:"can_enable"`
	// Reason 说明为何不能开启（云盘未配置等）；为空表示无限制。
	Reason string `json:"reason,omitempty"`
	// SinkName 当前生效的存储后端名。
	SinkName string `json:"sink_name"`
}

// GetGatewayStatus 组装总开关状态。
func GetGatewayStatus() GatewayStatus {
	status := GatewayStatus{
		Enabled:   IsGatewayEnabled(),
		CanEnable: CanEnableGateway(),
		SinkName:  ActiveSink().Name(),
	}
	if !status.CanEnable {
		status.Reason = "尚未配置云盘存储后端（缺少凭据），无法启用数据采集"
		// 云盘不可用时，即使 DB 里开着也对外报告为「不可用」——
		// 否则用户会看到开关是开的却没有任何数据被采集。
		status.Enabled = false
	}
	return status
}

// ---------- 授权判定 ----------

// IsGranted 报告某用户**此刻**是否应被采集。
//
// 这是投递路径上的唯一判据，必须快（命中缓存时是一次 map 读 + 时间比较）。
//
// 判定顺序（任一不满足即不采集）：
//  1. 管理员总开关开
//  2. 云盘已配置
//  3. 用户已授权
//  4. 用户同意的声明版本 == 当前版本
func IsGranted(userId int) bool {
	if userId <= 0 {
		return false
	}
	if !IsGatewayEnabled() || !CanEnableGateway() {
		return false
	}

	now := time.Now()
	consentCacheMu.RLock()
	entry, ok := consentCache[userId]
	consentCacheMu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.granted && entry.policyVersion == CurrentPolicyVersion
	}

	return refreshConsentCache(userId, now)
}

// refreshConsentCache 回源 DB 并写缓存。
func refreshConsentCache(userId int, now time.Time) bool {
	consent, err := model.GetTrainingDataConsent(userId)
	if err != nil {
		// 查库失败：**保守判定为未授权**（不采集）。
		// 反过来（失败时默认采集）会让未授权用户的数据被采——不可接受。
		common.SysError("training capture: consent lookup failed: " + err.Error())
		return false
	}

	entry := consentEntry{expiresAt: now.Add(consentCacheTTL)}
	if consent != nil {
		entry.granted = consent.Granted
		entry.policyVersion = consent.PolicyVersion
	}
	storeConsentCache(userId, entry)
	return entry.granted && entry.policyVersion == CurrentPolicyVersion
}

func storeConsentCache(userId int, entry consentEntry) {
	consentCacheMu.Lock()
	defer consentCacheMu.Unlock()
	// 容量保护：满了就整体清空（简单且不会泄漏）。
	// 不用 LRU：授权状态的访问分布没有局部性（用户随时可能来），
	// 而清空的代价只是一次回源。
	if len(consentCache) >= consentCacheMaxEntries {
		consentCache = make(map[int]consentEntry, 1024)
	}
	consentCache[userId] = entry
}

// invalidateConsentCache 清空整个授权缓存。
func invalidateConsentCache() {
	consentCacheMu.Lock()
	consentCache = make(map[int]consentEntry, 1024)
	consentCacheMu.Unlock()
}

// InvalidateConsent 让某用户的授权缓存立即失效。
//
// **撤回时必须调用**：不能等 TTL 过期，否则「我刚关掉它还在传」是可观测的
// 隐私问题（用户点了撤回却看到数据继续上传，会直接失去信任）。
func InvalidateConsent(userId int) {
	consentCacheMu.Lock()
	delete(consentCache, userId)
	consentCacheMu.Unlock()
}

// ResetConsentCacheForTest 清空缓存（仅测试用）。
func ResetConsentCacheForTest() {
	invalidateConsentCache()
}

// ---------- 授权操作 ----------

// GrantConsent 记录用户同意（服务端强制，不信前端）。
//
// 返回错误时**不改动任何状态**。
func GrantConsent(userId int, policyVersion string) error {
	if userId <= 0 {
		return errors.New("training capture: invalid user id")
	}
	version := strings.TrimSpace(policyVersion)
	if version == "" {
		version = CurrentPolicyVersion
	}
	if err := model.UpsertTrainingDataConsent(userId, true, version, time.Now().Unix()); err != nil {
		return err
	}
	InvalidateConsent(userId)
	return nil
}

// RevokeConsent 撤回授权。
//
// 语义（**必须对用户诚实说明**，见 spec Clarifications Q10）：
// 撤回 = **停止后续采集**，**不等于**删除已上传到云盘的历史数据。
// 直接在 UI 暗示「撤回即删除」是合规风险，故文案必须区分这两件事。
func RevokeConsent(userId int) error {
	if userId <= 0 {
		return errors.New("training capture: invalid user id")
	}
	if err := model.UpsertTrainingDataConsent(userId, false, CurrentPolicyVersion, time.Now().Unix()); err != nil {
		return err
	}
	InvalidateConsent(userId)
	return nil
}

// ConsentStatus 供用户端展示的授权状态。
type ConsentStatus struct {
	// Available 为 false 时前端**不渲染开关**（用户明确要求：
	// 管理员关闭后，用户侧不应看到点了无效的开关）。
	Available bool `json:"available"`
	// Reason 说明为何不可用；为空表示可用。
	Reason string `json:"reason,omitempty"`
	// Granted 当前是否已授权且声明版本匹配。
	Granted bool `json:"granted"`
	// PolicyVersion 当前声明版本（前端用于判断是否需要重新同意）。
	PolicyVersion string `json:"policy_version"`
	// GrantedAt 最近一次同意时间（unix 秒）；未同意为 0。
	GrantedAt int64 `json:"granted_at"`
}

// GetConsentStatus 读取用户授权状态（供接口层）。
//
// 与 `IsGranted` 的区别：本函数**回源 DB**（用户主动查询，不在热路径），
// 因此结果精确、不受缓存影响。
func GetConsentStatus(userId int) (ConsentStatus, error) {
	status := ConsentStatus{PolicyVersion: CurrentPolicyVersion}

	if !CanEnableGateway() {
		// 云盘未配置 → 整体不可用（用户侧连开关都不该看到）。
		status.Reason = "数据采集功能当前未开放"
		return status, nil
	}
	if !IsGatewayEnabled() {
		// 管理员关闭 → 用户侧隐藏。
		status.Available = false
		status.Reason = "数据采集功能当前已关闭"
		return status, nil
	}
	status.Available = true

	consent, err := model.GetTrainingDataConsent(userId)
	if err != nil {
		return status, err
	}
	if consent != nil {
		status.GrantedAt = consent.GrantedAt
		// 只有「已授权」**且**「同意的是当前版本」才算真正授权。
		status.Granted = consent.Granted && consent.PolicyVersion == CurrentPolicyVersion
	}
	return status, nil
}
