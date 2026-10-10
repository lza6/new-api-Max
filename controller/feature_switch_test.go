package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/middleware"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/feature_switch"
)

// setupFeatureSwitchTestDB 在共用内存 SQLite 基础上补建本接口需要的表。
func setupFeatureSwitchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.AuditLog{}))

	// model.UpdateOption 会把值同步进 common.OptionMap；生产里由 InitOptionMap
	// 初始化，单元测试环境需要显式建好，否则写路径 panic。
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()

	return db
}

// resetFeatureSwitch 清理被用例改动过的开关，避免用例间互相影响。
func resetFeatureSwitch(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		require.NoError(t, feature_switch.Reset(key))
	}
	t.Cleanup(func() {
		for _, key := range keys {
			_ = feature_switch.Reset(key)
		}
	})
}

type featureSwitchAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Switches []feature_switch.Snapshot `json:"switches"`
		Metrics  map[string]float64        `json:"metrics"`
	} `json:"data"`
}

func callFeatureSwitchAPI(t *testing.T, recorder *httptest.ResponseRecorder, body string) featureSwitchAPIResponse {
	t.Helper()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("PUT", "/api/option/feature-switches", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateFeatureSwitch(c)

	var parsed featureSwitchAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &parsed))
	return parsed
}

// 管理端接口必须只对 root 开放：鉴权被摘掉时本用例会失败。
func TestFeatureSwitchEndpointsRequireRootAuth(t *testing.T) {
	setupFeatureSwitchTestDB(t)
	resetFeatureSwitch(t, common.FlagRelayAuditEnabled)

	router := gin.New()
	router.GET("/api/option/feature-switches", middleware.RootAuth(), GetFeatureSwitches)
	router.PUT("/api/option/feature-switches", middleware.RootAuth(), UpdateFeatureSwitch)

	for _, tc := range []struct {
		method string
		body   string
	}{
		{"GET", ""},
		{"PUT", `{"key":"RELAY_AUDIT_ENABLED","value":"true"}`},
	} {
		request := httptest.NewRequest(tc.method, "/api/option/feature-switches", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		router.ServeHTTP(result, request)

		assert.GreaterOrEqual(t, result.Code, 400,
			"%s 未鉴权必须被拒绝（摘掉 RootAuth 会让本断言失败）", tc.method)
	}

	// 更关键的一条：被拒绝的请求**不得留下任何状态变更**。
	assert.False(t, feature_switch.IsEnabled(common.FlagRelayAuditEnabled),
		"未授权请求不得改变开关状态")
	_, configured := feature_switch.ConfiguredValue(common.FlagRelayAuditEnabled)
	assert.False(t, configured, "未授权请求不得留下持久化配置")
}

func TestGetFeatureSwitchesReturnsAllRegisteredSwitches(t *testing.T) {
	setupFeatureSwitchTestDB(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/api/option/feature-switches", nil)
	GetFeatureSwitches(c)

	var parsed featureSwitchAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &parsed))

	require.True(t, parsed.Success, parsed.Message)
	assert.Len(t, parsed.Data.Switches, len(common.FeatureFlagKeys()),
		"接口必须暴露全部受治理开关")
	assert.NotNil(t, parsed.Data.Metrics, "即便无数据也必须返回 metrics 字段（前端据此渲染）")

	// 每个开关都必须带 i18n 键与回滚提示，否则管理页无法渲染。
	for _, s := range parsed.Data.Switches {
		assert.NotEmpty(t, s.Key)
		assert.NotEmpty(t, s.TitleKey, "%s 缺少标题 i18n 键", s.Key)
		assert.NotEmpty(t, s.DescriptionKey, "%s 缺少描述 i18n 键", s.Key)
		assert.NotEmpty(t, s.RollbackHint, "%s 缺少回滚提示 i18n 键", s.Key)
	}
}

