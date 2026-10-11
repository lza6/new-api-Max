package service

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// Batch-13 访客追踪的契约测试。
//
// 这里锁的是用户明确的几条要求：**真实数据、一 IP 一天只算一次、来源可归类**。
// 尤其第一条：去重如果只写在调用方（"记得别重复记"）就会在并发/重启/重放时失守，
// 因此下面同时验证**内存去重**与**数据库唯一键**两层。

func visitorTestContext(ip, path, ua, referer, host string) *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", path, nil)
	c.Request.Header.Set("User-Agent", ua)
	if referer != "" {
		c.Request.Header.Set("Referer", referer)
	}
	if host != "" {
		c.Request.Host = host
	}
	c.Request.RemoteAddr = ip + ":12345"
	return c
}

// resetVisitorTracker 清空 tracker 内存状态，保证用例互不干扰。
//
// 关键：`lastFlush` 置为**当前时间**而不是零值。零值会让 `time.Since(lastFlush)`
// 巨大，从而在 `TrackSiteVisit` 内部就触发一次**异步**落库 —— 测试随即查库时
// 那个 goroutine 可能还没提交，断言就会随机失败（这正是本文件最初踩到的坑）。
// 置为当前时间可获得确定性：写入只由测试显式调用的 `FlushSiteVisitors()` 完成（同步）。
func resetVisitorTracker(t *testing.T) {
	t.Helper()
	tr := siteVisitorTrackerInstance
	tr.mu.Lock()
	tr.day = 0
	tr.dedup = make(map[visitorDedupKey]*visitorRecord, 16)
	tr.pending = make(map[visitorDedupKey]*visitorDelta, 16)
	tr.lastFlush = time.Now()
	tr.overflowCount = 0
	tr.mu.Unlock()
}

const visitorTestUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36"

// 核心要求：**一个 IP 一天只统计一次**，无论它访问多少次、命中多少路径。
func TestTrackSiteVisitDeduplicatesPerIPPerDay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetVisitorTracker(t)
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)

	for i := 0; i < 5; i++ {
		TrackSiteVisit(visitorTestContext("203.0.113.10", "/", visitorTestUA, "", "example.test"))
	}
	TrackSiteVisit(visitorTestContext("203.0.113.10", "/pricing", visitorTestUA, "", "example.test"))

	tr := siteVisitorTrackerInstance
	tr.mu.Lock()
	dedupSize := len(tr.dedup)
	day := tr.day
	tr.mu.Unlock()
	assert.Equal(t, 1, dedupSize, "同一 IP 同一天在去重集合里只能有一行")

	require.NoError(t, FlushSiteVisitors())
	var rows []model.SiteDailyVisitor
	require.NoError(t, model.DB.Where("day = ?", day).Find(&rows).Error)
	require.Len(t, rows, 1, "数据库里同一 (day, ip_hash) 只能有一行")
	assert.Equal(t, 6, rows[0].Hits, "hits 应累加 6 次访问")
	assert.Equal(t, "/", rows[0].Path, "落地路径应是**首次**访问的路径")
}

// 第二个 IP 必须被算成第二个访客（去重不能过头）。
func TestTrackSiteVisitCountsDistinctIPsSeparately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetVisitorTracker(t)
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)

	TrackSiteVisit(visitorTestContext("203.0.113.1", "/", visitorTestUA, "", "example.test"))
	TrackSiteVisit(visitorTestContext("203.0.113.2", "/", visitorTestUA, "", "example.test"))
	TrackSiteVisit(visitorTestContext("203.0.113.3", "/", visitorTestUA, "", "example.test"))
	require.NoError(t, FlushSiteVisitors())

	var n int64
	require.NoError(t, model.DB.Model(&model.SiteDailyVisitor{}).Where("day = ?", VisitorCurrentDay()).Count(&n).Error)
	assert.Equal(t, int64(3), n)
}

