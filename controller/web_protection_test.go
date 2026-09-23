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
