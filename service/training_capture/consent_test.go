package training_capture

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/model"
)

// 授权判定是**合规边界**：判错就是未授权用户的数据被采集。
// 因此每个「不采集」的条件都要有独立用例。

func TestIsGrantedRequiresPositiveUserId(t *testing.T) {
	defer SetSinkForTest(fakeSink{name: "t"})()
	defer SetGatewayEnabledForTest(true)()
	ResetConsentCacheForTest()

	// 0 / 负数用户 ID 无意义，必须判为不可采集。
	assert.False(t, IsGranted(0))
	assert.False(t, IsGranted(-1))
}

func TestIsGrantedFalseWhenGatewayDisabled(t *testing.T) {
	// 管理员关总开关 → 一律不采集（即使云盘配好、用户已授权）。
	defer SetSinkForTest(fakeSink{name: "t"})()
	defer SetGatewayEnabledForTest(false)()
	ResetConsentCacheForTest()

	assert.False(t, IsGranted(1), "总开关关闭时任何人都不得被采集")
}

func TestIsGrantedFalseWhenSinkNotConfigured(t *testing.T) {
	// 云盘未配置 → 不采集（数据无处可去，采了只能丢）。
	defer SetSinkForTest(nil)()
	defer SetGatewayEnabledForTest(true)()
	ResetConsentCacheForTest()

	assert.False(t, IsGranted(1), "云盘未配置时不得采集")
}

func TestGatewayStatusReportsDisabledWhenSinkMissing(t *testing.T) {
	// 关键行为：云盘不可用时，即使内部开关是「开」，对外也必须报告为
	// **不可用**——否则用户会看到开关是开的却没有任何数据被采集。
	defer SetSinkForTest(nil)()
	defer SetGatewayEnabledForTest(true)()

	status := GetGatewayStatus()
	assert.False(t, status.CanEnable, "云盘未配置时不允许开启")
	assert.False(t, status.Enabled, "不可用时对外的 Enabled 必须是 false")
	assert.NotEmpty(t, status.Reason, "必须给出可读原因，供管理端展示")
	assert.Equal(t, "disabled", status.SinkName)
}

func TestGatewayStatusEnabledWhenSinkConfigured(t *testing.T) {
	defer SetSinkForTest(fakeSink{name: "onedrive-fake"})()
	defer SetGatewayEnabledForTest(true)()

	status := GetGatewayStatus()
	assert.True(t, status.CanEnable)
	assert.True(t, status.Enabled)
	assert.Empty(t, status.Reason)
	assert.Equal(t, "onedrive-fake", status.SinkName)
}

func TestCanEnableGatewayTracksSinkConfiguration(t *testing.T) {
	restore := SetSinkForTest(nil)
	assert.False(t, CanEnableGateway(), "未配置云盘 → 不能开启")
	restore()

	restore2 := SetSinkForTest(fakeSink{name: "x"})
	assert.True(t, CanEnableGateway(), "已配置云盘 → 可开启")
	restore2()
}

func TestSetGatewayEnabledInvalidatesConsentCache(t *testing.T) {
	// 若不清缓存，「刚关闭总开关」在 TTL 内仍会被判为可采集——
	// 那是可观测的隐私问题（管理员刚关，数据还在传）。
	defer SetSinkForTest(fakeSink{name: "t"})()
	ResetConsentCacheForTest()

	storeConsentCache(42, consentEntry{granted: true, policyVersion: CurrentPolicyVersion})
	consentCacheMu.RLock()
	_, cached := consentCache[42]
	consentCacheMu.RUnlock()
	assert.True(t, cached, "前置：缓存应有该用户")

	defer SetGatewayEnabledForTest(true)()
	consentCacheMu.RLock()
	_, stillCached := consentCache[42]
	consentCacheMu.RUnlock()
	assert.False(t, stillCached, "切换总开关后授权缓存必须被清空")
}

func TestInvalidateConsentRemovesOnlyThatUser(t *testing.T) {
	// 撤回必须**立即生效**（不能等 TTL）——用户点了撤回却看到数据继续上传，
	// 会直接失去信任。
	ResetConsentCacheForTest()
	storeConsentCache(1, consentEntry{granted: true, policyVersion: CurrentPolicyVersion})
	storeConsentCache(2, consentEntry{granted: true, policyVersion: CurrentPolicyVersion})

	InvalidateConsent(1)

	consentCacheMu.RLock()
	_, has1 := consentCache[1]
	_, has2 := consentCache[2]
	consentCacheMu.RUnlock()
	assert.False(t, has1, "被撤回用户的缓存必须清掉")
	assert.True(t, has2, "其他用户不受影响")
}

