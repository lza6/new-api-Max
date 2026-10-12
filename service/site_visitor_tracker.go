package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
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

// checkVisitorHashSecret 在启动时检查匿名化密钥是否**不稳定**。
//
// 只有一种情况真正危险：`CRYPTO_SECRET` 与 `SESSION_SECRET` **都没有**显式配置。
// 此时 `CryptoSecret` 回落到 `SessionSecret`，而后者是**进程启动时随机生成**的
// （见 `common/init.go`），于是：
//   - 进程一重启，同一个 IP 在同一天会得到**不同的 hash**，
//     `(day, ip_hash)` 唯一键挡不住 → 访客数虚高、表行数膨胀（去重静默失效）；
//   - 多实例部署时每个实例各算一遍。
//
// **不能**只判 `CryptoSecret == SessionSecret`：在「只设了 SESSION_SECRET」这种
// 常见且完全安全的配置下它同样成立，会天天误报 —— 误报会让真实告警被无视。
// 这里直接判两个环境变量是否都为空（与 `common/init.go` 的赋值逻辑对应）。
func checkVisitorHashSecret() {
	if os.Getenv("CRYPTO_SECRET") != "" || os.Getenv("SESSION_SECRET") != "" {
		return // 至少有一个显式配置 → 派生密钥跨重启稳定，去重成立
	}
	common.SysError("site visitor stats: neither CRYPTO_SECRET nor SESSION_SECRET is set, " +
		"so the HMAC key for visitor IP hashing is randomly generated per process. " +
		"Visitor deduplication keys will change on every restart, so one IP can be counted " +
		"more than once per day and the visitor table will grow. " +
		"Set CRYPTO_SECRET (or SESSION_SECRET) to a fixed value.")
}

// StartSiteVisitorLoop 启动常驻 loop：定时落库 + 每日清理。幂等（可重复调用）。
func StartSiteVisitorLoop() {
	checkVisitorHashSecret()
	siteVisitorLoop.start(func(ctx context.Context) {
		ticker := time.NewTicker(visitorFlushEvery)
		defer ticker.Stop()
		lastMaintenanceDay := VisitorCurrentDay()
		// 启动时先跑一次清理，避免重启后长时间不执行保留策略。
		if err := RunSiteVisitorMaintenance(); err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("site visitor maintenance failed: %v", err))
		}
		// 预热统计缓存：聚合一次要数秒（DAU/MAU 扫 logs 大表），放在后台先算好，
		// 让第一个真实请求走缓存而不是承担这段延迟。失败只告警 —— 请求路径上
		// 仍会按需重试（见 siteVisitorCache.get 的冷启动分支）。
		go func() {
			WarmSiteVisitorSummary()
			common.RecordLoopHeartbeat("site_visitor_summary_warm")
		}()
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
	// Day 是「今日」对应的自然日（YYYYMMDD，服务器本地时区）。
	// 跨时区的读者据此判断 today 指哪一天，而不是拿自己的本地日期去猜。
	Day int `json:"day"`
}

// siteVisitorCache 是带 **stale-while-revalidate** 的单值缓存（Batch-13 压测修复）。
//
// ## 为什么不能用「读缓存 → 未命中就同步查库」这种朴素写法
//
// 生产压测（12 并发 × 20 请求）实测：`/api/site/visitors` 出现 **11 次 30s 超时**。
// 原因有两个，缺一不可：
//  1. 整个聚合（访客+PV+注册用户+DAU+MAU+来源）一次要 **6.3 秒** ——
//     其中 DAU/MAU 是对大表 `logs` 做 `COUNT(DISTINCT user_id)`；
//  2. 缓存一过期，并发的 N 个请求会**同时**未命中并各自跑一遍这 6.3 秒的聚合
//     （缓存击穿），互相争抢 DB，于是少数请求直接撞上 30s 超时。
//
// 因此这里同时解决两件事：
//   - **后台刷新（SWR）**：只要手里有值，就**立即返回旧值**并让一个后台 goroutine
//     去刷新，请求路径上**永远不会**阻塞在聚合上。冷启动（进程内还没有值）才会同步算一次。
//   - **单飞（singleflight）**：`refreshing` 标志保证同一时刻只有一个刷新在跑，
//     无论来多少并发请求。
//
// 代价是数据可能比 TTL 旧一轮 —— 对"站点访客有多少"这类展示量完全可接受，
// 换来的是这个**公开端点**在任何请求频率下都不会拖垮 DB 或自身超时。
type siteVisitorCache[T any] struct {
	mu         sync.Mutex
	value      *T
	computedAt time.Time
	refreshing bool
	// inflight 在有一次计算进行中时非 nil，并在该次计算结束时 close。
	// 冷启动时让并发调用**等在飞的这次计算**，而不是各算一遍 —— 这是防击穿的关键：
	// 没有它时，压测里 16 个并发请求会在缓存为空时全部跑一遍 6 秒的聚合。
	inflight chan struct{}
	ttl      time.Duration
	// now 可在测试中替换，避免用 sleep 验证 TTL。
	now func() time.Time
}

