package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupWebProtectionTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&BannedIP{}, &WebRequestLog{}))
	original := DB
	DB = db
	t.Cleanup(func() { DB = original })
}

func seedBannedIPs(t *testing.T, rows []BannedIP) {
	t.Helper()
	for i := range rows {
		require.NoError(t, DB.Create(&rows[i]).Error)
	}
}

// TestUnbanIPsBatch B1-3：批量解封只删除给定 IP；忽略不存在/空串。
func TestUnbanIPsBatch(t *testing.T) {
	setupWebProtectionTest(t)
	seedBannedIPs(t, []BannedIP{
		{IP: "1.1.1.1", Reason: "auto:web_rate_limit", BannedAt: time.Now().Unix()},
		{IP: "2.2.2.2", Reason: "manual:admin_ban", BannedAt: time.Now().Unix()},
		{IP: "3.3.3.3", Reason: "auto:web_rate_limit", BannedAt: time.Now().Unix()},
	})

	require.NoError(t, UnbanIPs([]string{"1.1.1.1", "3.3.3.3", "", "9.9.9.9"}))

	var remain []BannedIP
	require.NoError(t, DB.Find(&remain).Error)
	require.Len(t, remain, 1, "批量解封后应只剩 2.2.2.2")
	assert.Equal(t, "2.2.2.2", remain[0].IP)

	// 空列表 no-op。
	require.NoError(t, UnbanIPs(nil))
	require.NoError(t, UnbanIPs([]string{""}))
	var count int64
	require.NoError(t, DB.Model(&BannedIP{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// TestDeleteExpiredBannedIPsIdempotent B1-3：到期清理幂等；永久(expires_at=0)不被删。
func TestDeleteExpiredBannedIPsIdempotent(t *testing.T) {
	setupWebProtectionTest(t)
	now := time.Now().Unix()
	seedBannedIPs(t, []BannedIP{
		{IP: "expired", ExpiresAt: now - 10},
		{IP: "active", ExpiresAt: now + 3600},
		{IP: "permanent"}, // expires_at=0 永久
	})

	del, err := DeleteExpiredBannedIPs(now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), del, "只删已过期的一条")

	// 幂等：再次执行无变化。
	del2, err := DeleteExpiredBannedIPs(now)
	require.NoError(t, err)
	assert.Zero(t, del2, "幂等：第二次删除 0 条")

	var ips []string
	require.NoError(t, DB.Model(&BannedIP{}).Pluck("ip", &ips).Error)
	assert.ElementsMatch(t, []string{"active", "permanent"}, ips)
}

// TestCountActiveBannedIPs B1-3：生效封禁数 = 未过期 + 永久。
func TestCountActiveBannedIPs(t *testing.T) {
	setupWebProtectionTest(t)
	now := time.Now().Unix()
	seedBannedIPs(t, []BannedIP{
		{IP: "expired", ExpiresAt: now - 10},
		{IP: "active", ExpiresAt: now + 3600},
		{IP: "permanent"},
	})

	count, err := CountActiveBannedIPs(now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "生效封禁 = active + permanent")
}

// TestTodayWebRequestStats B1-3：按 window_start 统计今日请求量/带宽。
func TestTodayWebRequestStats(t *testing.T) {
	setupWebProtectionTest(t)
	now := time.Now().Unix()
	dayStart := now - 3600 // 视为"今日"起点
	old := now - 7200      // 昨日

	require.NoError(t, DB.Create(&WebRequestLog{
		IP: "a", Path: "/", RequestCount: 10, BytesSent: 1000, BytesReceived: 100, WindowStart: now,
	}).Error)
	require.NoError(t, DB.Create(&WebRequestLog{
		IP: "b", Path: "/", RequestCount: 5, BytesSent: 500, BytesReceived: 50, WindowStart: now,
	}).Error)
	require.NoError(t, DB.Create(&WebRequestLog{
		IP: "c", Path: "/", RequestCount: 99, BytesSent: 9999, BytesReceived: 99, WindowStart: old,
	}).Error)

	stat, err := TodayWebRequestStats(dayStart)
	require.NoError(t, err)
	assert.Equal(t, int64(15), stat.RequestCount, "只统计今日两条（10+5）")
	assert.Equal(t, int64(1500), stat.BytesSent)
	assert.Equal(t, int64(150), stat.BytesReceived)
}