// 数据库唯一键是**第二道防线**：即使 tracker 重复提交同一行，也不能出现两条。
// 这条直接绕过内存去重，模拟"重启后重新见过同一个 IP"。
func TestUpsertSiteDailyVisitorsIsIdempotent(t *testing.T) {
	resetVisitorTracker(t)
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)

	day := VisitorCurrentDay()
	row := model.SiteDailyVisitorRow{
		Day: day, IPHash: strings.Repeat("a", 64), FirstSeenAt: 1000, LastSeenAt: 1000,
		Hits: 1, Source: "direct", Path: "/",
	}
	require.NoError(t, model.UpsertSiteDailyVisitors([]model.SiteDailyVisitorRow{row}))

	later := row
	later.LastSeenAt = 2000
	later.Hits = 3
	later.Source = "search"
	later.Path = "/pricing"
	require.NoError(t, model.UpsertSiteDailyVisitors([]model.SiteDailyVisitorRow{later}))

	var rows []model.SiteDailyVisitor
	require.NoError(t, model.DB.Where("day = ?", day).Find(&rows).Error)
	require.Len(t, rows, 1, "唯一键必须挡住重复行")
	assert.Equal(t, 4, rows[0].Hits, "hits 应累加（1 + 3）")
	assert.Equal(t, int64(1000), rows[0].FirstSeenAt, "首次时间不能被后续写入覆盖")
	assert.Equal(t, "direct", rows[0].Source, "来源只记首次，不能被后续覆盖")
}

// 爬虫不能计入访客 —— 否则主页数字虚高且不代表真实访问。
func TestTrackSiteVisitExcludesBots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetVisitorTracker(t)
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)

	TrackSiteVisit(visitorTestContext("203.0.113.50", "/", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "", "example.test"))
	require.NoError(t, FlushSiteVisitors())

	day := VisitorCurrentDay()
	var rows []model.SiteDailyVisitor
	require.NoError(t, model.DB.Where("day = ?", day).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].IsBot, "爬虫行应被标记")

	stats, err := model.GetSiteVisitorStats(day)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.Today, "爬虫不得计入今日访客")
	assert.Equal(t, int64(0), stats.PageViewsToday, "PV 与 UV 同口径排除爬虫")
}

// 各种 UA 判定（含空 UA —— 真实浏览器不会不发）。
func TestIsBotUserAgent(t *testing.T) {
	bots := []string{
		"", "   ",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"curl/8.4.0", "python-requests/2.31.0", "Go-http-client/1.1",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 HeadlessChrome/120.0.0.0",
		"Mozilla/5.0 (compatible; bingbot/2.0)", "AhrefsBot/7.0", "facebookexternalhit/1.1",
	}
	for _, ua := range bots {
		assert.True(t, isBotUserAgent(ua), "应判为机器人：%q", ua)
	}
	humans := []string{
		visitorTestUA,
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36",
	}
	for _, ua := range humans {
		assert.False(t, isBotUserAgent(ua), "不应误判真实浏览器：%q", ua)
	}
}

// 流量来源归类。搜索引擎（含 AI 搜索）优先于其它渠道。
func TestTrafficSourceFromReferrer(t *testing.T) {
	cases := []struct {
		referer string
		site    string
		want    string
	}{
		{"", "example.test", "direct"},
		{"https://example.test/pricing", "example.test", "internal"},
		{"https://www.google.com/search?q=ai+gateway", "example.test", "search"},
		{"https://www.bing.com/search?q=x", "example.test", "search"},
		{"https://www.baidu.com/s?wd=x", "example.test", "search"},
		{"https://chatgpt.com/", "example.test", "search"},
		{"https://www.perplexity.ai/search?q=x", "example.test", "search"},
		{"https://t.co/abc", "example.test", "social"},
		{"https://twitter.com/x/status/1", "example.test", "social"},
		{"https://www.zhihu.com/question/1", "example.test", "social"},
		{"https://news.ycombinator.com/item?id=1", "example.test", "social"},
		{"https://someblog.org/post", "example.test", "referral"},
	}
	for _, tc := range cases {
		host := parseReferrerHost(tc.referer)
		assert.Equal(t, tc.want, trafficSourceFromReferrer(host, tc.site), "referer=%q", tc.referer)
	}
}