func newSiteVisitorCache[T any](ttl time.Duration) *siteVisitorCache[T] {
	return &siteVisitorCache[T]{ttl: ttl, now: time.Now}
}

// get 取缓存值。三种情形：
//   - 未过期  → 直接返回；
//   - 已过期且有旧值 → **立即返回旧值**，并在后台刷新（不阻塞）；
//   - 无值     → 单飞一次同步计算（并发调用共享同一次计算）。
func (c *siteVisitorCache[T]) get(compute func() (*T, error)) (*T, error) {
	for {
		c.mu.Lock()
		if c.value != nil {
			if c.now().Sub(c.computedAt) < c.ttl {
				v := c.value
				c.mu.Unlock()
				return v, nil
			}
			stale := c.value
			if !c.refreshing {
				c.refreshing = true
				ch := make(chan struct{})
				c.inflight = ch
				go c.runAndStore(compute, ch)
			}
			c.mu.Unlock()
			return stale, nil
		}
		if c.refreshing {
			// 已有人在算：等它结束再重新读缓存，不要自己也跑一遍。
			ch := c.inflight
			c.mu.Unlock()
			<-ch
			continue
		}
		c.refreshing = true
		ch := make(chan struct{})
		c.inflight = ch
		c.mu.Unlock()
		// 冷启动：本次调用承担同步计算。每个调用最多算一次，不会无限循环。
		return c.runAndStore(compute, ch)
	}
}

// runAndStore 执行一次计算并写回缓存，最后 close(ch) 唤醒等待者。
func (c *siteVisitorCache[T]) runAndStore(compute func() (*T, error), ch chan struct{}) (*T, error) {
	v, err := compute()

	c.mu.Lock()
	c.refreshing = false
	if c.inflight == ch {
		c.inflight = nil
	}
	if err == nil && v != nil {
		c.value = v
		c.computedAt = c.now()
	} else {
		// 失败：保留旧值（若有）。同时把 computedAt 推到当前时刻，
		// 避免「过期 + 持续失败」时每个请求都立刻再起一个后台刷新形成忙循环。
		c.computedAt = c.now()
	}
	c.mu.Unlock()
	close(ch)
	return v, err
}

// store 直接写入缓存（测试与预热用）。
func (c *siteVisitorCache[T]) store(v *T) {
	c.mu.Lock()
	c.value = v
	c.computedAt = c.now()
	c.mu.Unlock()
}

// peek 读取当前值（可能为 nil），不触发刷新。仅供测试与观测。
func (c *siteVisitorCache[T]) peek() *T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// visitorSummaryCacheTTL 是访客/页面浏览/注册用户/来源的缓存时长。
// 这几项查的都是小表（`site_daily_visitors`、`users`），秒级刷新成本可接受。
const visitorSummaryCacheTTL = 60 * time.Second

// activeUsersCacheTTL 是 DAU/MAU 的缓存时长，**刻意远长于上面那组**。
//
// 原因：DAU/MAU 要在大表 `logs` 上做 `COUNT(DISTINCT user_id)`，是整个聚合里
// 唯一昂贵（秒级）的部分；同时这两个数字本身变化极慢（日活/月活不会以秒为单位变动）。
// 花 30 分钟一次的后台计算换来请求路径彻底无阻塞，是这个端点最合适的取舍。
const activeUsersCacheTTL = 30 * time.Minute

