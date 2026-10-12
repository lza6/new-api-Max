package service

import (
	"strings"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/operation_setting"
)

// loginLockoutMaxTrackedAccounts 失败台账按账号数封顶。
// 攻击者可以拿海量随机用户名把"失败计数"本身变成内存放大器（反向 DoS），
// 因此台账必须有界。淘汰只在台账满或周期性维护时发生，且**锁定中/仍在跟踪窗口内
// 的条目绝不淘汰**（淘汰等于解锁，会让攻击者用批量新账号把台账挤爆后自由爆破
// 老账号）。被淘汰的条目只是丢失失败计数，永远不会导致"锁死别人"。
const loginLockoutMaxTrackedAccounts = 5000

// loginLockoutCleanupInterval 过期条目维护间隔。
const loginLockoutCleanupInterval = 30 * time.Second

// loginLockoutEntry 单个账号的密码认证失败台账。
// 键为归一化后的账号（小写、去首尾空白；用户名与邮箱都走同一张表），
// 因此攻击者无法用大小写/空白变体绕过锁定。
type loginLockoutEntry struct {
	mu sync.Mutex
	// failures 是窗口内的失败时间戳（秒），最多保留 threshold 个。
	failures []int64
	// lockLevel 为已触发的锁定倍数：0=未锁定，n=第 n 次连续触发；
	// 成功后归零（见 AccountAuthSuccess）。
	lockLevel int
	// lockedUntil 为当前锁定截止时间；零值表示未锁定。
	lockedUntil time.Time
	// trackedUntil 为锁定解除后仍需保留台账的时间（= 解锁时刻 + 失败窗口）。
	// 它把「锁定期内的尝试是否延长锁定」变成一个纯粹的取舍：
	// 本实现选择**不延长**（见 recordFailure），因此这一策略必须是显式可见的。
	trackedUntil time.Time
	// lastSeen 为最近一次写入时间，仅用于过期淘汰。
	lastSeen time.Time
}

var (
	loginLockoutMu    sync.RWMutex
	loginLockoutStore = make(map[string]*loginLockoutEntry)
)

func init() {
	go loginLockoutCleanupLoop()
}

// NormalizeAccountKey 归一化账号键：密码登录按「用户名 或 邮箱」定位账号
// （model.User.ValidateAndFill），两种写法命中同一账号，因此锁定必须落在
// 同一把钥匙上。空账号返回 ""，调用方必须跳过（不参与锁定）。
func NormalizeAccountKey(account string) string {
	key := strings.ToLower(strings.TrimSpace(account))
	if len(key) > 255 {
		key = key[:255]
	}
	return key
}

// AccountLockout 是账号当前锁定状态的只读投影。
//
// 只暴露布尔值：剩余锁定时长**不得**进入任何面向调用方的响应体，
// 差异化文案会构成账号枚举（OWASP Authentication Cheat Sheet,
// Authentication and Error Messages）。剩余时长由测试钩子
// loginLockoutDurationForTest 观测，不进生产 API。
type AccountLockout struct {
	// Locked 为 true 时调用方必须拒绝该账号的密码认证。
	Locked bool
}

// AccountLockoutStatus 只读判定账号是否处于密码认证锁定期。
// 任何账号键命中即锁定——攻击者或受害者用同一账号的任一写法都会被判定到。
func AccountLockoutStatus(accounts ...string) AccountLockout {
	if !operation_setting.IsLoginLockoutEnabled() {
		return AccountLockout{}
	}
	now := time.Now()
	var status AccountLockout
	for _, account := range accounts {
		entry := lookupLoginLockoutEntry(NormalizeAccountKey(account))
		if entry == nil {
			continue
		}
		entry.mu.Lock()
		if entry.lockedUntil.After(now) {
			status.Locked = true
		}
		entry.mu.Unlock()
	}
	return status
}

// RecordPasswordAuthFailure 记录一次密码认证失败（可同时记多个账号键，
// 见 controller.Login 的 refreshLoginLockoutKeys）。
//
// 归一到账号而非来源 IP 是本机制的全部意义：OWASP Authentication Cheat Sheet
// （Account Lockout）指出按 IP 计数会被"从大量不同 IP 发起尝试"绕过。
func RecordPasswordAuthFailure(accounts ...string) {
	if !operation_setting.IsLoginLockoutEnabled() {
		return
	}
	for _, account := range accounts {
		accountLockoutRecordFailure(account)
	}
}

// accountLockoutRecordFailure 记录一次密码认证失败；达到阈值即触发（退避）锁定。
// 返回该账号在本次记录后是否处于锁定状态。
//
// 除密码登录外的其它入口（2FA/passkey/OAuth）在校验**持有物**失败时也调用本函数，
// 这样攻击者换一种因子来猜也不能获得无限尝试次数。
func accountLockoutRecordFailure(account string) bool {
	if !operation_setting.IsLoginLockoutEnabled() {
		return false
	}
	key := NormalizeAccountKey(account)
	if key == "" {
		return false
	}

	entry := getOrCreateLoginLockoutEntry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.recordFailure(time.Now())
}