// 只统计公开页面文档请求；API/静态资源不参与访客计数。
func TestIsVisitorTrackablePath(t *testing.T) {
	tracked := []string{"/", "/pricing", "/about", "/rankings", "/some/route", "/tool-setup"}
	for _, p := range tracked {
		assert.True(t, isVisitorTrackablePath(p), "应统计：%s", p)
	}
	skipped := []string{
		"/api/status", "/api/home_page_content", "/v1/chat/completions",
		"/static/js/index.js", "/assets/main.css", "/logo.png", "/favicon.ico",
		"/sw.js", "/robots.txt", "/sitemap.xml", "/mj/task", "/pg/foo",
	}
	for _, p := range skipped {
		assert.False(t, isVisitorTrackablePath(p), "不应统计：%s", p)
	}
}

// 非 GET/HEAD 不算"访问页面"。
func TestTrackSiteVisitIgnoresNonGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetVisitorTracker(t)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/pricing", nil)
	c.Request.RemoteAddr = "203.0.113.70:1234"
	c.Request.Header.Set("User-Agent", visitorTestUA)
	TrackSiteVisit(c)

	tr := siteVisitorTrackerInstance
	tr.mu.Lock()
	n := len(tr.dedup)
	tr.mu.Unlock()
	assert.Zero(t, n, "POST 不应计入访客")
}

// IP 匿名化：不可反查、跨日不可关联、畸形输入不写脏数据。
func TestVisitorIPHashAnonymity(t *testing.T) {
	common.CryptoSecret = "unit-test-secret"

	h1 := VisitorIPHash(20261011, "203.0.113.9")
	h2 := VisitorIPHash(20261011, "203.0.113.9")
	assert.Equal(t, h1, h2, "同一天同一 IP 必须稳定")
	assert.Len(t, h1, 64, "应为 64 位十六进制")

	hOtherDay := VisitorIPHash(20261012, "203.0.113.9")
	assert.NotEqual(t, h1, hOtherDay, "跨日必须不可关联")

	assert.NotContains(t, h1, "203.0.113.9", "不得包含 IP 明文")
	assert.Empty(t, VisitorIPHash(20261011, ""), "空 IP 不产生哈希")
	assert.Empty(t, VisitorIPHash(20261011, "not-an-ip"), "畸形 IP 不产生哈希")
}

// 内存上界：达到上界后新 IP 不再缓存（统计变粗，但内存有界）。
func TestTrackSiteVisitRespectsDedupBound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetVisitorTracker(t)

	tr := siteVisitorTrackerInstance
	tr.mu.Lock()
	// 直接灌满去重集合，避免真跑 20 万次。
	day := VisitorCurrentDay()
	tr.day = day
	for i := 0; i < visitorDedupMaxEntries; i++ {
		tr.dedup[visitorDedupKey{day: day, ipHash: "filler" + strconv.Itoa(i)}] = &visitorRecord{hits: 1}
	}
	tr.mu.Unlock()

	TrackSiteVisit(visitorTestContext("203.0.113.99", "/", visitorTestUA, "", "example.test"))

	tr.mu.Lock()
	size := len(tr.dedup)
	overflow := tr.overflowCount
	tr.mu.Unlock()
	assert.Equal(t, visitorDedupMaxEntries, size, "达到上界后集合不得继续增长")
	assert.Equal(t, int64(1), overflow, "溢出应被计数（可观测）")
	resetVisitorTracker(t)
}

