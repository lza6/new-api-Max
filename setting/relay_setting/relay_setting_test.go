package relay_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserRateLimitTierDefaults(t *testing.T) {
	// 恢复全局状态，避免污染其它用例。
	prev := relaySetting
	defer func() { relaySetting = prev }()
	relaySetting = RelaySetting{
		UserBaseConcurrencyLimit: DefaultUserBaseConcurrencyLimit,
		UserBaseRpmLimit:         DefaultUserBaseRpmLimit,
	}
	c, r := GetUserRateLimitTier(1, "default")
	require.Equal(t, DefaultUserBaseConcurrencyLimit, c)
	require.Equal(t, DefaultUserBaseRpmLimit, r)

	// 显式关闭 → 不限。
	enabled := false
	relaySetting.UserBaseRateLimitEnabled = &enabled
	c, r = GetUserRateLimitTier(1, "default")
	require.Zero(t, c)
	require.Zero(t, r)
}

func TestGetUserRateLimitTierOverrides(t *testing.T) {
	prev := relaySetting
	defer func() { relaySetting = prev }()
	relaySetting = RelaySetting{
		UserBaseConcurrencyLimit: 3,
		UserBaseRpmLimit:         120,
		GroupRateLimitOverrides:  map[string]RateLimitTier{"vip": {Concurrency: 10, Rpm: 600}},
		UserRateLimitOverrides:   map[int]RateLimitTier{7: {Concurrency: 1, Rpm: 30}},
	}

	// 用户覆盖优先。
	c, r := GetUserRateLimitTier(7, "vip")
	require.Equal(t, 1, c)
	require.Equal(t, 30, r)

	// 分组覆盖次之。
	c, r = GetUserRateLimitTier(8, "vip")
	require.Equal(t, 10, c)
	require.Equal(t, 600, r)

	// 无覆盖回退基础默认。
	c, r = GetUserRateLimitTier(8, "default")
	require.Equal(t, 3, c)
	require.Equal(t, 120, r)

	// 移除覆盖（0,0）。
	SetUserRateLimitOverride(7, RateLimitTier{})
	_, ok := relaySetting.UserRateLimitOverrides[7]
	require.False(t, ok)
	SetGroupRateLimitOverride("vip", RateLimitTier{})
	_, ok = relaySetting.GroupRateLimitOverrides["vip"]
	require.False(t, ok)
}

func TestIsSubscriptionRequiredGroup(t *testing.T) {
	prev := relaySetting
	defer func() { relaySetting = prev }()
	relaySetting = RelaySetting{SubscriptionRequiredGroups: []string{"subscriber", "vip"}}
	require.True(t, IsSubscriptionRequiredGroup("subscriber"))
	require.True(t, IsSubscriptionRequiredGroup("vip"))
	require.False(t, IsSubscriptionRequiredGroup("default"))
	require.False(t, IsSubscriptionRequiredGroup(""))
}

func TestGetUserRateLimitExemptModelsDefaultEmpty(t *testing.T) {
	prev := GetRelaySetting().UserRateLimitExemptModels
	t.Cleanup(func() { GetRelaySetting().UserRateLimitExemptModels = prev })
	GetRelaySetting().UserRateLimitExemptModels = nil
	assert.Empty(t, GetUserRateLimitExemptModels())
	GetRelaySetting().UserRateLimitExemptModels = []string{"google-translate"}
	assert.Equal(t, []string{"google-translate"}, GetUserRateLimitExemptModels())
}
func TestGetNonStreamFirstByteTimeout(t *testing.T) {
	prev := GetRelaySetting().NonStreamFirstByteTimeout
	t.Cleanup(func() { GetRelaySetting().NonStreamFirstByteTimeout = prev })
	assert.Equal(t, DefaultNonStreamFirstByteTimeout, GetNonStreamFirstByteTimeout())
	GetRelaySetting().NonStreamFirstByteTimeout = 0
	assert.Equal(t, 0, GetNonStreamFirstByteTimeout())
	GetRelaySetting().NonStreamFirstByteTimeout = 60
	assert.Equal(t, 60, GetNonStreamFirstByteTimeout())
}
