package service

import (
	"testing"
	"time"

	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 账号级登录失败锁定（OWASP Authentication Cheat Sheet — Account Lockout：
// 「The counter of failed logins should be associated with the account itself,
// rather than the source IP address」）。
//
// 本文件的用例保护三条真实契约：
//  1. 达到阈值后该账号被锁；锁定有上限、且逐次退避（不上升为永久 DoS）。
//  2. 成功登录清零计数并解除锁定（受害者可自解，不依赖管理员）。
//  3. 锁定期内的尝试既不累加也不延长锁定——锁定本身不能被反用成攻击面。

// lockoutAllowed 是测试侧的可读谓词；生产代码读 AccountLockoutStatus。
func lockoutAllowed(account string) bool {
	return !AccountLockoutStatus(account).Locked
}

// useLoginLockoutPolicy 覆盖策略参数，用例结束后恢复。
func useLoginLockoutPolicy(t *testing.T, apply func(*operation_setting.LoginLockoutSetting)) {
	t.Helper()
	original := *operation_setting.GetLoginLockoutSetting()
	t.Cleanup(func() {
		*operation_setting.GetLoginLockoutSetting() = original
	})
	apply(operation_setting.GetLoginLockoutSetting())
	resetLoginLockoutTrackersForTest()
}

func TestLoginLockoutLocksAccountAfterThreshold(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 3
		s.WindowSeconds = 600
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 120
	})

	const account = "lockout_threshold_user"
	assert.True(t, lockoutAllowed(account), "未触发阈值前必须放行")

	for attempt := 1; attempt <= 3; attempt++ {
		accountLockoutRecordFailure(account)
		if attempt < 3 {
			assert.True(t, lockoutAllowed(account), "第 %d 次失败尚未达阈值，必须放行", attempt)
			continue
		}
		assert.False(t, lockoutAllowed(account), "达到阈值后必须锁定该账号")
	}
}

// 退避必须增长且有上限。上限是「锁定不可被用作永久 DoS」的硬边界。
func TestLoginLockoutBackoffGrowsAndIsBounded(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 2
		s.WindowSeconds = 600
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 240
	})

	const account = "lockout_backoff_user"
	// 第 1 轮：2 次失败 → 锁 60s。
	accountLockoutRecordFailure(account)
	accountLockoutRecordFailure(account)
	require.False(t, lockoutAllowed(account))
	require.NotNil(t, loginLockoutEntryForTest(account), "锁定状态必须可观测（供运维/测试）")
	assert.Equal(t, 1, loginLockoutEntryForTest(account).lockLevel, "首次触发为第 1 级")
	firstLock := loginLockoutDurationForTest(account)
	assert.Greater(t, firstLock, 50*time.Second, "首次锁定约为基数 60s")
	assert.LessOrEqual(t, firstLock, 60*time.Second)

	// 第 2 轮：第 1 轮锁定到期后再失败 2 次 → 锁 120s（指数）。
	expireLoginLockoutForTest(account)
	require.True(t, lockoutAllowed(account), "前置条件：上一轮锁定已到期")
	for range 2 {
		accountLockoutRecordFailure(account)
	}
	assert.Equal(t, 2, loginLockoutEntryForTest(account).lockLevel)
	secondLock := loginLockoutDurationForTest(account)
	assert.Greater(t, secondLock, firstLock, "连续触发必须退避增长")
	assert.LessOrEqual(t, secondLock, 120*time.Second)

	// 第 3 轮：再失败 2 次 → 仍不得超过上限 240s。
	expireLoginLockoutForTest(account)
	for range 2 {
		accountLockoutRecordFailure(account)
	}
	assert.Equal(t, 3, loginLockoutEntryForTest(account).lockLevel)
	thirdLock := loginLockoutDurationForTest(account)
	assert.Greater(t, thirdLock, secondLock)
	assert.LessOrEqual(t, thirdLock, 240*time.Second, "退避不得突破上限（否则可退化为永久 DoS）")

	// 继续触发必须被硬上限钳住（这是「锁定不可被滥用为永久 DoS」的直接证据）。
	for range 6 {
		expireLoginLockoutForTest(account)
		accountLockoutRecordFailure(account)
		accountLockoutRecordFailure(account)
	}
	assert.Equal(t, 240*time.Second, loginLockoutDurationForTest(account),
		"无论触发多少次，锁定都被钳在上限")
	assert.Greater(t, loginLockoutEntryForTest(account).lockLevel, 3,
		"退避级别持续上升，只是时长被封顶")
}

// 锁定期内的尝试**不得**延长锁定——这是本设计最重要的反 DoS 不变量。
// 若延长成立，攻击者只需在锁定期间持续提交错误密码即可把受害者永久锁在门外，
// 锁定本身就成了攻击面（OWASP Authentication Cheat Sheet 明确警告 lockout
// 可被滥用为 DoS）。
func TestLoginLockoutAttemptsDuringLockDoNotExtendLock(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 2
		s.WindowSeconds = 600
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 240
	})

	const account = "lockout_extend_probe"
	accountLockoutRecordFailure(account)
	accountLockoutRecordFailure(account)
	before := loginLockoutDurationForTest(account)
	require.Greater(t, before, 50*time.Second)

	// 锁定期内狂打（远超阈值次数）。
	for range 50 {
		assert.True(t, accountLockoutRecordFailure(account), "锁定期内的尝试应被拒绝（返回已锁定）")
	}

	after := loginLockoutDurationForTest(account)
	assert.LessOrEqual(t, after, before,
		"锁定期内的尝试绝不能延长锁定（否则可被滥用为对账号的永久 DoS）")
	assert.Equal(t, 1, loginLockoutEntryForTest(account).lockLevel,
		"锁定期内的尝试不得推进退避级别")
}

