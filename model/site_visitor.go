package model

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// SiteDailyVisitor 是站点访客的**按天去重**记录（Batch-13 / GEO & 站点流量透明度）。
//
// ## 为什么需要一张新表
//
// 站点已有的两张表都不能回答「今天有多少人来过」：
//   - `web_request_logs` 是**按 60 秒窗口聚合**的：同一个人刷新 10 次就是 10 行，
//     而且只保留 7 天（`DeleteWebRequestLogsBefore(now-7d)`）。拿它数人头会把
//     一个人算成几十个，且窗口跨界还会重复计数。
//   - `logs`（消费日志）只记录**已认证的 API 调用**，访问首页/定价页的访客不在其中。
//
// 因此新建本表，用**唯一键 (day, ip_hash)** 把「一天一个 IP 只算一次」变成
// **数据库层面的约束**，而不是依赖写入方的正确性：即使 tracker 因为重启、
// 并发、重放等原因重复提交同一 (day, ip_hash)，`ON CONFLICT DO NOTHING` 也只会留下一条。
// 这是「数据必须真实」这条要求最可靠的实现方式 —— 统计口径由 schema 保证。
//
// ## 隐私：不存 IP 明文
//
// `ip_hash` 是 `HMAC-SHA256(CryptoSecret, day + "|" + ip)` 的十六进制串：
//   - **带 day 前缀**：不同日期之间的哈希不可关联，无法用本表做长期 IP 追踪；
//   - **带密钥**：没有 `CryptoSecret` 就无法用字典穷举反查 IPv4（IPv4 空间只有 2^32，
//     裸 SHA256 是可直接爆破的）。
//
// 因此本表可以安全地作为公开统计的数据源：它能数出「多少人」，但反推不出「是谁」。
type SiteDailyVisitor struct {
	ID uint64 `json:"id" gorm:"primaryKey"`
	// Day 是访客所属的自然日，格式 YYYYMMDD（服务器本地时区）。
	// 用整数而不是日期类型，是为了让唯一索引与范围查询在不同数据库上都一致
	// （SQLite / MySQL / PostgreSQL 的日期类型语义有差异）。
	Day int `json:"day" gorm:"uniqueIndex:idx_site_visitor_day_ip,priority:1;index:idx_site_visitor_day"`
	// IPHash 见上方隐私说明，恒为 64 位十六进制。
	IPHash string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_site_visitor_day_ip,priority:2"`
	// FirstSeenAt / LastSeenAt 是当天的首次与最后一次访问（unix 秒）。
	// 两者都不参与去重口径，仅用于运营参考（如判断访客是否回访）。
	FirstSeenAt int64 `json:"first_seen_at"`
	LastSeenAt  int64 `json:"last_seen_at"`
	// Hits 是该 IP 当天的访问次数（聚合计数，便于观察回访强度）。
	Hits int `json:"hits"`
	// Source 是**首次**访问时的流量来源归类（direct / search / social / referral / internal）。
	// 只记首次：一个访客当天从多个渠道进来时，记「他是怎么发现本站的」比记最后一次更符合
	// 「流量来源」的语义。归类的判定逻辑见 service.trafficSourceFromReferrer。
	Source string `json:"source" gorm:"type:varchar(24)"`
	// ReferrerHost 是首次访问时的来源主机名（不含路径与查询串，避免记录敏感 query）。
	// direct 时为空串。
	ReferrerHost string `json:"referrer_host" gorm:"type:varchar(255)"`
	// Path 是首次访问的落地路径，用于观察「访客从哪个页面进入」。
	Path string `json:"path" gorm:"type:varchar(255)"`
	// IsBot 标记 UA 判定为爬虫/自动化的访客。**统计默认排除它们** ——
	// 把 Googlebot 算进「今日访客」会让数字虚高且没有意义。
	IsBot bool `json:"is_bot" gorm:"index:idx_site_visitor_bot"`
}

func (SiteDailyVisitor) TableName() string {
	return "site_daily_visitors"
}

// SiteDailyVisitorRow 是待写入的一行（tracker 的内部待落库结构）。
type SiteDailyVisitorRow struct {
	Day          int
	IPHash       string
	FirstSeenAt  int64
	LastSeenAt   int64
	Hits         int
	Source       string
	ReferrerHost string
	Path         string
	IsBot        bool
}