// 跨日切换：清空内存集合，避免"昨天的人算进今天"与内存只增不减。
func TestTrackSiteVisitRollsOverOnNewDay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetVisitorTracker(t)

	tr := siteVisitorTrackerInstance
	tr.mu.Lock()
	tr.day = VisitorCurrentDay() - 1
	tr.dedup[visitorDedupKey{day: VisitorCurrentDay() - 1, ipHash: "yesterday"}] = &visitorRecord{hits: 5}
	tr.mu.Unlock()

	TrackSiteVisit(visitorTestContext("203.0.113.11", "/", visitorTestUA, "", "example.test"))

	tr.mu.Lock()
	day := tr.day
	size := len(tr.dedup)
	tr.mu.Unlock()
	assert.Equal(t, VisitorCurrentDay(), day, "应切换到今天")
	assert.Equal(t, 1, size, "跨日后旧集合必须被清空")
	require.NoError(t, FlushSiteVisitors())

	// 同一个 IP 在今天应能再算一次（昨天的记录不影响今天）。
	_ = model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{})
	TrackSiteVisit(visitorTestContext("203.0.113.11", "/", visitorTestUA, "", "example.test"))
	require.NoError(t, FlushSiteVisitors())
	var n int64
	require.NoError(t, model.DB.Model(&model.SiteDailyVisitor{}).Where("day = ? AND is_bot = ?", VisitorCurrentDay(), false).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

// 统计聚合：今日 / 近 7 天 / 近 30 天 / 累计 / PV，且排除爬虫。
func TestGetSiteVisitorStatsWindows(t *testing.T) {
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)
	today := VisitorCurrentDay()

	insert := func(day int, ipHash string, hits int, isBot bool) {
		require.NoError(t, model.UpsertSiteDailyVisitors([]model.SiteDailyVisitorRow{
			{Day: day, IPHash: ipHash, FirstSeenAt: 1, LastSeenAt: 1, Hits: hits,
				Source: "direct", Path: "/", IsBot: isBot},
		}))
	}
	insert(today, "h1", 3, false)                // 今天
	insert(today, "h2", 1, false)                // 今天
	insert(today, "hbot", 9, true)               // 今天（爬虫，应排除）
	insert(shiftDay(today, -3), "h3", 2, false)  // 3 天前
	insert(shiftDay(today, -10), "h4", 5, false) // 10 天前
	insert(shiftDay(today, -29), "h5", 4, false) // 29 天前（含在 30 天内）
	insert(shiftDay(today, -60), "h6", 7, false) // 60 天前（仅累计）

	stats, err := model.GetSiteVisitorStats(today)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.Today, "今日 UV=2（爬虫排除）")
	assert.Equal(t, int64(3), stats.Last7d, "近 7 天 = 今天 2 + 3 天前 1")
	assert.Equal(t, int64(5), stats.Last30d, "近 30 天 = h1,h2,h3,h4,h5")
	assert.Equal(t, int64(6), stats.Total, "累计 = 全部非爬虫（含 60 天前的 h6）")
	// PV 与 UV 同口径：都排除爬虫（见 model.GetSiteVisitorStats 的口径说明）。
	assert.Equal(t, int64(3+1), stats.PageViewsToday, "今日 PV（不含爬虫）")
	assert.Equal(t, int64(4+2+5+4), stats.PageViewsLast30d, "近 30 天 PV")
	assert.Equal(t, int64(4+2+5+4+7), stats.PageViewsTotal, "累计 PV（含 60 天前）")
}

// 「近 N 天」必须跨月正确 —— day 是 YYYYMMDD，直接做整数减法会算错。
func TestDayOffsetHandlesMonthBoundary(t *testing.T) {
	assert.Equal(t, 20260228, shiftDay(20260301, -1))
	assert.Equal(t, 20260301, shiftDay(20260228, 1))
	assert.Equal(t, 20251231, shiftDay(20260101, -1))
	assert.Equal(t, 20260101, shiftDay(20251231, 1))
	assert.Equal(t, 20260228, model.DayOffset(20260301, -1))
}

