package model

import (
	"errors"
	"testing"

	"github.com/lza6/new-api-Max/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateAndFillDistinguishesNoPasswordAccount：账号存在但无密码时，必须返回
// ErrUserNoPassword（而非笼统的 ErrInvalidCredentials），否则登录失败会显示
// 「用户名或密码错误，或用户已被封禁」，用户误以为账号被封。
func TestValidateAndFillDistinguishesNoPasswordAccount(t *testing.T) {
	setupUserUpdateTestState(t)

	oauthUser := User{
		Username:    "nodeloc-no-password-user",
		Email:       "oauth-only@example.com",
		Password:    "", // 第三方登录注册，从未设置密码
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		AffCode:     "nopass-aff-code",
		Source:      UserSourceLinuxDO,
	}
	require.NoError(t, DB.Create(&oauthUser).Error)

	login := User{Username: oauthUser.Username, Password: "anything"}
	err := login.ValidateAndFill()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUserNoPassword),
		"无密码账号登录应返回 ErrUserNoPassword，实际: %v", err)
	assert.False(t, errors.Is(err, ErrInvalidCredentials),
		"不应退化为笼统的 ErrInvalidCredentials")
}

// TestValidateAndFillWrongPasswordStillGeneric：密码错误的普通账号仍走通用错误，
// 不泄露「账号存在」之外的额外信息。
func TestValidateAndFillWrongPasswordStillGeneric(t *testing.T) {
	setupUserUpdateTestState(t)

	normalUser := User{
		Username:    "password-user",
		Email:       "password@example.com",
		Password:    "correct-hash-placeholder",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		AffCode:     "pw-aff-code",
		Source:      UserSourcePassword,
	}
	require.NoError(t, DB.Create(&normalUser).Error)

	login := User{Username: normalUser.Username, Password: "wrong"}
	err := login.ValidateAndFill()
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrUserNoPassword))
	assert.True(t, errors.Is(err, ErrInvalidCredentials))
}

// TestLoginProviderDisplayName：注册来源 → 可展示的登录方式名。
func TestLoginProviderDisplayName(t *testing.T) {
	setupUserUpdateTestState(t)

	t.Run("builtin sources map to brand names", func(t *testing.T) {
		assert.Equal(t, "LinuxDO", LoginProviderDisplayName(UserSourceLinuxDO))
		assert.Equal(t, "GitHub", LoginProviderDisplayName(UserSourceGithub))
		assert.Equal(t, "Discord", LoginProviderDisplayName(UserSourceDiscord))
	})

	t.Run("password/admin sources give no login hint", func(t *testing.T) {
		assert.Equal(t, "", LoginProviderDisplayName(UserSourcePassword))
		assert.Equal(t, "", LoginProviderDisplayName(UserSourceAdmin))
		assert.Equal(t, "", LoginProviderDisplayName(""))
	})

	t.Run("custom provider resolves to its display name", func(t *testing.T) {
		provider := CustomOAuthProvider{
			Name:    "NodeLoc",
			Slug:    "nodeloc",
			Enabled: true,
		}
		require.NoError(t, DB.Create(&provider).Error)
		assert.Equal(t, "NodeLoc", LoginProviderDisplayName("nodeloc"))
	})

	t.Run("unknown source falls back to raw identifier", func(t *testing.T) {
		assert.Equal(t, "some-new-provider", LoginProviderDisplayName("some-new-provider"))
	})
}

// 时延拉平：账号不存在 / 无密码账号也必须执行一次口令校验，
// 否则响应耗时差异会暴露账号是否存在（OWASP 账号枚举旁路）。
func TestValidateAndFillEqualizesTimingForMissingAccount(t *testing.T) {
	setupUserUpdateTestState(t)

	calls := 0
	usedHash := ""
	previous := validatePasswordAndHash
	validatePasswordAndHash = func(_ string, hash string) bool {
		calls++
		usedHash = hash
		return false
	}
	t.Cleanup(func() { validatePasswordAndHash = previous })

	login := User{Username: "definitely-not-registered-xyz", Password: "guess"}
	err := login.ValidateAndFill()

	require.ErrorIs(t, err, ErrInvalidCredentials)
	assert.Equal(t, 1, calls, "账号不存在时必须执行一次 dummy 口令校验")
	assert.Equal(t, dummyPasswordHash, usedHash)
}

func TestValidateAndFillEqualizesTimingForNoPasswordAccount(t *testing.T) {
	setupUserUpdateTestState(t)

	oauthUser := User{
		Username:    "timing-no-password",
		Email:       "timing-oauth@example.com",
		Password:    "",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		AffCode:     "timing-aff",
		Source:      UserSourceLinuxDO,
	}
	require.NoError(t, DB.Create(&oauthUser).Error)

	calls := 0
	previous := validatePasswordAndHash
	validatePasswordAndHash = func(_ string, _ string) bool {
		calls++
		return false
	}
	t.Cleanup(func() { validatePasswordAndHash = previous })

	login := User{Username: oauthUser.Username, Password: "guess"}
	err := login.ValidateAndFill()

	require.ErrorIs(t, err, ErrUserNoPassword)
	assert.Equal(t, 1, calls, "无密码账号也必须执行一次 dummy 口令校验")
}