// recordFailure 在持有 entry.mu 的前提下推进失败计数与锁定。
//
// 锁定期内到达的尝试**只更新 lastSeen，不推进计数、不延长锁定**。
// 这是本次设计的核心 DoS 取舍：若锁定期内继续计数，攻击者只要在锁定期间
// 持续提交错误密码，就能把受害者永久锁在门外（锁定变成攻击面）；而"不延长"
// 的代价只是攻击者在锁定期里做的尝试不额外累积——反正这些尝试本来就被拒绝。
//
// 锁定解除后，之前触发锁定的失败已清空（见下），因此下一次触发要重新累积
// 完整的 threshold 次失败，而 lockLevel 保留 → 退避级别继续上升但有硬上限。
func (entry *loginLockoutEntry) recordFailure(now time.Time) bool {
	entry.lastSeen = now
	if entry.lockedUntil.After(now) {
		return true
	}

	threshold := operation_setting.GetLoginLockoutThreshold()
	window := operation_setting.GetLoginLockoutWindowSeconds()
	nowUnix := now.Unix()

	// 丢弃窗口外的历史失败。
	cutoff := nowUnix - window
	kept := entry.failures[:0]
	for _, at := range entry.failures {
		if at > cutoff {
			kept = append(kept, at)
		}
	}
	entry.failures = append(kept, nowUnix)
	if len(entry.failures) > threshold {
		entry.failures = entry.failures[len(entry.failures)-threshold:]
	}
	if len(entry.failures) < threshold {
		return false
	}

	// 触发锁定：退避为 base * 2^(level-1)，封顶 maxLockSeconds（硬上限）。
	entry.failures = entry.failures[:0]
	if entry.lockLevel < 30 {
		entry.lockLevel++
	}
	duration := loginLockoutBackoff(entry.lockLevel)
	entry.lockedUntil = now.Add(duration)
	entry.trackedUntil = entry.lockedUntil.Add(time.Duration(window) * time.Second)
	// §4.1.4 指标：账号级锁定触发计数（不含账号名，避免把标识写进标签空间）。
	common.MetricsInc("login_lockout_triggered_total", nil, 1)
	return true
}

// loginLockoutBackoff 返回第 level 次触发的锁定时长。
// 上限（MaxLockSeconds）是「锁定不可被滥用为永久 DoS」的硬边界：
// 受害者用正确密码等一个有限窗口即可自解，无需管理员介入。
func loginLockoutBackoff(level int) time.Duration {
	base := operation_setting.GetLoginLockoutBaseLockSeconds()
	maximum := operation_setting.GetLoginLockoutMaxLockSeconds()
	seconds := base
	for i := 1; i < level; i++ {
		seconds *= 2
		if seconds >= maximum {
			return time.Duration(maximum) * time.Second
		}
	}
	if seconds > maximum {
		seconds = maximum
	}
	return time.Duration(seconds) * time.Second
}

// RecordPasswordAuthSuccess 在认证成功后清零失败台账。
//
// 这是「攻击者无法锁死他人账号」的核心保证：受害者一旦完成一次成功登录，
// 计数、锁定与退避级别立即全部作废——无需管理员介入、无需等待。
// 因此针对某个账号的恶意锁定最多只能维持一个退避上限（默认 15 分钟）。
//
// 已接线的调用点（`controller/user.go` 的 clearAccountLockout）：
// 密码登录主认证通过后（setupLogin）、以及 2FA/passkey 二次验证通过后
// （completeVerifiedLoginResponse）。**未接线**：直接持有 PAT / access_token /
// 会话 cookie 的请求路径——这类请求不经过登录入口，无法在此处清零。
// 那部分用户若被恶意锁定，需等一个退避上限后自解（有界，不会永久失联）。
func RecordPasswordAuthSuccess(account string) {
	key := NormalizeAccountKey(account)
	if key == "" {
		return
	}
	loginLockoutMu.Lock()
	delete(loginLockoutStore, key)
	loginLockoutMu.Unlock()
}

// loginLockoutTidy 清除已解锁、失败窗口已过、且久未写入的条目。
// 锁定中（或仍在 trackedUntil 内）的条目绝不淘汰——淘汰等于解锁并清空退避级别，
// 会让攻击者用批量新账号把台账挤爆后自由爆破老账号。
func loginLockoutTidy(now time.Time) {
	window := operation_setting.GetLoginLockoutWindowSeconds()
	loginLockoutMu.Lock()
	defer loginLockoutMu.Unlock()
	for key, entry := range loginLockoutStore {
		entry.mu.Lock()
		keep := loginLockoutEntryIsLiveLocked(entry, now, window)
		entry.mu.Unlock()
		if keep {
			continue
		}
		delete(loginLockoutStore, key)
	}
}

