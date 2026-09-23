package service

import (
	"testing"

	"github.com/lza6/new-api-Max/setting/relay_setting"
	"github.com/stretchr/testify/assert"
)

// 重置 relay 设置到已知基线，避免测试间相互污染。
func resetRelayRateSettingForTest() {
	s := relay_setting.GetRelaySetting()
	s.UserRateLimitOverrides = nil
	s.GroupRateLimitOverrides = nil
	s.UserBaseRateLimitEnabled = nil
	s.UserBaseConcurrencyLimit = 3
	s.UserBaseRpmLimit = 120
}

func TestResolveUserRateLimitBaseDefault(t *testing.T) {
	resetRelayRateSettingForTest()
	c, r, src := ResolveUserRateLimit(7, "default")
	assert.Equal(t, 3, c)
	assert.Equal(t, 120, r)
	assert.Equal(t, "base", src)
}

func TestResolveUserRateLimitOverrides(t *testing.T) {
	resetRelayRateSettingForTest()
	relay_setting.GetRelaySetting().UserRateLimitOverrides = map[int]relay_setting.RateLimitTier{7: {Concurrency: 50, Rpm: 1000}}
	c, r, src := ResolveUserRateLimit(7, "default")
	assert.Equal(t, 50, c)
	assert.Equal(t, 1000, r)
	assert.Equal(t, "user", src)

	resetRelayRateSettingForTest()
	relay_setting.GetRelaySetting().GroupRateLimitOverrides = map[string]relay_setting.RateLimitTier{"vip": {Concurrency: 20, Rpm: 400}}
	c, r, src = ResolveUserRateLimit(8, "vip")
	assert.Equal(t, 20, c)
	assert.Equal(t, 400, r)
	assert.Equal(t, "group", src)
}

func TestResolveUserRateLimitDisabled(t *testing.T) {
	resetRelayRateSettingForTest()
	disabled := false
	relay_setting.GetRelaySetting().UserBaseRateLimitEnabled = &disabled
	c, r, src := ResolveUserRateLimit(9, "default")
	assert.Equal(t, 0, c)
	assert.Equal(t, 0, r)
	assert.Equal(t, "off", src)
}
