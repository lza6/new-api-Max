package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// TestUpdateSelfSkillsAndPresets T13：技能表与 Agent 预设的保存、限长、限数，
// 以及「保存其一不抹掉其它设置」。
func TestUpdateSelfSkillsAndPresets(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	user := model.User{
		Username: "skills-presets-user", Password: "password",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, model.UpdateUserSetting(user.Id, dto.UserSetting{
		Language:        "zh",
		MemoryInjection: "我常用 Go",
	}))

	// 1) 保存技能表。
	rec := postSelf(t, user.Id, map[string]any{
		"skills": []map[string]any{
			{"id": "s1", "name": "先给结论", "prompt": "先一句话给结论，再展开依据", "enabled": true},
			{"id": "s2", "name": "给代码", "prompt": "涉及编码时给出可运行示例", "enabled": false},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	reloaded, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	got := reloaded.GetSetting()
	require.Len(t, got.Skills, 2)
	assert.Equal(t, "先给结论", got.Skills[0].Name)
	assert.True(t, got.Skills[0].Enabled)
	assert.False(t, got.Skills[1].Enabled)
	// 保存技能不得抹掉记忆/语言。
	assert.Equal(t, "我常用 Go", got.MemoryInjection, "保存技能不得抹掉 MemoryInjection")
	assert.Equal(t, "zh", got.Language, "保存技能不得抹掉 Language")

	// 2) 保存 Agent 预设（含指针字段）。
	rec = postSelf(t, user.Id, map[string]any{
		"agent_presets": []map[string]any{
			{
				"id": "p1", "name": "代码助手", "model": "deepseek-v4.1-flash",
				"group": "default", "system_prompt": "你是资深 Go 工程师",
				"temperature": 0.2, "max_tokens": 4096, "reasoning_effort": "medium",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	reloaded, err = model.GetUserById(user.Id, false)
	require.NoError(t, err)
	got = reloaded.GetSetting()
	require.Len(t, got.AgentPresets, 1)
	assert.Equal(t, "代码助手", got.AgentPresets[0].Name)
	assert.Equal(t, "deepseek-v4.1-flash", got.AgentPresets[0].Model)
	require.NotNil(t, got.AgentPresets[0].Temperature)
	assert.InDelta(t, 0.2, *got.AgentPresets[0].Temperature, 1e-9)
	require.NotNil(t, got.AgentPresets[0].MaxTokens)
	assert.Equal(t, uint(4096), *got.AgentPresets[0].MaxTokens)
	// 保存预设不得抹掉技能。
	assert.Len(t, got.Skills, 2, "保存预设不得抹掉 Skills")

	// 3) 越界一律被拒且不落库（不做静默截断）。
	// 注意：本项目 API 约定是 HTTP 200 + body.success=false（见 common.ApiError*），
	// 所以「被拒」要看 body 的 success，而不是 HTTP 状态码。
	before := got.AgentPresets
	beforeSkills := got.Skills
	for name, payload := range map[string]map[string]any{
		"姓名超长":          {"skills": []map[string]any{{"name": strings.Repeat("名", dto.MaxSkillNameRunes+1), "prompt": "x"}}},
		"内容超长":          {"skills": []map[string]any{{"name": "n", "prompt": strings.Repeat("字", dto.MaxSkillPromptRunes+1)}}},
		"技能数超限":         {"skills": tooManySkills(dto.MaxUserSkills + 1)},
		"预设数超限":         {"agent_presets": tooManyPresets(dto.MaxAgentPresets + 1)},
		"temperature越界": {"agent_presets": []map[string]any{{"name": "n", "temperature": 3}}},
		"max_tokens越界":  {"agent_presets": []map[string]any{{"name": "n", "max_tokens": 32001}}},
	} {
		rec = postSelf(t, user.Id, payload)
		var body struct {
			Success bool `json:"success"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), name)
		assert.False(t, body.Success, "应被拒绝: "+name)

		reloaded, err = model.GetUserById(user.Id, false)
		require.NoError(t, err)
		assert.Len(t, reloaded.GetSetting().AgentPresets, len(before), "被拒后预设不得改变: "+name)
		assert.Len(t, reloaded.GetSetting().Skills, len(beforeSkills), "被拒后技能不得改变: "+name)
	}

	// 4) 空数组是合法输入（清空），不是越界。
	rec = postSelf(t, user.Id, map[string]any{"skills": []map[string]any{}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	reloaded, err = model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Empty(t, reloaded.GetSetting().Skills, "空数组应清空技能")
}

func tooManySkills(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range n {
		out[i] = map[string]any{"name": "s" + strconv.Itoa(i), "prompt": "p"}
	}
	return out
}

func tooManyPresets(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range n {
		out[i] = map[string]any{"name": "p" + strconv.Itoa(i)}
	}
	return out
}
