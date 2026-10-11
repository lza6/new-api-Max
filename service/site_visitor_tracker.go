package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
)

// 站点访客追踪（Batch-13）。
//
// ## 目标
//
// 主页要显示「今日访客 / 总访问量 / 近 30 天访客」，且**一个 IP 一天只算一次**。
// 计数必须真实，不能因为 SPA 一次访问发多个请求就重复计数。
//
// ## 为什么不是「每个请求写一次库」
//
// 首页一次访问会产生十几个请求（壳、JS、CSS、字体、`/api/status`…）。逐请求落库
// 会把这个纯粹用于展示的统计变成最大的写库来源之一，得不偿失。因此这里是
// **内存去重 → 定期批量 upsert**：
//   - 内存里的 key 是 `(day, ipHash)`，与数据库唯一键同构，天然完成「一天一次」去重；
//   - 同一 (day, ipHash) 在内存里只保留**一行**（累加 hits），不是每请求一行；
//   - 每 30 秒把缓冲区批量 upsert 一次，冲突时数据库只累加 hits。
//
// ## 内存上界（这是本文件最重要的一条纪律）
//
// 内存里的去重集合大小 = **当天独立访客数**，在极端情况下（扫描器换源 IP 洪泛）
// 可以无限增长。因此有两道硬闸：
//  1. `visitorDedupMaxEntries`：去重集合条目上限。达到后新 IP **不再缓存**，
//     但计入 `overflowCount` 并照旧累计 hits —— 统计口径变粗，但内存不涨；
//  2. `visitorBufferMaxEntries`：待落库缓冲区条目上限（正常不会触发，因为
//     去重集合本身就限制了行数）。
//
// 另有一条随日期自动切换：跨自然日时整个去重集合与缓冲区被清空（见 rolloverLocked），
// 保证不会出现「昨天的人算进今天」以及集合永远不释放的问题。
const (
	visitorFlushEvery       = 30 * time.Second
	visitorDedupMaxEntries  = 200_000
	visitorBufferMaxEntries = 50_000
	// visitorStatsCacheTTL 是公开统计端点的短缓存：主页统计不需要秒级实时，
	// 而短缓存能把「匿名爬虫反复请求统计接口」变成一次查询。
	visitorStatsCacheTTL = 60 * time.Second
	// visitorRetentionDays 是访客明细的保留天数。本表逐日累积，必须有保留策略。
	visitorRetentionDays = 400
)

// visitorDedupKey 是内存去重键（与数据库唯一键同构）。
type visitorDedupKey struct {
	day    int
	ipHash string
}

// visitorRecord 是内存中**当天该 IP 的累计状态**（去重集合用）。
type visitorRecord struct {
	firstSeenAt  int64
	lastSeenAt   int64
	hits         int
	source       string
	referrerHost string
	path         string
	isBot        bool
}

// visitorDelta 是**待落库的增量**（pending 用）。
//
// 关键设计：pending 存的是**自上次 flush 以来新增的 hits**，而不是累计值。
// 数据库侧写入用的是 `hits = hits + excluded.hits`，所以只有传增量才不会重复累加：
//   - 传累计值 → 第 N 次 flush 会把前 N-1 次的数量再累加一遍（计数虚高）；
//   - 传增量   → 每次 flush 只补上"上次没说过的部分"，总和恰好正确。
//
// 这也是"同一 IP 在多次 flush 之间持续回访"能被正确统计的原因。
type visitorDelta struct {
	hits       int
	lastSeenAt int64
}

type siteVisitorTracker struct {
	mu        sync.Mutex
	day       int
	dedup     map[visitorDedupKey]*visitorRecord
	pending   map[visitorDedupKey]*visitorDelta
	lastFlush time.Time
	// overflowCount 记录「因去重集合满而未缓存」的请求数，仅用于观测。
	overflowCount int64
}

var siteVisitorTrackerInstance = &siteVisitorTracker{
	dedup:   make(map[visitorDedupKey]*visitorRecord, 256),
	pending: make(map[visitorDedupKey]*visitorDelta, 256),
}

// siteVisitorLoop 是落库/清理常驻循环（复用既有 backgroundLoop：可停止、可观测）。
var siteVisitorLoop backgroundLoop

// VisitorTrackerDedupMaxEntries 暴露上界供测试断言。
func VisitorTrackerDedupMaxEntries() int { return visitorDedupMaxEntries }

// VisitorTrackerOverflowCount 暴露溢出计数供测试与运维观测。
func VisitorTrackerOverflowCount() int64 {
	t := siteVisitorTrackerInstance
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.overflowCount
}