// UpsertSiteDailyVisitors 批量写入访客行，**按 (day, ip_hash) 去重**。
//
// 冲突时更新：`last_seen_at`、`hits`（累加）、`is_bot`（见下）。
// **不改** first_seen_at / source / referrer_host / path —— 这几个字段语义上属于
// 「首次」：覆盖它们会让来源统计随当天最后一次访问漂移。
//
// `is_bot` 是唯一采用**累积语义**的字段：`新值 = 旧值 AND 本次值`。
// 理由是它与「首次」类字段性质不同 —— 它表达的是「这个访客是不是真人」：
//   - 若只按首次写入，一个爬虫先来、真人后来，真人当天就**永远不会被计入**；
//   - 反过来真人先来、爬虫后来，爬虫的 hits 会把一个真人行的次数刷高。
//
// 取 AND 之后：只有**当天该 IP 的全部访问都是爬虫**时，该行才标记为爬虫。
// 这是「统计要真实」这条要求在去重场景下的正确语义。
//
// 分方言实现：SQLite / PostgreSQL 用 `ON CONFLICT (col) DO UPDATE`，
// MySQL 用 `ON DUPLICATE KEY UPDATE`。不用 GORM 的 clause.OnConflict 是因为
// 它在 MySQL 上生成的 `VALUES(col)` 已被 8.0.20 弃用，且目标列写法三库不通用。
func UpsertSiteDailyVisitors(rows []SiteDailyVisitorRow) error {
	if len(rows) == 0 {
		return nil
	}
	if len(rows) > siteVisitorMaxBatch {
		rows = rows[:siteVisitorMaxBatch]
	}
	rows = dedupeSiteVisitorRows(rows)
	return DB.Transaction(func(tx *gorm.DB) error {
		switch DB.Dialector.Name() {
		case "mysql":
			return upsertSiteDailyVisitorsMySQL(tx, rows)
		default:
			return upsertSiteDailyVisitorsConflict(tx, rows)
		}
	})
}

// dedupeSiteVisitorRows 合并同一批次里重复的 (day, ip_hash)。
//
// **必须合并**：SQLite 的 `ON CONFLICT DO UPDATE` 在同一条 INSERT 里遇到两行
// 冲突于同一唯一键时会直接报错（"ON CONFLICT clause does not match any PRIMARY KEY
// or unique constraint" 或 UNIQUE 冲突），MySQL 的行为则是顺序应用。把重复行
// 先合并成一行，两个方言才有一致且确定的行为。
func dedupeSiteVisitorRows(rows []SiteDailyVisitorRow) []SiteDailyVisitorRow {
	seen := make(map[visitorRowKey]int, len(rows))
	out := make([]SiteDailyVisitorRow, 0, len(rows))
	for _, r := range rows {
		k := visitorRowKey{day: r.Day, ipHash: r.IPHash}
		if idx, ok := seen[k]; ok {
			merged := out[idx]
			merged.Hits += r.Hits
			if r.LastSeenAt > merged.LastSeenAt {
				merged.LastSeenAt = r.LastSeenAt
			}
			if r.FirstSeenAt > 0 && (merged.FirstSeenAt == 0 || r.FirstSeenAt < merged.FirstSeenAt) {
				merged.FirstSeenAt = r.FirstSeenAt
			}
			merged.IsBot = merged.IsBot && r.IsBot
			out[idx] = merged
			continue
		}
		seen[k] = len(out)
		out = append(out, r)
	}
	return out
}

type visitorRowKey struct {
	day    int
	ipHash string
}

// siteVisitorMaxBatch 限制单次事务的行数：一批过大既会拉长事务持锁时间，
// 也可能撞上驱动/数据库的参数上限（每行 9 个占位符）。
const siteVisitorMaxBatch = 500

// siteVisitorInsertColumns 是 INSERT 的列清单（两个方言共用）。
const siteVisitorInsertColumns = `(day, ip_hash, first_seen_at, last_seen_at, hits, source, referrer_host, path, is_bot)`

// siteVisitorRowPlaceholders 是**一行**的占位符个数（列数）。
const siteVisitorRowPlaceholders = 9

func upsertSiteDailyVisitorsConflict(tx *gorm.DB, rows []SiteDailyVisitorRow) error {
	sql := `INSERT INTO site_daily_visitors ` + siteVisitorInsertColumns + `
		VALUES ` + visitorValuesClause(len(rows)) + `
		ON CONFLICT (day, ip_hash) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			hits = site_daily_visitors.hits + excluded.hits,
			is_bot = site_daily_visitors.is_bot AND excluded.is_bot`
	return execSiteVisitorUpsert(tx, sql, rows)
}

