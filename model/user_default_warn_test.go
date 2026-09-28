package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestNewUserDefaultQuotaWarningThreshold B6-3：新用户注册时默认开启
// 80% 成本告警阈值；显式传 Setting 的用户不被覆盖。
func TestNewUserDefaultQuotaWarningThreshold(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	// 显式配置 QuotaForNewUser>0，确保 80% 阈值注入路径真实执行
	// （测试环境默认 0 时该注入被跳过，断言会落空）。
	originalQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 500000
	t.Cleanup(func() { common.QuotaForNewUser = originalQuota })

	// 零值 Setting 新用户 → 阈值 = QuotaForNewUser * 0.8。
	newUser := &User{Username: "default_warn", Email: "w@example.com"}
	require.NoError(t, newUser.Insert(0))
	var reloaded User
	require.NoError(t, DB.Where("username = ?", "default_warn").First(&reloaded).Error)
	var setting dto.UserSetting
	require.NoError(t, common.UnmarshalJsonStr(reloaded.Setting, &setting))
	expected := float64(common.QuotaForNewUser) * 0.8
	assert.Equal(t, expected, setting.QuotaWarningThreshold, "新用户默认 80% 告警阈值")
	// P1-1（批次 005）：注入默认 80% 必须标记 QuotaWarnThresholdsDefault=true，
	// 决策层据此走多档默认而非误判为「用户显式单档」（否则多档对主流用户不可达）。
	assert.True(t, setting.QuotaWarnThresholdsDefault, "注册注入的默认阈值须标记为 default")

	// 已带 Setting 的用户 → 原样保留。
	custom := dto.UserSetting{QuotaWarningThreshold: 123.0}
	customUser := &User{Username: "custom_warn", Email: "c@example.com"}
	customUser.SetSetting(custom)
	require.NoError(t, customUser.Insert(0))
	var reloadedCustom User
	require.NoError(t, DB.Where("username = ?", "custom_warn").First(&reloadedCustom).Error)
	var got dto.UserSetting
	require.NoError(t, common.UnmarshalJsonStr(reloadedCustom.Setting, &got))
	assert.Equal(t, 123.0, got.QuotaWarningThreshold, "显式设置不被覆盖")
}