// VisitorCurrentDay 返回当前自然日（YYYYMMDD）。
func VisitorCurrentDay() int {
	n := time.Now()
	return n.Year()*10000 + int(n.Month())*100 + n.Day()
}

// VisitorIPHash 把客户端 IP 匿名化为可入库的哈希。
//
// `HMAC-SHA256(CryptoSecret, day + "|" + ip)`：
//   - 带 day 前缀 → 跨日不可关联，本表无法用于长期追踪某个 IP；
//   - 带密钥 → IPv4 只有 2^32 种取值，裸 SHA256 可字典爆破，HMAC 密钥挡住了这点。
//
// 返回空串表示 IP 不可用（调用方应跳过记录，而不是写入一条空哈希的脏数据）。
func VisitorIPHash(day int, ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	if net.ParseIP(ip) == nil {
		return ""
	}
	return common.HmacSha256(fmt.Sprintf("%d|%s", day, ip), common.CryptoSecret)
}

// trafficSourceFromReferrer 把 Referer 归类为流量来源。
//
// 判定顺序有意为「越具体的渠道越优先」：搜索引擎 > 社交 > 外部推荐 > 直接。
// 只看**首次**访问的来源（由调用方保证）。
//
// 站内跳转（referer 主机 == 本站主机）归为 internal，并在统计时与 direct 一起
// 视为「直接访问」—— 因为站内跳转不代表新来源。
func trafficSourceFromReferrer(referrerHost, siteHost string) string {
	h := strings.ToLower(strings.TrimSpace(referrerHost))
	if h == "" {
		return "direct"
	}
	if siteHost != "" && h == strings.ToLower(siteHost) {
		return "internal"
	}
	// 搜索引擎
	for _, s := range []string{"google.", "bing.", "baidu.", "duckduckgo.", "yandex.", "so.com", "sogou.", "naver.", "yahoo.", "ecosia.", "brave.com", "perplexity.ai", "chatgpt.com", "openai.com", "copilot.microsoft.com", "gemini.google.com", "claude.ai"} {
		if strings.Contains(h, s) {
			return "search"
		}
	}
	// 社交 / 内容平台
	for _, s := range []string{"t.co", "twitter.", "x.com", "facebook.", "fb.com", "reddit.", "linkedin.", "lnkd.in", "weibo.", "zhihu.", "juejin.", "csdn.net", "bilibili.", "t.me", "telegram.", "discord.", "youtube.", "medium.com", "v2ex.com", "hacker news", "news.ycombinator.com", "producthunt.com", "github.com", "gitee.com"} {
		if strings.Contains(h, s) {
			return "social"
		}
	}
	return "referral"
}

// parseReferrerHost 从 Referer 提取主机名（去掉端口、路径、query）。
// 解析失败返回空串（按直接访问处理，不因一个畸形头丢掉整次统计）。
func parseReferrerHost(referrer string) string {
	referrer = strings.TrimSpace(referrer)
	if referrer == "" {
		return ""
	}
	u, err := url.Parse(referrer)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	if len(host) > 255 {
		host = host[:255]
	}
	return host
}

// isBotUserAgent 判定 UA 是否为爬虫/自动化。
//
// 统计**必须**排除它们：把 Googlebot、SEO 审计工具、监控探针算进「今日访客」，
// 会让主页数字虚高且完全不代表真实访问量 —— 而这次统计的全部意义就是「真实」。
//
// 判定策略偏保守（宁可漏判也不误伤真实浏览器）：只匹配公认的爬虫/工具标识。
// 空 UA 视为机器人（真实浏览器不会不发 UA）。
func isBotUserAgent(ua string) bool {
	ua = strings.ToLower(strings.TrimSpace(ua))
	if ua == "" {
		return true
	}
	markers := []string{
		"bot", "crawler", "spider", "crawl", "slurp", "baiduspider", "yandex",
		"facebookexternalhit", "twitterbot", "telegrambot", "whatsapp", "slackbot",
		"discordbot", "embedly", "quora link preview", "pinterest", "bitlybot",
		"skypeuripreview", "nuzzel", "vkShare", "w3c_validator", "redditbot",
		"applebot", "semrush", "ahrefs", "mj12bot", "dotbot", "petalbot", "bytespider",
		"python-requests", "python-urllib", "go-http-client", "curl/", "wget",
		"axios/", "node-fetch", "okhttp", "java/", "libwww-perl", "scrapy",
		"headlesschrome", "phantomjs", "puppeteer", "playwright", "lighthouse",
		"chrome-lighthouse", "pagespeed", "gtmetrix", "pingdom", "uptimerobot",
		"statuscake", "nagios", "zabbix", "prometheus", "kube-probe",
	}
	for _, m := range markers {
		if strings.Contains(ua, m) {
			return true
		}
	}
	return false
}