// 流量来源分布 + internal 并入 direct。
func TestGetTrafficSourceCountsMergesInternalIntoDirect(t *testing.T) {
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)
	today := VisitorCurrentDay()

	rows := []model.SiteDailyVisitorRow{
		{Day: today, IPHash: "s1", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "search", Path: "/"},
		{Day: today, IPHash: "s2", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "search", Path: "/"},
		{Day: today, IPHash: "d1", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "direct", Path: "/"},
		{Day: today, IPHash: "i1", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "internal", Path: "/"},
		{Day: today, IPHash: "i2", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "internal", Path: "/"},
		{Day: today, IPHash: "r1", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "referral", Path: "/"},
		{Day: today, IPHash: "bot1", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "search", Path: "/", IsBot: true},
	}
	require.NoError(t, model.UpsertSiteDailyVisitors(rows))

	out, err := model.GetTrafficSourceCounts(today, 30)
	require.NoError(t, err)
	merged := mergeInternalIntoDirect(out)

	got := map[string]int64{}
	for _, r := range merged {
		got[r.Source] = r.Count
	}
	assert.Equal(t, int64(2), got["search"], "爬虫不计入来源")
	assert.Equal(t, int64(3), got["direct"], "direct 1 + internal 2")
	assert.Equal(t, int64(1), got["referral"])
	_, hasInternal := got["internal"]
	assert.False(t, hasInternal, "internal 应被并入 direct，不再单列")
	// 降序：direct(3) 在前
	assert.Equal(t, "direct", merged[0].Source)
}

// 注册用户总数（只返回计数）。
func TestCountRegisteredUsers(t *testing.T) {
	before, err := model.CountRegisteredUsers()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, before, int64(0))
}

// 保留策略：早于 cutoff 的记录必须被删掉（本表逐日累积，必须有清理）。
func TestDeleteSiteDailyVisitorsBefore(t *testing.T) {
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)
	today := VisitorCurrentDay()
	old := shiftDay(today, -500)

	require.NoError(t, model.UpsertSiteDailyVisitors([]model.SiteDailyVisitorRow{
		{Day: old, IPHash: "old1", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "direct", Path: "/"},
		{Day: today, IPHash: "new1", FirstSeenAt: 1, LastSeenAt: 1, Hits: 1, Source: "direct", Path: "/"},
	}))

	deleted, err := model.DeleteSiteDailyVisitorsBefore(shiftDay(today, -400))
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	var n int64
	require.NoError(t, model.DB.Model(&model.SiteDailyVisitor{}).Where("day = ?", today).Count(&n).Error)
	assert.Equal(t, int64(1), n, "近期记录必须保留")
}

// 多次 flush 之间的回访必须被**累加**而不是被覆盖或丢失。
//
// 这条锁的是一个真实缺陷：pending 起初存的是「累计 hits」并复用 visitorRecord，
// 导致 ① flush 后同一 IP 的新访问从不再入队（增量丢失）；② 若简单重新入队又会
// 因为 SQL 侧是 `hits = hits + excluded.hits` 而把旧值重复累加（计数虚高）。
// 修法是把 pending 改成**只存增量**的 visitorDelta。此用例同时覆盖两种走法。
func TestTrackSiteVisitAccumulatesAcrossFlushes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetVisitorTracker(t)
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)
	day := VisitorCurrentDay()

	visit := func(n int) {
		for i := 0; i < n; i++ {
			TrackSiteVisit(visitorTestContext("198.51.100.7", "/", visitorTestUA, "", "example.test"))
		}
	}
	// 第一次 flush 前访问 2 次
	visit(2)
	require.NoError(t, FlushSiteVisitors())

	var first model.SiteDailyVisitor
	require.NoError(t, model.DB.Where("day = ?", day).First(&first).Error)
	assert.Equal(t, 2, first.Hits, "首次 flush 应写入 2 次")

	// flush 之后再访问 3 次：必须累加而不是停止计数
	visit(3)
	require.NoError(t, FlushSiteVisitors())

	var rows []model.SiteDailyVisitor
	require.NoError(t, model.DB.Where("day = ?", day).Find(&rows).Error)
	require.Len(t, rows, 1, "仍然是同一个访客，不能新增行")
	assert.Equal(t, 5, rows[0].Hits, "跨 flush 的访问次数必须累加：2 + 3 = 5")
	assert.Equal(t, first.FirstSeenAt, rows[0].FirstSeenAt, "首次时间保持不变")
}