func TestConsentCacheIsBounded(t *testing.T) {
	// 内存不得随用户数无界增长。
	ResetConsentCacheForTest()
	for i := range consentCacheMaxEntries + 500 {
		storeConsentCache(i, consentEntry{granted: true, policyVersion: CurrentPolicyVersion})
	}
	consentCacheMu.RLock()
	size := len(consentCache)
	consentCacheMu.RUnlock()
	assert.LessOrEqual(t, size, consentCacheMaxEntries,
		"缓存必须被硬守住（当前 %d）", size)
}

func TestCurrentPolicyVersionIsNonEmpty(t *testing.T) {
	// 空版本号会让「同意旧条款」与「同意当前条款」无法区分——合规缺口。
	assert.NotEmpty(t, CurrentPolicyVersion)
}

func TestStalePolicyVersionConsentDoesNotGrant(t *testing.T) {
	// v2 声明新增「模型回复同样入库」「管理员可查看/导出原始对话」两条，
	// 是采集范围的实质性扩张。用户在 v1 文本下点过的「同意」不能覆盖它 ——
	// 否则等于拿旧授权采新范围的数据，用户并没有被告知。
	require.NoError(t, ensureTestDB(t))
	defer SetSinkForTest(fakeSink{name: "t"})()
	defer SetGatewayEnabledForTest(true)()
	ResetConsentCacheForTest()

	const uid = 987001
	stale := CurrentPolicyVersion + "-stale"
	require.NoError(t, model.UpsertTrainingDataConsent(uid, true, stale, 1791937800))
	InvalidateConsent(uid)
	ResetConsentCacheForTest()

	assert.False(t, IsGranted(uid),
		"同意的是旧版本声明时，不得凭该记录采集")

	status, err := GetConsentStatus(uid)
	require.NoError(t, err)
	assert.False(t, status.Granted, "对外状态同样必须报告为未授权")
	assert.True(t, status.NeedsReconsent,
		"已授权用户遇到声明改版时必须被标记为「需重新确认」，"+
			"否则前端只会静默把开关变成关闭，用户不知道发生了什么")
	assert.Equal(t, CurrentPolicyVersion, status.PolicyVersion,
		"必须把当前版本告诉前端，前端据此要求用户重新确认")
}

func TestNeedsReconsentIsFalseForNeverGrantedAndForRevoked(t *testing.T) {
	// 「需重新确认」必须**只**在声明改版这一种情况下为真。
	// 从未同意 / 主动撤回时开关本就是关的，再提示「请重新确认」是噪音，
	// 还会让用户误以为自己的操作没生效。
	require.NoError(t, ensureTestDB(t))
	defer SetSinkForTest(fakeSink{name: "t"})()
	defer SetGatewayEnabledForTest(true)()
	ResetConsentCacheForTest()

	const neverGranted = 987002
	status, err := GetConsentStatus(neverGranted)
	require.NoError(t, err)
	assert.False(t, status.NeedsReconsent, "从未同意不得提示重新确认")

	const revoked = 987003
	require.NoError(t, model.UpsertTrainingDataConsent(revoked, false, CurrentPolicyVersion, 1791937800))
	status, err = GetConsentStatus(revoked)
	require.NoError(t, err)
	assert.False(t, status.Granted)
	assert.False(t, status.NeedsReconsent,
		"用户主动撤回后不得提示「请重新确认」")
}

func TestGrantAndRevokeRejectInvalidUserId(t *testing.T) {
	// 传非法 ID 必须报错且不改动状态（避免把状态写到错误的行上）。
	assert.Error(t, GrantConsent(0, CurrentPolicyVersion))
	assert.Error(t, GrantConsent(-5, CurrentPolicyVersion))
	assert.Error(t, RevokeConsent(0))
	assert.Error(t, RevokeConsent(-5))
}

func TestErrGatewayDisabledIsDistinct(t *testing.T) {
	// 与「云盘未配置」是不同原因，管理端要给不同指引。
	assert.NotErrorIs(t, ErrGatewayDisabled, ErrSinkNotConfigured)
	assert.NotErrorIs(t, ErrSinkNotConfigured, ErrGatewayDisabled)
}