// visitorTrackPaths 是**会被计入访客统计**的路径范围。
//
// 只统计公开页面，不统计 API/静态资源：SPA 的每个页面只有 1 个 HTML 文档请求，
// 其余是资源请求 —— 把人头统计挂在文档请求上，天然就是「一次访问一次」，
// 不需要额外去重逻辑。
func isVisitorTrackablePath(path string) bool {
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/v1/") ||
		strings.HasPrefix(path, "/assets") || strings.HasPrefix(path, "/static/") ||
		strings.HasPrefix(path, "/mj") || strings.HasPrefix(path, "/pg") {
		return false
	}
	// 带扩展名的静态资源不统计
	if i := strings.LastIndex(path, "/"); i >= 0 {
		if seg := path[i+1:]; strings.Contains(seg, ".") {
			return false
		}
	}
	return true
}

// TrackSiteVisit 记录一次站点访问（由公开页面中间件调用）。
//
// 幂等且**绝不阻塞请求**：全部是内存操作，加锁范围极小。
func TrackSiteVisit(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	path := c.Request.URL.Path
	if !isVisitorTrackablePath(path) {
		return
	}
	// 只统计 GET/HEAD：POST 等不是「访问页面」。
	if m := c.Request.Method; m != "GET" && m != "HEAD" {
		return
	}

	day := VisitorCurrentDay()
	ipHash := VisitorIPHash(day, c.ClientIP())
	if ipHash == "" {
		return
	}

	now := common.GetTimestamp()
	referrerHost := parseReferrerHost(c.Request.Referer())
	siteHost := c.Request.Host
	if i := strings.Index(siteHost, ":"); i > 0 {
		siteHost = siteHost[:i]
	}

	t := siteVisitorTrackerInstance
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.day != day {
		t.rolloverLocked(day)
	}

	key := visitorDedupKey{day: day, ipHash: ipHash}
	if rec, ok := t.dedup[key]; ok {
		// 当天已见过：累加累计状态，并在 pending 里累加**增量**（见 visitorDelta 的说明）。
		rec.hits++
		rec.lastSeenAt = now
		// is_bot 用**累积语义**（AND）：只要该 IP 当天出现过一次非爬虫访问，
		// 它就不再是「纯爬虫」，必须计入访客。只按首次 UA 判定会让
		// 「爬虫先来、真人后来」的整条记录被当成爬虫而漏统计（实测踩到过）。
		rec.isBot = rec.isBot && isBotUserAgent(c.Request.UserAgent())
		t.addDeltaLocked(key, 1, now)
		return
	}

	rec := &visitorRecord{
		firstSeenAt:  now,
		lastSeenAt:   now,
		hits:         1,
		source:       trafficSourceFromReferrer(referrerHost, siteHost),
		referrerHost: referrerHost,
		path:         path,
		isBot:        isBotUserAgent(c.Request.UserAgent()),
	}

	// 内存上界：达到后不再缓存新 IP（统计口径变粗，但内存有界，见文件头说明）。
	if len(t.dedup) >= visitorDedupMaxEntries {
		t.overflowCount++
		return
	}
	t.dedup[key] = rec
	t.addDeltaLocked(key, 1, now)

	if time.Since(t.lastFlush) >= visitorFlushEvery {
		t.flushLocked()
	}
}

// addDeltaLocked 在待落库缓冲区里累加一个增量。调用方必须持锁。
//
// 缓冲区满时**丢弃增量**而不是丢整行：该 IP 的行已经（或即将）写入数据库，
// 只损失这次的回访计数，不会产生一条错误的访客记录。反过来若为了保住增量而
// 让 pending 无界增长，就会把内存上限那道闸拆掉 —— 这里优先保证内存有界。
func (t *siteVisitorTracker) addDeltaLocked(key visitorDedupKey, hits int, lastSeen int64) {
	if d, ok := t.pending[key]; ok {
		d.hits += hits
		if lastSeen > d.lastSeenAt {
			d.lastSeenAt = lastSeen
		}
		return
	}
	if len(t.pending) >= visitorBufferMaxEntries {
		return
	}
	t.pending[key] = &visitorDelta{hits: hits, lastSeenAt: lastSeen}
}