// 缓冲区满时丢弃**增量**（而非整行），且不产生错误行。
func TestAddDeltaDropsIncrementWhenBufferFull(t *testing.T) {
	resetVisitorTracker(t)
	tr := siteVisitorTrackerInstance
	tr.mu.Lock()
	day := VisitorCurrentDay()
	tr.day = day
	// 灌满 pending
	for i := 0; i < visitorBufferMaxEntries; i++ {
		tr.pending[visitorDedupKey{day: day, ipHash: "p" + strconv.Itoa(i)}] = &visitorDelta{hits: 1}
	}
	tr.mu.Unlock()

	key := visitorDedupKey{day: day, ipHash: "overflowing"}
	tr.mu.Lock()
	tr.addDeltaLocked(key, 1, 123)
	size := len(tr.pending)
	_, exists := tr.pending[key]
	rows := tr.collectRowsLocked()
	tr.mu.Unlock()

	assert.Equal(t, visitorBufferMaxEntries, size, "缓冲区不得越界")
	assert.False(t, exists, "满时不缓存新键")
	// collectRows 不该为不存在的 dedup 行产出数据
	for _, r := range rows {
		assert.NotEqual(t, key.ipHash, r.IPHash)
	}
	resetVisitorTracker(t)
}

// collectRowsLocked 在 dedup 里没有对应行时**不产出该行**
// （否则会在库里留下字段全空的错误访客记录）。
func TestCollectRowsSkipsOrphanDeltas(t *testing.T) {
	resetVisitorTracker(t)
	tr := siteVisitorTrackerInstance
	tr.mu.Lock()
	day := VisitorCurrentDay()
	tr.day = day
	tr.pending[visitorDedupKey{day: day, ipHash: "orphan"}] = &visitorDelta{hits: 3, lastSeenAt: 1}
	rows := tr.collectRowsLocked()
	tr.mu.Unlock()
	assert.Empty(t, rows, "没有 dedup 状态时不得产出访客行")
}

