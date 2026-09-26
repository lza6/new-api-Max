package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResetPasswordNeverReturnsPlaintextTempPassword 覆盖 T8/G3 回归：
// 重置密码响应不得包含明文临时密码；密码确实被更新（旧密码不再可用）。
func TestResetPasswordNeverReturnsPlaintextTempPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	email := "reset-g3@example.com"
	// 构造用户（密码哈希格式有效）
	password, err := common.HashAccountPassword("original-password-123")
	require.NoError(t, err)
	user := model.User{
		Username: "reset-g3-user", Email: email, Password: password,
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "resetg3a",
	}
	require.NoError(t, model.DB.Create(&user).Error)

	// 写入密码重置验证码（模拟邮件中的 reset link token）
	token := "reset-token-abcdef"
	common.RegisterVerificationCodeWithKey(email, token, common.PasswordResetPurpose)

	body := `{"email":"` + email + `","token":"` + token + `"}`
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/reset", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	ResetPassword(c)

	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	// 明文临时密码不得出现在响应中
	assert.Empty(t, payload.Data)
	assert.NotContains(t, response.Body.String(), "original-password")
	assert.NotContains(t, response.Body.String(), "reset-token")

	// 密码确实被更新：旧密码校验失败
	var refreshed model.User
	require.NoError(t, model.DB.Where("email = ?", email).First(&refreshed).Error)
	assert.NotEqual(t, password, refreshed.Password)
	// 用旧密码登录应失败（证明不是原地不动）
	assert.False(t, common.ValidatePasswordAndHash("original-password-123", refreshed.Password))
}

// TestResetPasswordRejectsInvalidToken 覆盖过期/无效 token 的回归：失败响应不泄露内部细节。
func TestResetPasswordRejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	email := "reset-g3-invalid@example.com"
	password, err := common.HashAccountPassword("original-password-456")
	require.NoError(t, err)
	user := model.User{
		Username: "reset-g3-invalid", Email: email, Password: password,
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "resetg3b",
	}
	require.NoError(t, model.DB.Create(&user).Error)

	body := `{"email":"` + email + `","token":"wrong-token"}`
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/reset", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	ResetPassword(c)

	// 无效 token -> 业务错误，不返回明文/成功
	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.NotContains(t, response.Body.String(), "original-password-456")
}