// rolloverLocked 跨自然日时清空去重集合与缓冲区。
//
// 必须同时清空 dedup：否则前一天的 IP 会一直占着内存，集合只增不减。
// 清空前先 flush，保证跨日那一刻的计数不丢。
func (t *siteVisitorTracker) rolloverLocked(day int) {
	t.flushLocked()
	t.day = day
	t.dedup = make(map[visitorDedupKey]*visitorRecord, 256)
	t.pending = make(map[visitorDedupKey]*visitorDelta, 256)
	t.overflowCount = 0
}

// collectRowsLocked 把待落库缓冲区转成写库行，并清空缓冲区。调用方必须持锁。
//
// `hits` 取的是**增量**，但「首次」字段（first_seen_at/source/referrer/path/is_bot）
// 必须来自 dedup 里的累计状态 —— pending 只记增量，没有这些字段。
// 若 dedup 里找不到对应行（理论上不会发生：两者同锁同生命周期），
// 退化为不写该行，避免在库里留下一条字段全空的错误访客记录。
func (t *siteVisitorTracker) collectRowsLocked() []model.SiteDailyVisitorRow {
	if len(t.pending) == 0 {
		return nil
	}
	rows := make([]model.SiteDailyVisitorRow, 0, len(t.pending))
	for k, d := range t.pending {
		if d.hits <= 0 {
			continue
		}
		rec, ok := t.dedup[k]
		if !ok {
			continue
		}
		rows = append(rows, model.SiteDailyVisitorRow{
			Day:          k.day,
			IPHash:       k.ipHash,
			FirstSeenAt:  rec.firstSeenAt,
			LastSeenAt:   d.lastSeenAt,
			Hits:         d.hits,
			Source:       rec.source,
			ReferrerHost: rec.referrerHost,
			Path:         rec.path,
			IsBot:        rec.isBot,
		})
	}
	// 缓冲区整体替换为新的 map：已落库的增量不再需要重写。
	t.pending = make(map[visitorDedupKey]*visitorDelta, 256)
	return rows
}

// flushLocked 把待落库缓冲区快照出去并异步写库。调用方必须持锁。
func (t *siteVisitorTracker) flushLocked() {
	t.lastFlush = time.Now()
	rows := t.collectRowsLocked()
	if len(rows) == 0 {
		return
	}
	go func() {
		if err := model.UpsertSiteDailyVisitors(rows); err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("record site visitors failed: %v", err))
		}
	}()
}

// FlushSiteVisitors 强制落库（维护任务/优雅退出/测试用）。同步执行、返回真实错误。
func FlushSiteVisitors() error {
	t := siteVisitorTrackerInstance
	t.mu.Lock()
	rows := t.collectRowsLocked()
	t.lastFlush = time.Now()
	t.mu.Unlock()

	if len(rows) == 0 {
		return nil
	}
	return model.UpsertSiteDailyVisitors(rows)
}

// RunSiteVisitorMaintenance 维护任务：落库 + 保留策略清理。
// 幂等；失败仅告警不阻断（沿用 web 防护维护任务的既有约定）。
func RunSiteVisitorMaintenance() error {
	if err := FlushSiteVisitors(); err != nil {
		return err
	}
	cutoff := VisitorCurrentDay()
	for i := 0; i < visitorRetentionDays; i++ {
		cutoff = shiftDay(cutoff, -1)
	}
	if _, err := model.DeleteSiteDailyVisitorsBefore(cutoff); err != nil {
		return err
	}
	return nil
}

// shiftDay 对 YYYYMMDD 做自然日偏移（跨月/跨年正确）。
func shiftDay(day, delta int) int {
	t := time.Date(day/10000, time.Month((day%10000)/100), day%100, 0, 0, 0, 0, time.Local).
		AddDate(0, 0, delta)
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}

// StartSiteVisitorLoop 启动常驻 loop：定时落库 + 每日清理。幂等（可重复调用）。
func StartSiteVisitorLoop() {
	siteVisitorLoop.start(func(ctx context.Context) {
		ticker := time.NewTicker(visitorFlushEvery)
		defer ticker.Stop()
		lastMaintenanceDay := VisitorCurrentDay()
		// 启动时先跑一次清理，避免重启后长时间不执行保留策略。
		if err := RunSiteVisitorMaintenance(); err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("site visitor maintenance failed: %v", err))
		}
		for {
			select {
			case <-ctx.Done():
				// 优雅退出：把内存里未落库的访客写下去，避免重启丢计数。
				if err := FlushSiteVisitors(); err != nil {
					logger.LogError(context.Background(), fmt.Sprintf("flush site visitors on shutdown failed: %v", err))
				}
				return
			case <-ticker.C:
				if err := FlushSiteVisitors(); err != nil {
					logger.LogError(context.Background(), fmt.Sprintf("flush site visitors failed: %v", err))
				}
				// 每天跑一次保留策略清理（跨日时触发，避免每 30s 删一次库）。
				if day := VisitorCurrentDay(); day != lastMaintenanceDay {
					lastMaintenanceDay = day
					if err := RunSiteVisitorMaintenance(); err != nil {
						logger.LogError(context.Background(), fmt.Sprintf("site visitor maintenance failed: %v", err))
					}
				}
				common.RecordLoopHeartbeat("site_visitor_flush")
			}
		}
	})
}

