package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// useIsolatedVisitorDB 把 model.DB / model.LOG_DB 指向一个独立的内存库，并重置
// 进程级统计缓存。
//
// 为什么必须自持数据库：controller 包里有若干用例会替换或关闭全局 DB
// （见 login_audit_test.go 里 useIsolatedAuditDB 的注释）。共用 model.DB 的用例
// 在全量运行时可能拿到 `sql: database is closed` 而失败，**单独跑却通过** ——
// 这种假失败会污染回归判定（本项目已有「既有噪声清单」就是被它撑起来的）。
// 本文件的两个端点用例因此自持数据库，任何执行顺序下结论都成立。
func useIsolatedVisitorDB(t *testing.T) {
	t.Helper()
	prevDB, prevLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.SiteDailyVisitor{}, &model.User{}, &model.Log{},
	))
	model.DB, model.LOG_DB = db, db
	// 统计缓存是**进程级**的：换了库就必须重置，否则会读到上一个用例的结果。
	service.ResetSiteVisitorCachesForTest()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLogDB
		service.ResetSiteVisitorCachesForTest()
		_ = sqlDB.Close()
	})
}

// Batch-13 站点统计端点的契约测试。
//
// 用户明确要求「数据要真实、来源要有百分比」。这里锁两件事：
//  1. 响应**只含聚合量**，不得出现 IP、ip_hash、用户标识等任何可定位到个人的字段；
//  2. 结构完整 —— 前端渲染任何一项都不能拿到 undefined（否则主页会显示空数字）。
func TestGetSiteVisitorSummaryShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useIsolatedVisitorDB(t)

	today := serviceVisitorCurrentDay()
	require.NoError(t, model.UpsertSiteDailyVisitors([]model.SiteDailyVisitorRow{
		{Day: today, IPHash: "shapehash1", FirstSeenAt: 1, LastSeenAt: 1, Hits: 2, Source: "search", Path: "/"},
		{Day: today, IPHash: "shapehash2", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "direct", Path: "/"},
		{Day: today, IPHash: "shapehash3", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "referral", Path: "/"},
		{Day: today, IPHash: "shapehashbot", FirstSeenAt: 1, LastSeenAt: 1, Hits: 99, Source: "search", Path: "/", IsBot: true},
	}))

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/site/visitors", nil)
	GetSiteVisitorSummary(c)

	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Visitors struct {
				Today            int64 `json:"today"`
				Last7d           int64 `json:"last_7d"`
				Last30d          int64 `json:"last_30d"`
				Total            int64 `json:"total"`
				PageViewsToday   int64 `json:"page_views_today"`
				PageViewsTotal   int64 `json:"page_views_total"`
				PageViewsLast30d int64 `json:"page_views_last_30d"`
			} `json:"visitors"`
			RegisteredUsers int64 `json:"registered_users"`
			ActiveUsers24h  int64 `json:"active_users_24h"`
			ActiveUsers30d  int64 `json:"active_users_30d"`
			Sources         []struct {
				Source string `json:"source"`
				Count  int64  `json:"count"`
			} `json:"sources"`
			GeneratedAt int64 `json:"generated_at"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	d := payload.Data
	// 3 个真实访客 + 1 个爬虫被排除
	assert.Equal(t, int64(3), d.Visitors.Today, "爬虫不得计入今日访客")
	assert.Equal(t, int64(3), d.Visitors.Last7d)
	assert.Equal(t, int64(3), d.Visitors.Last30d)
	assert.Equal(t, int64(3), d.Visitors.Total)
	assert.Equal(t, int64(4), d.Visitors.PageViewsToday, "PV 与 UV 同口径（2+1+1）")
	assert.Greater(t, d.GeneratedAt, int64(0), "必须给出数据生成时刻，便于前端显示新鲜度")

	// 结构完整性：Sources 必须存在每一项（前端要算百分比，缺项会得到 NaN）
	require.NotEmpty(t, d.Sources)
	var total int64
	for _, s := range d.Sources {
		assert.NotEmpty(t, s.Source)
		assert.Greater(t, s.Count, int64(0))
		total += s.Count
	}
	assert.Equal(t, int64(3), total, "来源分布合计应等于 3 个真实访客")

	// 隐私：响应体不得泄露任何可定位到个人的标识
	for _, forbidden := range []string{"ip_hash", "shapehash", "192.168", "203.0.113", "user_id", "username"} {
		assert.NotContains(t, body, forbidden, "公开统计不得泄露 %s", forbidden)
	}
}

// 来源分布必须带足够信息让前端算百分比：返回计数即可（前端除以合计）。
// 但空数据时必须返回**空数组而不是 null**，否则前端 `.map` 会抛错。
func TestGetSiteVisitorSummaryEmptyData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useIsolatedVisitorDB(t)

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/site/visitors", nil)
	GetSiteVisitorSummary(c)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"sources"`, "无数据时也必须给出 sources 字段")
	assert.NotContains(t, response.Body.String(), `"sources":null`, "不得返回 null（前端会崩）")

	var payload struct {
		Data struct {
			Visitors struct {
				Today int64 `json:"today"`
			} `json:"visitors"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, int64(0), payload.Data.Visitors.Today, "无访客时必须是 0 而不是报错")
}

// serviceVisitorCurrentDay 复用服务的日期口径，避免测试自己算错日期。
func serviceVisitorCurrentDay() int {
	return visitorDayOf(common.GetTimestamp())
}

// visitorDayOf 由 unix 秒得到 YYYYMMDD（与 service.VisitorCurrentDay 同口径）。
func visitorDayOf(unix int64) int {
	t := time.Unix(unix, 0)
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}