func upsertSiteDailyVisitorsMySQL(tx *gorm.DB, rows []SiteDailyVisitorRow) error {
	sql := `INSERT INTO site_daily_visitors ` + siteVisitorInsertColumns + `
		VALUES ` + visitorValuesClause(len(rows)) + `
		ON DUPLICATE KEY UPDATE
			last_seen_at = VALUES(last_seen_at),
			hits = hits + VALUES(hits),
			is_bot = is_bot AND VALUES(is_bot)`
	return execSiteVisitorUpsert(tx, sql, rows)
}

// visitorValuesClause 生成 `(?,?,…), (?,?,…)` 形式的多行占位符。
//
// **必须按行数展开**：占位符数量与参数数量不一致时，SQLite 不报错，
// 而是静默只用第一组绑定值、丢弃其余行（本文件最初就踩了这个坑 ——
// 一次 flush 只入库一行，测试才把它抓出来）。因此这里由行数生成子句，
// 参数按行扁平追加，二者不可能错位。
func visitorValuesClause(rowCount int) string {
	if rowCount <= 0 {
		rowCount = 1
	}
	one := "(?, ?, ?, ?, ?, ?, ?, ?, ?)"
	var b strings.Builder
	b.Grow(len(one)*rowCount + (rowCount-1)*2)
	for i := 0; i < rowCount; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(one)
	}
	return b.String()
}

func execSiteVisitorUpsert(tx *gorm.DB, sql string, rows []SiteDailyVisitorRow) error {
	args := make([]any, 0, len(rows)*siteVisitorRowPlaceholders)
	for _, r := range rows {
		args = append(args, r.Day, r.IPHash, r.FirstSeenAt, r.LastSeenAt, r.Hits, r.Source, r.ReferrerHost, r.Path, r.IsBot)
	}
	return tx.Exec(sql, args...).Error
}

// SiteVisitorStats 是站点访客的聚合结果（公开端点返回的结构）。
type SiteVisitorStats struct {
	// Today / Last7d / Last30d 是**去重后的访客数**（UV），已排除爬虫。
	Today   int64 `json:"today"`
	Last7d  int64 `json:"last_7d"`
	Last30d int64 `json:"last_30d"`
	// Total 是自本表启用以来的累计去重访客数。
	//
	// 口径说明：同一个 IP 在不同日期各计一次，因此 Total = SUM(每日 UV)，
	// 不是「独立 IP 总数」。这里刻意选前者：它随时间单调增长、不会因为访客
	// 换 IP 而跳变，也符合「总访问量」在日常语境下的含义。
	Total int64 `json:"total"`
	// PageViews 是同期页面访问总次数（不按 IP 去重）。
	PageViewsToday   int64 `json:"page_views_today"`
	PageViewsTotal   int64 `json:"page_views_total"`
	PageViewsLast30d int64 `json:"page_views_last_30d"`
}

// GetSiteVisitorStats 聚合访客统计。day 参数为「今天」的 YYYYMMDD。
//
// 全部为分日范围的 COUNT/SUM，命中 (day, ip_hash) 唯一索引与 day 索引；
// 单表体量 = 天数 × 每日 UV，属于小表，无全表扫描风险。
//
// **PV 与 UV 同口径排除爬虫**：这两个数字在主页上并排展示，若 UV 排除爬虫而 PV
// 包含，同一张卡片上就会出现两个定义不同的数，读者极易误解为统计错误。
// 因此统一为「真实访客的页面浏览」。（爬虫行仍保存在表里以备将来分析，
// 只是不参与任何对外展示的口径。）
func GetSiteVisitorStats(today int) (*SiteVisitorStats, error) {
	stats := &SiteVisitorStats{}
	dayFloor := today - 10000 // 安全下界，避免极端 day 值

	countUV := func(from, to int) (int64, error) {
		var n int64
		err := DB.Model(&SiteDailyVisitor{}).
			Where("is_bot = ? AND day >= ? AND day <= ?", false, from, to).
			Count(&n).Error
		return n, err
	}

	sumPV := func(from, to int) (int64, error) {
		var n *int64
		err := DB.Model(&SiteDailyVisitor{}).
			Where("is_bot = ? AND day >= ? AND day <= ?", false, from, to).
			Select("COALESCE(SUM(hits), 0)").Scan(&n).Error
		if n == nil {
			return 0, err
		}
		return *n, err
	}

	// 「近 7 天 / 近 30 天」按自然日回溯，用 day 整数运算：day 是 YYYYMMDD，
	// 直接减 N 在跨月/跨年时**不正确**（如 20260101 - 1 = 20260100）。
	// 因此这里回退到用真实日期计算下界。
	from7 := DayOffset(today, -6)
	from30 := DayOffset(today, -29)

	var err error
	if stats.Today, err = countUV(today, today); err != nil {
		return nil, err
	}
	if stats.Last7d, err = countUV(from7, today); err != nil {
		return nil, err
	}
	if stats.Last30d, err = countUV(from30, today); err != nil {
		return nil, err
	}
	if stats.Total, err = countUV(dayFloor, today); err != nil {
		return nil, err
	}
	if stats.PageViewsToday, err = sumPV(today, today); err != nil {
		return nil, err
	}
	if stats.PageViewsLast30d, err = sumPV(from30, today); err != nil {
		return nil, err
	}
	if stats.PageViewsTotal, err = sumPV(dayFloor, today); err != nil {
		return nil, err
	}
	return stats, nil
}