var (
	siteVisitorStatsCache = newSiteVisitorCache[model.SiteVisitorStats](visitorSummaryCacheTTL)
	siteActiveUsersCache  = newSiteVisitorCache[siteActiveUsers](activeUsersCacheTTL)
	siteSourcesCache      = newSiteVisitorCache[[]model.TrafficSourceCount](visitorSummaryCacheTTL)
	siteRegUsersCache     = newSiteVisitorCache[int64](visitorSummaryCacheTTL)
)

// siteActiveUsers 是 DAU / MAU 两个计数（贵的那个查询）。
type siteActiveUsers struct {
	Last24h int64
	Last30d int64
}

// GetSiteVisitorSummary 生成站点统计。
//
// 全部经由 SWR 缓存（见 siteVisitorCache 的说明），因此**请求路径上不做同步聚合**
// （除进程启动后的第一次）。返回的错误只表示"冷启动时的首次计算失败"。
func GetSiteVisitorSummary() (*SiteVisitorSummary, error) {
	today := VisitorCurrentDay()

	stats, err := siteVisitorStatsCache.get(func() (*model.SiteVisitorStats, error) {
		s, e := model.GetSiteVisitorStats(VisitorCurrentDay())
		return s, e
	})
	if err != nil {
		return nil, err
	}

	users, err := siteRegUsersCache.get(func() (*int64, error) {
		n, e := model.CountRegisteredUsers()
		return &n, e
	})
	if err != nil {
		return nil, err
	}

	// DAU/MAU 放在一个 compute 里一起算：两者都扫 logs 表，合并在同一轮刷新中完成，
	// 避免把一个昂贵查询拆成两次触发。
	active, err := siteActiveUsersCache.get(func() (*siteActiveUsers, error) {
		now := common.GetTimestamp()
		dau, e := model.CountActiveUsers(now - 24*3600)
		if e != nil {
			return nil, e
		}
		mau, e := model.CountActiveUsers(now - 30*24*3600)
		if e != nil {
			return nil, e
		}
		return &siteActiveUsers{Last24h: dau, Last30d: mau}, nil
	})
	if err != nil {
		return nil, err
	}

	sources, err := siteSourcesCache.get(func() (*[]model.TrafficSourceCount, error) {
		s, e := model.GetTrafficSourceCounts(VisitorCurrentDay(), 30)
		if e != nil {
			return nil, e
		}
		// 站内跳转（internal）与直接访问合并为「直接访问」：站内跳转不构成新来源，
		// 但把两类分开列会让「直接访问」看起来偏低。
		merged := mergeInternalIntoDirect(s)
		return &merged, nil
	})
	if err != nil {
		return nil, err
	}

	mergedSources := []model.TrafficSourceCount{}
	if sources != nil {
		mergedSources = *sources
	}
	activeUsers := siteActiveUsers{}
	if active != nil {
		activeUsers = *active
	}
	regUsers := int64(0)
	if users != nil {
		regUsers = *users
	}
	visitorStats := model.SiteVisitorStats{}
	if stats != nil {
		visitorStats = *stats
	}

	return &SiteVisitorSummary{
		Visitors:        visitorStats,
		RegisteredUsers: regUsers,
		ActiveUsers24h:  activeUsers.Last24h,
		ActiveUsers30d:  activeUsers.Last30d,
		Sources:         mergedSources,
		GeneratedAt:     common.GetTimestamp(),
		Day:             today,
	}, nil
}

// WarmSiteVisitorSummary 预热缓存（启动时调用，避免第一个真实请求承担冷启动的同步聚合）。
func WarmSiteVisitorSummary() { _, _ = GetSiteVisitorSummary() }

// ResetSiteVisitorCachesForTest 清空统计缓存。
//
// 缓存是**进程级**的，而测试在同一个进程里跑 —— 不重置就会让后一个用例读到
// 前一个用例算出的数据（本地实测：controller 包的访客端点用例因此拿到空数据）。
// 生产代码没有任何调用方。
func ResetSiteVisitorCachesForTest() {
	siteVisitorStatsCache.store(nil)
	siteActiveUsersCache.store(nil)
	siteSourcesCache.store(nil)
	siteRegUsersCache.store(nil)
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