func TestUpdateFeatureSwitchRejectsBadInput(t *testing.T) {
	setupFeatureSwitchTestDB(t)
	resetFeatureSwitch(t, common.FlagComplexityRouting)

	for _, tc := range []struct {
		name string
		body string
	}{
		{"未注册的开关", `{"key":"NOT_REGISTERED","value":"true"}`},
		{"非法取值", `{"key":"COMPLEXITY_ROUTING","value":"maybe"}`},
		{"枚举取值越界", `{"key":"POLICY_ENGINE_MODE","value":"aggressive"}`},
		{"缺少 value", `{"key":"COMPLEXITY_ROUTING"}`},
		{"空请求体", `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			parsed := callFeatureSwitchAPI(t, recorder, tc.body)
			assert.False(t, parsed.Success, "非法输入必须被拒绝：%s", tc.name)
			assert.NotEmpty(t, parsed.Message, "拒绝时应给出可读原因")
		})
	}

	assert.False(t, feature_switch.IsEnabled(common.FlagComplexityRouting),
		"非法输入不得产生任何副作用")
	_, configured := feature_switch.ConfiguredValue(common.FlagComplexityRouting)
	assert.False(t, configured)
}

func TestUpdateFeatureSwitchHotAppliesAndPersists(t *testing.T) {
	db := setupFeatureSwitchTestDB(t)
	resetFeatureSwitch(t, common.FlagRelayAuditEnabled, common.FlagComplexityRouting)

	// 1) 打开：应即时生效（热更新，无需重启）。
	recorder := httptest.NewRecorder()
	parsed := callFeatureSwitchAPI(t, recorder, `{"key":"RELAY_AUDIT_ENABLED","value":"true"}`)
	require.True(t, parsed.Success, parsed.Message)
	assert.True(t, feature_switch.IsEnabled(common.FlagRelayAuditEnabled),
		"变更必须立即对热路径生效")
	assert.True(t, service.RelayAuditEnabled(), "能力模块的 getter 必须读到新值")

	// 2) 持久化：选项表必须写入 feature_switch.values。
	var option model.Option
	require.NoError(t, db.Where("key = ?", "feature_switch.values").First(&option).Error,
		"变更必须落库，否则重启后丢失")
	assert.Contains(t, option.Value, common.FlagRelayAuditEnabled)

	// 3) 审计：必须留痕，且记录 key 与变更前后值。
	var audit model.AuditLog
	require.NoError(t, db.Where("action = ?", "feature_switch.update").First(&audit).Error,
		"能力开关变更必须写审计日志")
	assert.Contains(t, audit.Content, "RELAY_AUDIT_ENABLED",
		"审计内容必须记录被改的开关名")

	// 4) 关闭：同样即时生效。
	recorder = httptest.NewRecorder()
	parsed = callFeatureSwitchAPI(t, recorder, `{"key":"RELAY_AUDIT_ENABLED","value":"false"}`)
	require.True(t, parsed.Success, parsed.Message)
	assert.False(t, feature_switch.IsEnabled(common.FlagRelayAuditEnabled))
	assert.False(t, service.RelayAuditEnabled())

	// 5) Reset：回退 env 默认（未设 env 时为 false），并清除持久化配置。
	recorder = httptest.NewRecorder()
	parsed = callFeatureSwitchAPI(t, recorder, `{"key":"RELAY_AUDIT_ENABLED","reset":true}`)
	require.True(t, parsed.Success, parsed.Message)
	_, configured := feature_switch.ConfiguredValue(common.FlagRelayAuditEnabled)
	assert.False(t, configured, "Reset 必须清除管理员配置")
	assert.NotContains(t, optionValueFor(t, db, "feature_switch.values"), common.FlagRelayAuditEnabled)
}

// optionValueFor 读取某个选项的当前值（用例辅助）。
func optionValueFor(t *testing.T, db *gorm.DB, key string) string {
	t.Helper()
	var option model.Option
	require.NoError(t, db.Where("key = ?", key).First(&option).Error)
	return option.Value
}
