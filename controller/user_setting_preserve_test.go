package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// postSelf 构造并执行一次 PUT /api/user/self 请求。
func postSelf(t *testing.T, userID int, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/self", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", userID)
	c.Set("role", common.RoleCommonUser)
	UpdateSelf(c)
	return rec
}

// TestUpdateSelfMemoryInjection T8：记忆注入文本的保存 / 清除 / 限长。
//
// 与 language 同构：以现有设置为基底只覆盖该字段，不抹掉其它设置；
// 文本会进入每次上游请求的 system 前缀，因此必须按字符数限长。
func TestUpdateSelfMemoryInjection(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	user := model.User{
		Username: "memory-injection-user", Password: "password",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&user).Error)

	// 预置无关字段，验证保存记忆不会抹掉它们。
	require.NoError(t, model.UpdateUserSetting(user.Id, dto.UserSetting{
		Language:          "zh",
		LastUsedModel:     "deepseek-v4.1-flash",
		BillingPreference: "wallet",
	}))

	// 1) 保存一段记忆。
	rec := postSelf(t, user.Id, map[string]any{"memory_injection": "我常用 Go，请用简体中文回答"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	reloaded, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	got := reloaded.GetSetting()
	assert.Equal(t, "我常用 Go，请用简体中文回答", got.MemoryInjection)
	assert.Equal(t, "zh", got.Language, "保存记忆不得抹掉 Language")
	assert.Equal(t, "deepseek-v4.1-flash", got.LastUsedModel, "保存记忆不得抹掉 LastUsedModel")
	assert.Equal(t, "wallet", got.BillingPreference, "保存记忆不得抹掉 BillingPreference")

	// 2) 超长文本被拒且不落库（按 Unicode 码点计数）。
	tooLong := strings.Repeat("记", dto.MaxMemoryInjectionRunes+1)
	postSelf(t, user.Id, map[string]any{"memory_injection": tooLong})
	reloaded, err = model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "我常用 Go，请用简体中文回答", reloaded.GetSetting().MemoryInjection,
		"超长文本必须被拒绝且不改变已存值")

	// 3) 恰好等于上限被接受。
	atLimit := strings.Repeat("记", dto.MaxMemoryInjectionRunes)
	rec = postSelf(t, user.Id, map[string]any{"memory_injection": atLimit})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	reloaded, err = model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, atLimit, reloaded.GetSetting().MemoryInjection)

	// 4) 空串可清除记忆。
	rec = postSelf(t, user.Id, map[string]any{"memory_injection": ""})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	reloaded, err = model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Empty(t, reloaded.GetSetting().MemoryInjection, "空串应清除记忆")
}