// is_bot 必须是**累积语义**：只有该 IP 当天**全部**访问都是爬虫，才标记为爬虫。
//
// 这条锁的是一个实测到的真实缺陷：原先 is_bot 只在首次访问时写入，
// 于是「爬虫先来、真人后来」时整条记录被当成爬虫，真人当天**完全不被计入**。
// 对用户要求的「数据要真实」来说这是直接的错误。
func TestTrackSiteVisitBotFlagIsCumulative(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const botUA = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

	t.Run("爬虫先到，真人后到 -> 该行不算爬虫", func(t *testing.T) {
		resetVisitorTracker(t)
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)
		day := VisitorCurrentDay()
		// 先把该 IP 标记为"已落库的爬虫行"（模拟爬虫先访问并 flush 过）
		require.NoError(t, model.UpsertSiteDailyVisitors([]model.SiteDailyVisitorRow{
			{Day: day, IPHash: mustHash(t, day, "203.0.113.200"), FirstSeenAt: 1, LastSeenAt: 1,
				Hits: 1, Source: "direct", Path: "/", IsBot: true},
		}))
		// 清掉内存，模拟进程重启后再来真人
		resetVisitorTracker(t)
		TrackSiteVisit(visitorTestContext("203.0.113.200", "/", visitorTestUA, "", "example.test"))
		require.NoError(t, FlushSiteVisitors())

		var row model.SiteDailyVisitor
		require.NoError(t, model.DB.Where("day = ?", day).First(&row).Error)
		assert.False(t, row.IsBot, "该 IP 当天出现过真人访问，就不能再算作纯爬虫")
	})

	t.Run("真人先到，爬虫后到 -> 该行仍算真人", func(t *testing.T) {
		resetVisitorTracker(t)
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)
		day := VisitorCurrentDay()
		TrackSiteVisit(visitorTestContext("203.0.113.201", "/", visitorTestUA, "", "example.test"))
		require.NoError(t, FlushSiteVisitors())
		TrackSiteVisit(visitorTestContext("203.0.113.201", "/", botUA, "", "example.test"))
		require.NoError(t, FlushSiteVisitors())

		var row model.SiteDailyVisitor
		require.NoError(t, model.DB.Where("day = ?", day).First(&row).Error)
		assert.False(t, row.IsBot, "真人先访问过，后续爬虫不能把这一行变成爬虫")
	})

	t.Run("全程只有爬虫 -> 仍算爬虫", func(t *testing.T) {
		resetVisitorTracker(t)
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)
		day := VisitorCurrentDay()
		TrackSiteVisit(visitorTestContext("203.0.113.202", "/", botUA, "", "example.test"))
		require.NoError(t, FlushSiteVisitors())
		TrackSiteVisit(visitorTestContext("203.0.113.202", "/", botUA, "", "example.test"))
		require.NoError(t, FlushSiteVisitors())

		var row model.SiteDailyVisitor
		require.NoError(t, model.DB.Where("day = ?", day).First(&row).Error)
		assert.True(t, row.IsBot, "全程爬虫必须仍被标记为爬虫（否则 UV 会虚高）")
		stats, err := model.GetSiteVisitorStats(day)
		require.NoError(t, err)
		assert.Equal(t, int64(0), stats.Today, "纯爬虫行的 IP 不计入访客")
	})
}

// 同一批里出现重复 (day, ip_hash) 时必须先合并 —— 否则 SQLite 的
// ON CONFLICT DO UPDATE 会在同一语句内二次匹配同一键而报错。
func TestUpsertSiteDailyVisitorsDedupesWithinBatch(t *testing.T) {
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SiteDailyVisitor{}).Error)
	day := VisitorCurrentDay()
	hash := mustHash(t, day, "203.0.113.210")

	err := model.UpsertSiteDailyVisitors([]model.SiteDailyVisitorRow{
		{Day: day, IPHash: hash, FirstSeenAt: 5, LastSeenAt: 5, Hits: 2, Source: "search", Path: "/", IsBot: true},
		{Day: day, IPHash: hash, FirstSeenAt: 3, LastSeenAt: 9, Hits: 4, Source: "direct", Path: "/x", IsBot: false},
	})
	require.NoError(t, err, "同批重复键必须被合并而不是报错")

	var rows []model.SiteDailyVisitor
	require.NoError(t, model.DB.Where("day = ?", day).Find(&rows).Error)
	require.Len(t, rows, 1, "同批重复键只能落一行")
	assert.Equal(t, 6, rows[0].Hits, "hits 应合并累加 2+4")
	assert.Equal(t, int64(3), rows[0].FirstSeenAt, "取最早的 first_seen_at")
	assert.Equal(t, int64(9), rows[0].LastSeenAt, "取最晚的 last_seen_at")
	assert.False(t, rows[0].IsBot, "本批含非爬虫 → AND 后应为 false")
}

// mustHash 生成与运行期一致的匿名化哈希（用服务暴露的函数，避免测试自己实现一遍）。
func mustHash(t *testing.T, day int, ip string) string {
	t.Helper()
	h := VisitorIPHash(day, ip)
	require.NotEmpty(t, h)
	return h
}
