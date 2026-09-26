package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// webProtectionTestDB 初始化内存 SQLite 的 banned_ips 表，避免 IsIPBannedCached 查询报错。
func webProtectionTestDB(t *testing.T) {
	t.Helper()
	prev := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.BannedIP{}))
	model.DB = db
	t.Cleanup(func() { model.DB = prev })
}

// webProtectionTestCtx 构造一个命中 Web 防护判定链的 gin 上下文（真实请求对象）。
func webProtectionTestCtx(path string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	c.Request.RemoteAddr = "198.51.100.7:4444"
	return c
}

// TestWebProtectionInFlightCounter T3：在线请求计数 Begin+1 / End-1，原子返回一致。
func TestWebProtectionInFlightCounter(t *testing.T) {
	settings := operation_setting.GetWebProtectionSetting()
	prev := *settings
	t.Cleanup(func() { *settings = prev })
	webProtectionTestDB(t)

	settings.Enabled = true
	settings.AutoBan = false

	base := GetWebProtectionInFlight()
	c := webProtectionTestCtx("/dashboard")
	require.True(t, TrackWebRequestBegin(c))
	assert.Equal(t, base+1, GetWebProtectionInFlight())
	TrackWebRequestEnd(c, http.StatusOK)
	assert.Equal(t, base, GetWebProtectionInFlight())
}

// TestWebProtectionInFlightRejectedNotCounted T3：被策略拒绝的请求不进入计数。
func TestWebProtectionInFlightRejectedNotCounted(t *testing.T) {
	settings := operation_setting.GetWebProtectionSetting()
	prev := *settings
	t.Cleanup(func() { *settings = prev })
	webProtectionTestDB(t)
	settings.Enabled = true
	settings.BlockedPaths = []string{"/admin"}
	settings.AllowedPaths, settings.UAAllowlist = nil, nil
	settings.AutoBan = false

	base := GetWebProtectionInFlight()
	c := webProtectionTestCtx("/admin/users")
	assert.False(t, TrackWebRequestBegin(c))
	assert.Equal(t, base, GetWebProtectionInFlight())
	// End 不应被调用（Begin 已拒绝）；即使误调用也不应为负。
	TrackWebRequestEnd(c, http.StatusTooManyRequests)
	assert.Equal(t, base, GetWebProtectionInFlight())
}

// TestWebProtectionInFlightDisabledPassThrough T3：防护关闭时直通且不计数。
func TestWebProtectionInFlightDisabledPassThrough(t *testing.T) {
	settings := operation_setting.GetWebProtectionSetting()
	prev := *settings
	t.Cleanup(func() { *settings = prev })
	webProtectionTestDB(t)
	settings.Enabled = false

	base := GetWebProtectionInFlight()
	c := webProtectionTestCtx("/dashboard")
	require.True(t, TrackWebRequestBegin(c))
	assert.Equal(t, base, GetWebProtectionInFlight())
}

// TestWebProtectionInFlightMidRequestDisable T3：Begin 后中途关闭防护，End 仍回减不泄漏。
func TestWebProtectionInFlightMidRequestDisable(t *testing.T) {
	settings := operation_setting.GetWebProtectionSetting()
	prev := *settings
	t.Cleanup(func() { *settings = prev })
	webProtectionTestDB(t)
	settings.Enabled = true
	settings.AutoBan = false

	base := GetWebProtectionInFlight()
	c := webProtectionTestCtx("/dashboard")
	require.True(t, TrackWebRequestBegin(c))
	assert.Equal(t, base+1, GetWebProtectionInFlight())

	// 请求进行中关闭防护，End 必须仍回减。
	settings.Enabled = false
	TrackWebRequestEnd(c, http.StatusOK)
	assert.Equal(t, base, GetWebProtectionInFlight())
}