// 成功登录必须清零计数并解锁：受害者用正确密码即可立即恢复，
// 不需要管理员介入——这是「攻击者无法永久锁死他人账号」的关键保证。
func TestLoginLockoutSuccessClearsLockAndCounter(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 3
		s.WindowSeconds = 600
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 120
	})

	const account = "lockout_recovery_user"
	for range 3 {
		accountLockoutRecordFailure(account)
	}
	require.False(t, lockoutAllowed(account), "前置条件：账号已被锁定")

	// 密码登录成功路径（controller.Login 在 ValidateAndFill 成功后调用）。
	RecordPasswordAuthSuccess(account)

	assert.True(t, lockoutAllowed(account), "成功认证必须立即解除锁定")
	assert.Nil(t, loginLockoutEntryForTest(account), "成功认证必须清空失败台账")
}

// 失败计数有窗口：窗口外的旧失败不得把账号锁死。
func TestLoginLockoutFailuresExpireWithWindow(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 3
		s.WindowSeconds = 1
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 120
	})

	const account = "lockout_window_user"
	accountLockoutRecordFailure(account)
	accountLockoutRecordFailure(account)
	// 模拟窗口已过（把已有失败记录的时间整体回拨）。
	shiftLoginLockoutEntryForTest(account, -2*time.Second)

	accountLockoutRecordFailure(account)
	assert.True(t, lockoutAllowed(account),
		"窗口外的历史失败不得计入，账号不得被锁")
	entry := loginLockoutEntryForTest(account)
	require.NotNil(t, entry, "窗口外的失败必须被丢弃，只保留窗口内的一次")
	assert.Len(t, entry.failures, 1, "台账只保留窗口内的失败时间戳")
}

// 停用开关后必须立即恢复放行——"绝不能锁死他人账号"的最后一道保险。
func TestLoginLockoutDisabledUnblocksImmediately(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 2
		s.WindowSeconds = 600
		s.BaseLockSeconds = 300
		s.MaxLockSeconds = 900
	})

	const account = "lockout_disabled_user"
	accountLockoutRecordFailure(account)
	accountLockoutRecordFailure(account)
	require.False(t, lockoutAllowed(account))

	operation_setting.GetLoginLockoutSetting().Enabled = false

	assert.True(t, lockoutAllowed(account),
		"关闭开关后必须立即放行（管理员恢复被锁账号的运维手段）")
}

// 计数键大小写/空白不敏感：同一账号换写法不能绕过锁定。
func TestLoginLockoutIsCaseAndSpaceInsensitive(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 2
		s.WindowSeconds = 600
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 120
	})

	accountLockoutRecordFailure("Lockout.Mixed@Example.com")
	accountLockoutRecordFailure("  lockout.mixed@example.com  ")

	assert.False(t, lockoutAllowed("LOCKOUT.MIXED@EXAMPLE.COM"),
		"同一账号的写法差异不得用来绕过锁定")
}

// 不同账号互不影响（避免锁定"外溢"到无辜账号）。
func TestLoginLockoutIsPerAccount(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 2
		s.WindowSeconds = 600
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 120
	})

	accountLockoutRecordFailure("lockout_victim")
	accountLockoutRecordFailure("lockout_victim")

	assert.False(t, lockoutAllowed("lockout_victim"))
	assert.True(t, lockoutAllowed("lockout_bystander"),
		"锁定必须只作用于目标账号，不得影响其它账号")
}

// 空账号名不得进入台账（避免匿名/无用户名请求把全局键填满）。
func TestLoginLockoutIgnoresEmptyAccount(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 1
		s.WindowSeconds = 600
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 120
	})

	accountLockoutRecordFailure("   ")
	assert.True(t, lockoutAllowed(""), "空账号不参与锁定")
	assert.Nil(t, loginLockoutEntryForTest(""))
}

// 台账必须有界：攻击者用海量随机用户名打不满内存（DoS 反向防护）。
func TestLoginLockoutStoreIsBounded(t *testing.T) {
	useLoginLockoutPolicy(t, func(s *operation_setting.LoginLockoutSetting) {
		s.Threshold = 1000 // 不触发锁定，只压入台账
		s.WindowSeconds = 600
		s.BaseLockSeconds = 60
		s.MaxLockSeconds = 120
	})

	for i := range loginLockoutMaxTrackedAccounts + 50 {
		accountLockoutRecordFailure("bulk_user_" + itoaForTest(i))
	}
	assert.LessOrEqual(t, loginLockoutTrackedAccountsForTest(), loginLockoutMaxTrackedAccounts,
		"失败台账必须按账号数封顶，避免无界内存增长")
}
