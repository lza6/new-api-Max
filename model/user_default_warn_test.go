package model

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/glebarez/sqlite"
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

	// 零值 Setting 新用户 → 阈值 = QuotaForNewUser * 0.8。
	newUser := &User{Username: "default_warn", Email: "w@example.com"}
	require.NoError(t, newUser.Insert(0))
	var reloaded User
	require.NoError(t, DB.Where("username = ?", "default_warn").First(&reloaded).Error)
	var setting dto.UserSetting
	require.NoError(t, common.UnmarshalJsonStr(reloaded.Setting, &setting))
	expected := float64(common.QuotaForNewUser) * 0.8
	assert.Equal(t, expected, setting.QuotaWarningThreshold, "新用户默认 80% 告警阈值")

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
