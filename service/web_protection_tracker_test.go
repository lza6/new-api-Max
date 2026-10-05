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

// useWebProtectionSettings 在写锁内修改 Web 防护配置并发布快照，结束后恢复原值。
// 快照发布后再修改返回对象不会生效，故所有写入必须经此辅助。
func useWebProtectionSettings(t *testing.T, mutate func(*operation_setting.WebProtectionSetting)) {
	t.Helper()
	original := *operation_setting.GetWebProtectionSetting()
	t.Cleanup(func() {
		operation_setting.UpdateWebProtectionSetting(func(settings *operation_setting.WebProtectionSetting) {
			*settings = original
		})
	})
	operation_setting.UpdateWebProtectionSetting(mutate)
}

// TestWebProtectionInFlightCounter T3：在线请求计数 Begin+1 / End-1，原子返回一致。
func TestWebProtectionInFlightCounter(t *testing.T) {
	webProtectionTestDB(t)
	useWebProtectionSettings(t, func(settings *operation_setting.WebProtectionSetting) {
		settings.Enabled = true
		settings.AutoBan = false
	})

	base := GetWebProtectionInFlight()
	c := webProtectionTestCtx("/dashboard")
	require.True(t, TrackWebRequestBegin(c))
	assert.Equal(t, base+1, GetWebProtectionInFlight())
	TrackWebRequestEnd(c, http.StatusOK)
	assert.Equal(t, base, GetWebProtectionInFlight())
}

// TestWebProtectionInFlightRejectedNotCounted T3：被策略拒绝的请求不进入计数。
func TestWebProtectionInFlightRejectedNotCounted(t *testing.T) {
	webProtectionTestDB(t)
	useWebProtectionSettings(t, func(settings *operation_setting.WebProtectionSetting) {
		settings.Enabled = true
		settings.BlockedPaths = []string{"/admin"}
		settings.AllowedPaths, settings.UAAllowlist = nil, nil
		settings.AutoBan = false
	})

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
	webProtectionTestDB(t)
	useWebProtectionSettings(t, func(settings *operation_setting.WebProtectionSetting) {
		settings.Enabled = false
	})

	base := GetWebProtectionInFlight()
	c := webProtectionTestCtx("/dashboard")
	require.True(t, TrackWebRequestBegin(c))
	assert.Equal(t, base, GetWebProtectionInFlight())
}

// TestWebProtectionInFlightMidRequestDisable T3：Begin 后中途关闭防护，End 仍回减不泄漏。
func TestWebProtectionInFlightMidRequestDisable(t *testing.T) {
	webProtectionTestDB(t)
	useWebProtectionSettings(t, func(settings *operation_setting.WebProtectionSetting) {
		settings.Enabled = true
		settings.AutoBan = false
	})

	base := GetWebProtectionInFlight()
	c := webProtectionTestCtx("/dashboard")
	require.True(t, TrackWebRequestBegin(c))
	assert.Equal(t, base+1, GetWebProtectionInFlight())

	// 请求进行中关闭防护，End 必须仍回减。
	operation_setting.UpdateWebProtectionSetting(func(settings *operation_setting.WebProtectionSetting) {
		settings.Enabled = false
	})
	TrackWebRequestEnd(c, http.StatusOK)
	assert.Equal(t, base, GetWebProtectionInFlight())
}

// webProtectionTestCtxIP 构造指定来源 IP 的 gin 上下文（默认 198.51.100.7 外部）。
func webProtectionTestCtxIP(remoteIP, requestPath string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, requestPath, nil)
	c.Request.RemoteAddr = remoteIP + ":4444"
	return c
}

// TestWebProtectionTrustedSourceBypassesLocalIPs 回归：Docker 网关/环回/内网
// 来源完全豁免限流与自动封禁（只计数）——Web 防护只防外部恶意攻击，
// 内网自身流量（Caddy 健康检查 172.18.0.1）绝不能被误封。
func TestWebProtectionTrustedSourceBypassesLocalIPs(t *testing.T) {
	webProtectionTestDB(t)
	useWebProtectionSettings(t, func(settings *operation_setting.WebProtectionSetting) {
		settings.Enabled = true
		settings.AutoBan = true     // 自动封禁开启，确保内部来源仍不被封
		settings.LimitPerSecond = 1 // 极低速率：外部来源必被限流
		settings.Burst = 1
		settings.AutoBanThresholdPerMinute = 1
		settings.AutoBanMinutes = 60
	})

	for _, ip := range []string{"172.18.0.1", "127.0.0.1", "10.0.0.5", "192.168.1.10"} {
		// 连打 5 发远超 burst=1：内部来源必须全部放行
		for i := 0; i < 5; i++ {
			c := webProtectionTestCtxIP(ip, "/dashboard")
			require.True(t, TrackWebRequestBegin(c), "trusted IP %s should bypass rate limit", ip)
			TrackWebRequestEnd(c, http.StatusOK)
		}
	}
	// 内部来源绝不能进 banned_ips
	var count int64
	require.NoError(t, model.DB.Model(&model.BannedIP{}).Count(&count).Error)
	assert.Zero(t, count, "trusted internal IPs must never be auto-banned")
}

// TestWebProtectionCGNATIsNotTrustedSource §审查 C2：CGNAT 100.64.0.0/10 是运营商大内网/
// VPN 出口，属外部来源——修复 IsPrivateIP 加 CGNAT 后，绝不能连带把 CGNAT 当信任来源
// 豁免限流/封禁（信任来源用 IsTrustedSourceIP，不含 CGNAT）。
func TestWebProtectionCGNATIsNotTrustedSource(t *testing.T) {
	assert.False(t, isWebProtectionTrustedSource("100.64.0.1"), "CGNAT must not be a trusted source")
	assert.False(t, isWebProtectionTrustedSource("100.127.255.255"), "CGNAT must not be a trusted source")
	// 回归：真正的内部来源仍被信任。
	assert.True(t, isWebProtectionTrustedSource("172.18.0.1"))
	assert.True(t, isWebProtectionTrustedSource("10.0.0.5"))
	assert.True(t, isWebProtectionTrustedSource("127.0.0.1"))
}

// TestWebProtectionExternalIPStillRateLimitedAndBanned 回归：外部来源保持原有
// 防御——超限触发 429 + 自动封禁，防御能力不因内部豁免而削弱。
func TestWebProtectionExternalIPStillRateLimitedAndBanned(t *testing.T) {
	webProtectionTestDB(t)
	useWebProtectionSettings(t, func(settings *operation_setting.WebProtectionSetting) {
		settings.Enabled = true
		settings.AutoBan = true
		settings.LimitPerSecond = 1
		settings.Burst = 1
		settings.AutoBanThresholdPerMinute = 2
		settings.AutoBanMinutes = 60
	})

	external := "203.0.113.9"
	rejected := 0
	for i := 0; i < 6; i++ {
		c := webProtectionTestCtxIP(external, "/dashboard")
		if !TrackWebRequestBegin(c) {
			rejected++
		}
		TrackWebRequestEnd(c, http.StatusTooManyRequests)
	}
	require.GreaterOrEqual(t, rejected, 3, "external IP should be rate limited")
	// 触发自动封禁后，后续请求直接 ip_banned 拒绝
	_, banned := model.IsIPBanned(external)
	assert.True(t, banned, "external misbehaving IP should be auto-banned")
}
