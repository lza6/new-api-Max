package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/middleware"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestServerStatsAdminOnly T3：实时状态接口仅管理员可访问。
// 普通用户/未登录请求必须被 AdminAuth 拒绝（403/401），不得泄露服务器状态。
func TestServerStatsAdminOnly(t *testing.T) {
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	common.SessionSecret = "web-protection-stats-test-secret"
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
	})

	normal := &model.User{
		Username: "wp-normal-user", Password: "unused", AffCode: "ab12", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(normal).Error)
	admin := &model.User{
		Username: "wp-admin-user", Password: "unused", AffCode: "cd34", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(admin).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/admin/web-protection/server-stats", middleware.AdminAuth(), GetServerStats)

	// 普通用户 → 403
	normalSession, err := service.CreateLoginSession(normal.Id, "password", "127.0.0.1", "wp-normal")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/web-protection/server-stats", nil)
	req.Header.Set("Authorization", "Bearer "+normalSession.AccessToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 未登录 → 401
	req = httptest.NewRequest(http.MethodGet, "/api/admin/web-protection/server-stats", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 管理员 → 200
	adminSession, err := service.CreateLoginSession(admin.Id, "password", "127.0.0.1", "wp-admin")
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodGet, "/api/admin/web-protection/server-stats", nil)
	req.Header.Set("Authorization", "Bearer "+adminSession.AccessToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	// 响应含实时字段（网络吞吐必在）
	var body struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Contains(t, body.Data, "network_in_mbps")
	assert.Contains(t, body.Data, "network_out_mbps")
}

// TestWebProtectionPathAndUAPolicy T3-1：策略维度（路径白/黑名单 + UA 白名单）
// 经真实中间件判定；默认空配置零行为变化；各维度独立生效。
func TestWebProtectionPathAndUAPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := operation_setting.GetWebProtectionSetting()
	prevEnabled := settings.Enabled
	prevAllowed := settings.AllowedPaths
	prevBlocked := settings.BlockedPaths
	prevUA := settings.UAAllowlist
	t.Cleanup(func() {
		settings.Enabled = prevEnabled
		settings.AllowedPaths = prevAllowed
		settings.BlockedPaths = prevBlocked
		settings.UAAllowlist = prevUA
	})

	newRouter := func() *gin.Engine {
		router := gin.New()
		require.NoError(t, router.SetTrustedProxies(nil))
		router.Use(middleware.WebProtection())
		router.GET("/dashboard", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		router.GET("/admin/users", func(c *gin.Context) { c.String(http.StatusOK, "admin") })
		router.GET("/assets/app.js", func(c *gin.Context) { c.String(http.StatusOK, "js") })
		return router
	}
	do := func(router *gin.Engine, path, ua string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "192.0.2.10:1234"
		if ua != "" {
			req.Header.Set("User-Agent", ua)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	t.Run("empty config allows all", func(t *testing.T) {
		settings.Enabled = true
		settings.AllowedPaths, settings.BlockedPaths, settings.UAAllowlist = nil, nil, nil
		assert.Equal(t, http.StatusOK, do(newRouter(), "/dashboard", "anything/1.0").Code)
		assert.Equal(t, http.StatusOK, do(newRouter(), "/admin/users", "anything/1.0").Code)
	})

	t.Run("blocked path rejected", func(t *testing.T) {
		settings.Enabled = true
		settings.BlockedPaths = []string{"/admin"}
		settings.AllowedPaths, settings.UAAllowlist = nil, nil
		w := do(newRouter(), "/admin/users", "anything/1.0")
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, "60", w.Header().Get("Retry-After"))
		assert.JSONEq(t, `{"error":{"message":"path blocked","type":"blocked_path","code":"blocked_path"}}`, w.Body.String())
		assert.Equal(t, http.StatusOK, do(newRouter(), "/dashboard", "anything/1.0").Code)
	})

	t.Run("allowed path mismatch rejected", func(t *testing.T) {
		settings.Enabled = true
		settings.AllowedPaths = []string{"/dashboard"}
		settings.BlockedPaths, settings.UAAllowlist = nil, nil
		assert.Equal(t, http.StatusOK, do(newRouter(), "/dashboard", "anything/1.0").Code)
		w := do(newRouter(), "/admin/users", "anything/1.0")
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.JSONEq(t, `{"error":{"message":"path not allowed","type":"not_allowed_path","code":"not_allowed_path"}}`, w.Body.String())
	})

	t.Run("allowed path glob", func(t *testing.T) {
		settings.Enabled = true
		settings.AllowedPaths = []string{"/assets/*"}
		settings.BlockedPaths, settings.UAAllowlist = nil, nil
		assert.Equal(t, http.StatusOK, do(newRouter(), "/assets/app.js", "anything/1.0").Code)
		assert.Equal(t, http.StatusTooManyRequests, do(newRouter(), "/dashboard", "anything/1.0").Code)
	})

	t.Run("ua allowlist", func(t *testing.T) {
		settings.Enabled = true
		settings.UAAllowlist = []string{"Mozilla"}
		settings.AllowedPaths, settings.BlockedPaths = nil, nil
		assert.Equal(t, http.StatusOK, do(newRouter(), "/dashboard", "mozilla/5.0 test").Code)
		w := do(newRouter(), "/dashboard", "curl/8.1")
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.JSONEq(t, `{"error":{"message":"user agent not allowed","type":"ua_not_allowed","code":"ua_not_allowed"}}`, w.Body.String())
	})

	t.Run("disabled protection is pass-through", func(t *testing.T) {
		settings.Enabled = false
		settings.BlockedPaths = []string{"/admin"}
		settings.UAAllowlist = []string{"Mozilla"}
		assert.Equal(t, http.StatusOK, do(newRouter(), "/admin/users", "curl/8.1").Code)
	})
}

// TestWebProtectionPathAndUAGettersSanitize T3-1：getter 清洗空白条目、空配置回退空。
func TestWebProtectionPathAndUAGettersSanitize(t *testing.T) {
	settings := operation_setting.GetWebProtectionSetting()
	prevAllowed, prevBlocked, prevUA := settings.AllowedPaths, settings.BlockedPaths, settings.UAAllowlist
	t.Cleanup(func() {
		settings.AllowedPaths, settings.BlockedPaths, settings.UAAllowlist = prevAllowed, prevBlocked, prevUA
	})
	settings.AllowedPaths = []string{"/a", " ", "\t/b"}
	settings.BlockedPaths = []string{"", "/c"}
	settings.UAAllowlist = []string{"Mozilla", ""}
	allowed, blocked := operation_setting.GetWebProtectionPathPolicy()
	assert.Equal(t, []string{"/a", "/b"}, allowed)
	assert.Equal(t, []string{"/c"}, blocked)
	assert.Equal(t, []string{"Mozilla"}, operation_setting.GetWebProtectionUAAllowlist())

	settings.AllowedPaths, settings.BlockedPaths, settings.UAAllowlist = nil, nil, nil
	allowed, blocked = operation_setting.GetWebProtectionPathPolicy()
	assert.Nil(t, allowed)
	assert.Nil(t, blocked)
	assert.Nil(t, operation_setting.GetWebProtectionUAAllowlist())
}
