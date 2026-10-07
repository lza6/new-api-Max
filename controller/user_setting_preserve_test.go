package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateUserSettingPreservesUnrelatedFields 生产回归（数据丢失 bug）：
// PUT /api/user/setting 旧实现从零构造 dto.UserSetting{}，导致每次保存「通知设置」
// 都会抹掉 LastUsedModel / Language / SidebarModules / BillingPreference / MemoryInjection
// 等无关字段。修复后必须以现有设置为基底、只覆盖通知字段。
func TestUpdateUserSettingPreservesUnrelatedFields(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	user := model.User{
		Username: "preserve-fields-user", Password: "password",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&user).Error)

	// 预置一条含「无关字段」的设置。
	seed := dto.UserSetting{
		LastUsedModel:     "deepseek-v4.1-flash",
		Language:          "zh",
		SidebarModules:    `{"console":true}`,
		BillingPreference: "wallet",
		MemoryInjection:   "I prefer concise answers",
	}
	require.NoError(t, model.UpdateUserSetting(user.Id, seed))

	// 调用通知设置保存端点（只带通知字段）。
	body, _ := json.Marshal(map[string]any{
		"notify_type":                    "webhook",
		"quota_warning_threshold":        80,
		"webhook_url":                    "https://example.com/hook",
		"webhook_secret":                 "secret",
		"accept_unset_model_ratio_model": false,
		"record_ip_log":                  true,
	})
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", user.Id)
	c.Set("role", common.RoleCommonUser)
	UpdateUserSetting(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// 断言：通知字段已更新，且无关字段**保留**。
	reloaded, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	got := reloaded.GetSetting()
	assert.Equal(t, "webhook", got.NotifyType)
	assert.Equal(t, 80.0, got.QuotaWarningThreshold)
	assert.Equal(t, "https://example.com/hook", got.WebhookUrl)

	assert.Equal(t, "deepseek-v4.1-flash", got.LastUsedModel, "LastUsedModel must survive")
	assert.Equal(t, "zh", got.Language, "Language must survive")
	assert.Equal(t, `{"console":true}`, got.SidebarModules, "SidebarModules must survive")
	assert.Equal(t, "wallet", got.BillingPreference, "BillingPreference must survive")
	assert.Equal(t, "I prefer concise answers", got.MemoryInjection, "MemoryInjection must survive")
}