// DayOffset 返回某个 YYYYMMDD 自然日前后偏移 N 天后的 YYYYMMDD。
// 用 time 正常解析以保证跨月/跨年正确 —— `day` 是 YYYYMMDD 整数，
// 直接做 `day - 1` 在每月 1 号会得到 20260100 这种非法值。
//
// 导出供 service 层复用（访客保留策略与管理任务需要同一套日期算术）。
func DayOffset(day, delta int) int {
	t := time.Date(day/10000, time.Month((day%10000)/100), day%100, 0, 0, 0, 0, time.Local).
		AddDate(0, 0, delta)
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}

// DeleteSiteDailyVisitorsBefore 删除早于 cutoffDay 的访客记录（保留策略，幂等）。
//
// 本表是**逐日累积**的，若不清理会无限增长（每天新增「当天 UV」行）。
// 默认保留 400 天（覆盖同比对比），由后台维护任务调用。
func DeleteSiteDailyVisitorsBefore(cutoffDay int) (int64, error) {
	if cutoffDay <= 0 {
		return 0, nil
	}
	result := DB.Where("day < ?", cutoffDay).Delete(&SiteDailyVisitor{})
	return result.RowsAffected, result.Error
}

// TrafficSourceCount 是流量来源分布的一行。
type TrafficSourceCount struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

// GetTrafficSourceCounts 返回近 days 天的流量来源分布（按首次访问来源归类，去重访客）。
//
// 返回的 Count 之和可能小于同期 UV 总数：来源只记首次访问，而 UV 按 (day, ip) 计 ——
// 同一 IP 在不同日期重新进入时各计一次 UV，但来源字段始终是它当天的首次来源。
// 这正是本函数只做「分布」而不声称「等于 UV 总数」的原因。
func GetTrafficSourceCounts(today, days int) ([]TrafficSourceCount, error) {
	if days <= 0 {
		days = 30
	}
	from := DayOffset(today, -(days - 1))
	rows := make([]TrafficSourceCount, 0, 6)
	err := DB.Model(&SiteDailyVisitor{}).
		Select("source, COUNT(*) AS count").
		Where("is_bot = ? AND day >= ? AND day <= ?", false, from, today).
		Group("source").
		Order("count DESC").
		Scan(&rows).Error
	return rows, err
}

// CountRegisteredUsers 返回站点已注册（未删除）的用户总数。
// 供公开统计使用，只返回计数，不暴露任何用户明细。
func CountRegisteredUsers() (int64, error) {
	var n int64
	err := DB.Model(&User{}).Count(&n).Error
	return n, err
}

// CountActiveUsers 返回近 days 天内**有实际 API 消费**的活跃用户数（DAU/MAU 口径）。
//
// 口径选择说明：这里以 `logs` 表的消费日志（type=2）为活动信号，而不是登录时间。
//   - 登录时间只能反映「打开过管理台」，对网关产品而言「用过 API」才是实质活跃；
//   - `logs` 表在 (type, created_at) 上有索引 idx_log_type_created_id，该查询走索引。
//
// 刻意不做「登录也算活跃」的并集：两种口径混在一起会让 DAU 既不是调用维度的、
// 也不是登录维度的，反而不可解释。若将来要并入登录，应作为**独立指标**返回。
func CountActiveUsers(fromUnix int64) (int64, error) {
	var n int64
	err := LOG_DB.Model(&Log{}).
		Where("type = ? AND created_at >= ?", LogTypeConsume, fromUnix).
		Distinct("user_id").
		Count(&n).Error
	return n, err
}

// String 便于日志输出。
func (s SiteVisitorStats) String() string {
	return fmt.Sprintf("uv today=%d 7d=%d 30d=%d total=%d", s.Today, s.Last7d, s.Last30d, s.Total)
}