// StopSiteVisitorLoop 停止 loop（优雅关闭/测试用）。
func StopSiteVisitorLoop() { siteVisitorLoop.stop() }

// SiteVisitorSummary 是公开端点返回的完整站点统计（访客 + 用户 + 活跃 + 来源）。
type SiteVisitorSummary struct {
	// Visitors 是访客口径（按 IP 按天去重，已排除爬虫）。
	Visitors model.SiteVisitorStats `json:"visitors"`
	// RegisteredUsers 是注册用户总数。
	RegisteredUsers int64 `json:"registered_users"`
	// ActiveUsers24h / 30d 是「有 API 消费」的活跃用户数（DAU / MAU 口径）。
	ActiveUsers24h int64 `json:"active_users_24h"`
	ActiveUsers30d int64 `json:"active_users_30d"`
	// Sources 是近 30 天流量来源分布（去重访客）。
	Sources []model.TrafficSourceCount `json:"sources"`
	// GeneratedAt 是本次统计的生成时刻（unix 秒），让读者知道数据新鲜度。
	GeneratedAt int64 `json:"generated_at"`
}

// GetSiteVisitorSummary 生成站点统计（带 60s 缓存）。
//
// 缓存的意义不只是提速：这些端点挂在公开路由上，匿名爬虫可以高频请求。
// 60s 缓存把任意请求频率折叠成每分钟一次查询。Redis 不可用时静默降级为实时查询
// （查询本身都是小表索引扫描，可接受）。
func GetSiteVisitorSummary() (*SiteVisitorSummary, error) {
	const cacheKey = "site_visitor_summary:v1"
	if common.RedisEnabled {
		if cached, err := common.RedisGet(cacheKey); err == nil && cached != "" {
			var hit SiteVisitorSummary
			if common.Unmarshal([]byte(cached), &hit) == nil && hit.GeneratedAt > 0 {
				return &hit, nil
			}
		}
	}

	today := VisitorCurrentDay()
	stats, err := model.GetSiteVisitorStats(today)
	if err != nil {
		return nil, err
	}
	users, err := model.CountRegisteredUsers()
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	active24h, err := model.CountActiveUsers(now - 24*3600)
	if err != nil {
		return nil, err
	}
	active30d, err := model.CountActiveUsers(now - 30*24*3600)
	if err != nil {
		return nil, err
	}
	sources, err := model.GetTrafficSourceCounts(today, 30)
	if err != nil {
		return nil, err
	}

	// 站内跳转（internal）与直接访问合并为「直接访问」：站内跳转不构成新来源，
	// 但把两类分开列会让「直接访问」看起来偏低。
	sources = mergeInternalIntoDirect(sources)

	summary := &SiteVisitorSummary{
		Visitors:        *stats,
		RegisteredUsers: users,
		ActiveUsers24h:  active24h,
		ActiveUsers30d:  active30d,
		Sources:         sources,
		GeneratedAt:     now,
	}

	if common.RedisEnabled {
		if payload, err := common.Marshal(summary); err == nil {
			_ = common.RedisSet(cacheKey, string(payload), visitorStatsCacheTTL)
		}
	}
	return summary, nil
}

// mergeInternalIntoDirect 把 internal 来源并入 direct（见调用处说明）。
//
// 必须先在整份结果里汇总 internal 再并入 —— 不能一边遍历一边合并：
// 数据库返回的行序是 `count DESC`，internal 完全可能排在 direct **之后**，
// 那时"遇到 direct 就并入"的写法会漏掉它。
func mergeInternalIntoDirect(rows []model.TrafficSourceCount) []model.TrafficSourceCount {
	if len(rows) == 0 {
		return rows
	}
	direct := int64(0)
	out := make([]model.TrafficSourceCount, 0, len(rows))
	for _, r := range rows {
		switch r.Source {
		case "internal":
			direct += r.Count
		case "direct":
			direct += r.Count
		default:
			out = append(out, r)
		}
	}
	if direct > 0 {
		out = append(out, model.TrafficSourceCount{Source: "direct", Count: direct})
	}
	// 合并后重新排序，保持「多的在前」。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Count > out[j-1].Count; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