// loginLockoutEntryIsLiveLocked 调用方必须持有 entry.mu。
func loginLockoutEntryIsLiveLocked(entry *loginLockoutEntry, now time.Time, windowSeconds int64) bool {
	if entry.lockedUntil.After(now) {
		return true
	}
	if !entry.trackedUntil.IsZero() && entry.trackedUntil.After(now) {
		return true
	}
	return now.Sub(entry.lastSeen) <= time.Duration(windowSeconds)*time.Second
}

func loginLockoutCleanupLoop() {
	ticker := time.NewTicker(loginLockoutCleanupInterval)
	defer ticker.Stop()
	for now := range ticker.C {
		loginLockoutTidy(now)
	}
}

// lookupLoginLockoutEntry 只读查找（不创建），供放行判定使用。
func lookupLoginLockoutEntry(key string) *loginLockoutEntry {
	if key == "" {
		return nil
	}
	loginLockoutMu.RLock()
	defer loginLockoutMu.RUnlock()
	return loginLockoutStore[key]
}

// getOrCreateLoginLockoutEntry 取或建条目；台账满时先清理过期条目，
// 仍满则拒绝创建新条目（返回一个不落库的临时条目，使本次失败无副作用而不是
// 撑爆内存）。返回的条目必然非 nil。
func getOrCreateLoginLockoutEntry(key string) *loginLockoutEntry {
	loginLockoutMu.Lock()
	defer loginLockoutMu.Unlock()
	if entry, ok := loginLockoutStore[key]; ok {
		return entry
	}
	if len(loginLockoutStore) >= loginLockoutMaxTrackedAccounts {
		dropExpiredLoginLockoutEntriesLocked(time.Now())
	}
	if len(loginLockoutStore) >= loginLockoutMaxTrackedAccounts {
		return &loginLockoutEntry{}
	}
	entry := &loginLockoutEntry{lastSeen: time.Now()}
	loginLockoutStore[key] = entry
	return entry
}

// dropExpiredLoginLockoutEntriesLocked 调用方必须持有 loginLockoutMu 写锁。
func dropExpiredLoginLockoutEntriesLocked(now time.Time) {
	window := operation_setting.GetLoginLockoutWindowSeconds()
	for key, entry := range loginLockoutStore {
		entry.mu.Lock()
		keep := loginLockoutEntryIsLiveLocked(entry, now, window)
		entry.mu.Unlock()
		if keep {
			continue
		}
		delete(loginLockoutStore, key)
	}
}

// --- 仅供测试的观测/构造钩子（避免用例等待真实退避时长） ---

func loginLockoutEntryForTest(account string) *loginLockoutEntry {
	return lookupLoginLockoutEntry(NormalizeAccountKey(account))
}

func loginLockoutTrackedAccountsForTest() int {
	loginLockoutMu.RLock()
	defer loginLockoutMu.RUnlock()
	return len(loginLockoutStore)
}

// loginLockoutDurationForTest 返回该账号剩余锁定时长（未锁定为 0），
// 使用例以「相对时长」而非绝对时间戳断言，避免墙钟推进造成的伪差异。
func loginLockoutDurationForTest(account string) time.Duration {
	entry := lookupLoginLockoutEntry(NormalizeAccountKey(account))
	if entry == nil {
		return 0
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if !entry.lockedUntil.After(time.Now()) {
		return 0
	}
	return entry.lockedUntil.Sub(time.Now())
}

// shiftLoginLockoutEntryForTest 把某账号的失败时间戳整体平移（模拟窗口流逝），
// 使用例无需真实等待。
func shiftLoginLockoutEntryForTest(account string, delta time.Duration) {
	entry := lookupLoginLockoutEntry(NormalizeAccountKey(account))
	if entry == nil {
		return
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	seconds := int64(delta / time.Second)
	for i := range entry.failures {
		entry.failures[i] += seconds
	}
}

// expireLoginLockoutForTest 把某账号的锁定与失败时间戳整体回拨，
// 模拟已等待足够久；使用例不必真实 sleep。
func expireLoginLockoutForTest(account string) {
	entry := lookupLoginLockoutEntry(NormalizeAccountKey(account))
	if entry == nil {
		return
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	past := time.Now().Add(-24 * time.Hour)
	if entry.lockedUntil.After(past) {
		entry.lockedUntil = past
	}
	if entry.trackedUntil.After(past) {
		entry.trackedUntil = past
	}
	entry.failures = entry.failures[:0]
	entry.lastSeen = time.Now()
}

func resetLoginLockoutTrackersForTest() {
	loginLockoutMu.Lock()
	defer loginLockoutMu.Unlock()
	loginLockoutStore = make(map[string]*loginLockoutEntry)
}
