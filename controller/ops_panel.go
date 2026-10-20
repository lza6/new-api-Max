package controller

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
)

// 运营面板（管理员专属）。所有 handler 的管理员身份只从会话取（AdminAuth 中间件注入
// id/role），不接受请求体里的角色/操作者。
//
// 口径（与返回字段名一一对应，运营者据此决策，不猜）：
//   - 消耗来源：logs 表中 type = LogTypeConsume 的行，字段 quota（原始额度单位）。
//   - 金额换算：本接口只返回原始 quota 整数。前端用 lib/currency 的 formatQuota 统一换算为
//     站点配置的展示货币（USD/CNY/CUSTOM/TOKENS），Go 侧不做 float 换算——SQL 侧整数 SUM，
//     避免在 Go 里用 float 累加 quota 导致的精度损失。
//   - today_quota：**服务器本地时区**当天 00:00:00 起至今的 consume 额度。
//     ⚠️ 与训练数据采集的「今日」（UTC，见 CaptureStats.day）**不是同一个今天**：
//     中国时区下两者在跨零点后的 8 小时内不一致。响应里分别下发
//     today_quota_timezone / capture_stats.day 让运营者能区分，而不是并排显示两个
//     「今日」却不说口径。
//   - total_quota：全部时间内 consume 额度之和（无时间上限）。
//   - total_remaining_quota：users.quota 之和，仅统计 role < RoleAdminUser（排除管理员与
//     超级管理员），不含软删除用户。
//   - user_count：计入 total_remaining_quota 的用户数（同上口径）。
//   - daily_average_quota：近 average_over_days 天累计 consume 额度 ÷ average_over_days。
//   - estimated_remaining_days：
//     estimated_remaining_days = total_remaining_quota / daily_average_quota
//     即「按最近 average_over_days 天的平均消耗速度，当前全体普通用户的剩余额度还能撑多少天」。
//     日均消耗为 0（窗口内无消耗）时返回 null（JSON null），而不是 +Inf——没有消耗就没有
//     「耗尽」的概念。
//   - 消耗排行榜（/consumption-ranking）：同样**只统计 role < RoleAdminUser**，
//     与 total_remaining_quota 口径一致。管理员常用自己账号跑分析/测试，
//     混进榜单会长期霸榜，且与「剩余额度不含管理员」自相矛盾。
const (
	opsDefaultAverageDays = 14
	opsMaxAverageDays     = 365
	opsDefaultRankDays    = 7
	opsDefaultRankLimit   = 20
	opsMaxRankDays        = 365
	opsMaxRankLimit       = 100
	opsOverviewCacheTTL   = 5 * time.Minute

	// opsMaxInlineAdminIds 排行榜排除管理员时允许内联的最大 ID 数。
	// 超过则退化为 Go 侧过滤（结果仍正确，但可能少于 limit 条并记日志）。
	// 取 500：本站管理员是个位数到几十的量级，内联 `NOT IN (...)` 的
	// 解析与传输代价可忽略；而设成无上限会让一个被误配成管理员的大批量
	// 账号把 SQL 撑成几千个占位符。
	opsMaxInlineAdminIds = 500

	// opsMaxRankingOverscan Go 侧过滤时的取行上限（防止一次巨量扫描）。
	// 超限时榜单可能少于 limit 条 —— 那时记日志说明，比拖垮日志库好。
	opsMaxRankingOverscan = 2000

	// opsTopUpBufferRatio 上游备货缓冲倍数：建议每日充值 = 日均成本 × 该倍数。
	// 1.5 表示留出 50% 余量，覆盖上游涨价、流量波动与结算延迟。
	opsTopUpBufferRatio = 1.5

	// opsMaxDailySeriesDays 日报序列的最大天数（避免一次拉太多聚合）。
	opsMaxDailySeriesDays = 90
)

// opsOverviewData 是 /api/ops/overview 的响应体（字段名即口径，见文件头注释）。
type opsOverviewData struct {
	TodayQuota int64 `json:"today_quota"`
	// TodayQuotaTimezone 上面 today_quota 所用的时区名（如 "Local" / "Asia/Shanghai"）。
	// 采集统计用的是 UTC 日，两者不同口径，必须分别标注。
	TodayQuotaTimezone     string   `json:"today_quota_timezone"`
	TotalQuota             int64    `json:"total_quota"`
	TotalRemainingQuota    int64    `json:"total_remaining_quota"`
	UserCount              int64    `json:"user_count"`
	DailyAverageQuota      float64  `json:"daily_average_quota"`
	AverageOverDays        int      `json:"average_over_days"`
	EstimatedRemainingDays *float64 `json:"estimated_remaining_days"`

	// ---- 财务口径（营收 / 成本 / 利润）----
	// 营收 = consume 日志 quota；成本 = 结算时快照写入的 logs.cost_quota。
	// 两者都是**额度单位**，前端用统一的 formatQuota 换算为人民币展示。
	// 调价只追加成本历史行，不会改写旧日志 → 历史利润可复现。
	TodayRevenueQuota  int64    `json:"today_revenue_quota"`
	TodayCostQuota     int64    `json:"today_cost_quota"`
	TodayProfitQuota   int64    `json:"today_profit_quota"`
	TodayMarginPercent *float64 `json:"today_margin_percent"`
	// 本月（服务器本地时区，1 号 00:00 起）与本年（1 月 1 日 00:00 起）。
	MonthRevenueQuota  int64    `json:"month_revenue_quota"`
	MonthCostQuota     int64    `json:"month_cost_quota"`
	MonthProfitQuota   int64    `json:"month_profit_quota"`
	MonthMarginPercent *float64 `json:"month_margin_percent"`
	YearRevenueQuota   int64    `json:"year_revenue_quota"`
	YearCostQuota      int64    `json:"year_cost_quota"`
	YearProfitQuota    int64    `json:"year_profit_quota"`
	YearMarginPercent  *float64 `json:"year_margin_percent"`
	// 按当前速度外推（本月已过天数 / 本年已过天数），不是承诺值。
	ProjectedMonthProfitQuota float64 `json:"projected_month_profit_quota"`
	ProjectedYearProfitQuota  float64 `json:"projected_year_profit_quota"`
	// 近 average_over_days 天的日均成本 × 1.5，作为「上游每天大概充多少钱」的建议。
	DailyAverageCostQuota      float64 `json:"daily_average_cost_quota"`
	RecommendedDailyTopupQuota float64 `json:"recommended_daily_topup_quota"`
	// 是否配置过任何成本规则；未配置时成本=0，利润数字会偏高，前端必须提示。
	CostConfigured bool `json:"cost_configured"`
	// 最早的成本生效时间（unix 秒）。早于它的日志没有成本快照。
	CostTrackingSince int64 `json:"cost_tracking_since"`
}

// opsDaySeriesItem 运营日报的一行。
type opsDaySeriesItem struct {
	Date         string `json:"date"`
	RevenueQuota int64  `json:"revenue_quota"`
	CostQuota    int64  `json:"cost_quota"`
	ProfitQuota  int64  `json:"profit_quota"`
	Requests     int64  `json:"requests"`
}

// opsRankingItem 是消耗排行榜的一行。
type opsRankingItem struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Quota    int64  `json:"quota"`
	Requests int64  `json:"requests"`
}

// parseOpsIntParam 读取可选的正整数 query 参数并夹取到 [1, maxValue]。
// 缺省或非法（非数字/<=0）时返回 fallback，避免无界聚合与负数 LIMIT。
func parseOpsIntParam(c *gin.Context, name string, fallback, maxValue int) int {
	raw := c.Query(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return min(n, maxValue)
}

// opsFinancialAggregates 一次扫描算出总营收与今日/本月/本年/窗口的营收+成本。
//
// 为什么不用多条独立查询：日志表 1.4M 行时单条全表 SUM 约 9s，五条并发会把
// 连接池打满且总耗时仍受最长那条限制；一条带 CASE 的聚合只扫一遍。
// 结果由 GetOpsOverview 整体缓存（5 分钟），不会每分钟重复扫描。
type opsFinancialAggregates struct {
	TotalRevenue int64
	Today        opsAggregate
	Month        opsAggregate
	Year         opsAggregate
	Window       opsAggregate
}

func queryOpsFinancialAggregates(todayStart, monthStart, yearStart, windowStart int64) (opsFinancialAggregates, error) {
	var row struct {
		TotalRevenue  int64 `gorm:"column:total_revenue"`
		TodayRevenue  int64 `gorm:"column:today_revenue"`
		TodayCost     int64 `gorm:"column:today_cost"`
		MonthRevenue  int64 `gorm:"column:month_revenue"`
		MonthCost     int64 `gorm:"column:month_cost"`
		YearRevenue   int64 `gorm:"column:year_revenue"`
		YearCost      int64 `gorm:"column:year_cost"`
		WindowRevenue int64 `gorm:"column:window_revenue"`
		WindowCost    int64 `gorm:"column:window_cost"`
	}
	err := model.LOG_DB.Model(&model.Log{}).
		Where("type = ?", model.LogTypeConsume).
		Select(`COALESCE(SUM(quota), 0) AS total_revenue,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN quota ELSE 0 END), 0) AS today_revenue,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN cost_quota ELSE 0 END), 0) AS today_cost,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN quota ELSE 0 END), 0) AS month_revenue,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN cost_quota ELSE 0 END), 0) AS month_cost,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN quota ELSE 0 END), 0) AS year_revenue,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN cost_quota ELSE 0 END), 0) AS year_cost,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN quota ELSE 0 END), 0) AS window_revenue,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN cost_quota ELSE 0 END), 0) AS window_cost`,
			todayStart, todayStart, monthStart, monthStart, yearStart, yearStart, windowStart, windowStart).
		Scan(&row).Error
	if err != nil {
		return opsFinancialAggregates{}, err
	}
	return opsFinancialAggregates{
		TotalRevenue: row.TotalRevenue,
		Today:        opsAggregate{Revenue: row.TodayRevenue, Cost: row.TodayCost},
		Month:        opsAggregate{Revenue: row.MonthRevenue, Cost: row.MonthCost},
		Year:         opsAggregate{Revenue: row.YearRevenue, Cost: row.YearCost},
		Window:       opsAggregate{Revenue: row.WindowRevenue, Cost: row.WindowCost},
	}, nil
}

// opsAggregate 一段时间内的营收/成本/请求数。
type opsAggregate struct {
	Revenue  int64
	Cost     int64
	Requests int64
}

// opsAggregateRange 汇总 [from, to) 区间内的 consume 日志。
func opsAggregateRange(from, to int64) (opsAggregate, error) {
	var agg struct {
		Revenue  int64 `gorm:"column:revenue"`
		Cost     int64 `gorm:"column:cost"`
		Requests int64 `gorm:"column:requests"`
	}
	err := model.LOG_DB.Model(&model.Log{}).
		Where("type = ? AND created_at >= ? AND created_at < ?", model.LogTypeConsume, from, to).
		Select("COALESCE(SUM(quota), 0) AS revenue, COALESCE(SUM(cost_quota), 0) AS cost, COUNT(*) AS requests").
		Scan(&agg).Error
	return opsAggregate{Revenue: agg.Revenue, Cost: agg.Cost, Requests: agg.Requests}, err
}

// marginPercent 利润率（0~100）。营收为 0 时返回 nil（JSON null），
// 而不是 0% 或 +Inf——没有营收就没有「利润率」这个概念。
func marginPercent(revenue, profit int64) *float64 {
	if revenue <= 0 {
		return nil
	}
	v := float64(profit) / float64(revenue) * 100
	return &v
}

// opsOverviewLocalCache 是无 Redis 部署的进程内兜底缓存。
// 总览冷查询在 1.46M 行日志上约 9s；没有 Redis 时不能每次打开面板都扫一遍。
type opsOverviewLocalCacheEntry struct {
	key     string
	data    opsOverviewData
	expires time.Time
}

var opsOverviewLocal struct {
	mu    sync.Mutex
	entry opsOverviewLocalCacheEntry
}

func getLocalOpsOverview(key string) (opsOverviewData, bool) {
	opsOverviewLocal.mu.Lock()
	defer opsOverviewLocal.mu.Unlock()
	e := opsOverviewLocal.entry
	if e.key == key && time.Now().Before(e.expires) {
		return e.data, true
	}
	return opsOverviewData{}, false
}

func setLocalOpsOverview(key string, data opsOverviewData) {
	opsOverviewLocal.mu.Lock()
	opsOverviewLocal.entry = opsOverviewLocalCacheEntry{key: key, data: data, expires: time.Now().Add(opsOverviewCacheTTL)}
	opsOverviewLocal.mu.Unlock()
}

// GetOpsOverview GET /api/ops/overview?days=14
// 当日/累计消耗、非管理员用户剩余额度合计、日均消耗与「还能运营多少天」。
func GetOpsOverview(c *gin.Context) {
	days := parseOpsIntParam(c, "days", opsDefaultAverageDays, opsMaxAverageDays)

	// 全表聚合属慢查：结果缓存 5 分钟。先查进程内缓存（无 Redis 部署的兜底），
	// 再查 Redis（多实例共享）；缓存值仅标量，无 OOM 风险。
	cacheKey := "ops:overview:" + strconv.Itoa(days)
	if hit, ok := getLocalOpsOverview(cacheKey); ok {
		common.ApiSuccess(c, hit)
		return
	}
	if common.RedisEnabled {
		if cached, err := common.RedisGet(cacheKey); err == nil && cached != "" {
			var hit opsOverviewData
			if common.Unmarshal([]byte(cached), &hit) == nil {
				setLocalOpsOverview(cacheKey, hit)
				common.ApiSuccess(c, hit)
				return
			}
		}
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Unix()
	windowStart := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()

	// 一次扫描算全部区间（1.4M 行实测 ~9s，结果整体缓存 5 分钟）。
	agg, err := queryOpsFinancialAggregates(todayStart, monthStart, yearStart, windowStart)
	if err != nil {
		common.ApiErrorMsg(c, "failed to query ops aggregates: "+err.Error())
		return
	}
	totalQuota := agg.TotalRevenue
	today, month, year, window := agg.Today, agg.Month, agg.Year, agg.Window

	// 非管理员用户剩余额度合计与人数。Model(&User{}) 自动附加软删除过滤。
	var remaining struct {
		Total     int64 `gorm:"column:total"`
		UserCount int64 `gorm:"column:user_count"`
	}
	if err := model.DB.Model(&model.User{}).
		Where("role < ?", common.RoleAdminUser).
		Select("COALESCE(SUM(quota), 0) AS total, COUNT(*) AS user_count").
		Scan(&remaining).Error; err != nil {
		common.ApiErrorMsg(c, "failed to query remaining quota: "+err.Error())
		return
	}

	dailyAverage := float64(window.Revenue) / float64(days)
	dailyAverageCost := float64(window.Cost) / float64(days)
	monthDays := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
	monthElapsed := max(now.Day(), 1)
	yearDays := 365
	if time.Date(now.Year(), 12, 31, 0, 0, 0, 0, now.Location()).YearDay() == 366 {
		yearDays = 366
	}
	yearElapsed := max(now.YearDay(), 1)

	costSince, costCount := model.ModelCostTrackingSince()

	data := opsOverviewData{
		TodayQuota: today.Revenue,
		// 明确标注口径：前端并排展示两个「今日」时不会误导（盲点 B15）。
		TodayQuotaTimezone:  now.Location().String(),
		TotalQuota:          totalQuota,
		TotalRemainingQuota: remaining.Total,
		UserCount:           remaining.UserCount,
		DailyAverageQuota:   dailyAverage,
		AverageOverDays:     days,

		TodayRevenueQuota:  today.Revenue,
		TodayCostQuota:     today.Cost,
		TodayProfitQuota:   today.Revenue - today.Cost,
		TodayMarginPercent: marginPercent(today.Revenue, today.Revenue-today.Cost),

		MonthRevenueQuota:  month.Revenue,
		MonthCostQuota:     month.Cost,
		MonthProfitQuota:   month.Revenue - month.Cost,
		MonthMarginPercent: marginPercent(month.Revenue, month.Revenue-month.Cost),

		YearRevenueQuota:  year.Revenue,
		YearCostQuota:     year.Cost,
		YearProfitQuota:   year.Revenue - year.Cost,
		YearMarginPercent: marginPercent(year.Revenue, year.Revenue-year.Cost),

		ProjectedMonthProfitQuota:  float64(month.Revenue-month.Cost) / float64(monthElapsed) * float64(monthDays),
		ProjectedYearProfitQuota:   float64(year.Revenue-year.Cost) / float64(yearElapsed) * float64(yearDays),
		DailyAverageCostQuota:      dailyAverageCost,
		RecommendedDailyTopupQuota: dailyAverageCost * opsTopUpBufferRatio,
		CostConfigured:             costCount > 0,
		CostTrackingSince:          costSince,
	}
	// 日均消耗为 0 时不给「还能运营几天」（返回 null），避免除零得到 +Inf。
	if dailyAverage > 0 {
		estimated := float64(remaining.Total) / dailyAverage
		data.EstimatedRemainingDays = &estimated
	}

	if common.RedisEnabled {
		if raw, err := common.Marshal(data); err == nil {
			_ = common.RedisSet(cacheKey, string(raw), opsOverviewCacheTTL)
		}
	}
	setLocalOpsOverview(cacheKey, data)
	common.ApiSuccess(c, data)
}

// opsAdminUserIds 取所有管理员（role >= RoleAdminUser）的用户 ID。
//
// 用于把管理员从**消耗排行榜**里剔除，使其与 `total_remaining_quota`
// （同样只统计 role < RoleAdminUser）口径一致。
// 软删除用户由 `Model(&model.User{})` 自动过滤。
func opsAdminUserIds() ([]int, error) {
	var ids []int
	err := model.DB.Model(&model.User{}).
		Where("role >= ?", common.RoleAdminUser).
		Pluck("id", &ids).Error
	return ids, err
}

// GetOpsConsumptionRanking GET /api/ops/consumption-ranking?days=7&limit=20
// 消耗排行榜：按用户聚合窗口内 consume 额度，降序限量。
//
// **排除管理员**（与 `total_remaining_quota` 同口径）：两处若口径不一致，
// 运营者会看到「排行榜第一名消耗最多，但他的额度不计入剩余额度」这种自相矛盾的数字。
// 管理员常用自己的账号跑分析/测试，混进来会长期占据榜首，让榜单失去意义。
//
// 为什么在 SQL 里用 NOT IN 而不是拉回来再过滤：日志库可能是**另一个库**
// （ClickHouse），没法 JOIN users 表；而若在 Go 侧过滤，`Limit(limit)` 先削了
// 行数，过滤后可能不足 limit 条 —— 榜单会莫名少人。
// 管理员数量极少（远小于用户数），内联 ID 列表的代价可忽略；超上限时
// 退化为 Go 侧过滤（仍然正确，只是可能少于 limit 条）并记日志。
func GetOpsConsumptionRanking(c *gin.Context) {
	days := parseOpsIntParam(c, "days", opsDefaultRankDays, opsMaxRankDays)
	limit := parseOpsIntParam(c, "limit", opsDefaultRankLimit, opsMaxRankLimit)

	adminIds, err := opsAdminUserIds()
	if err != nil {
		common.ApiErrorMsg(c, "failed to resolve admin users: "+err.Error())
		return
	}

	start := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	type rankRow struct {
		UserID   int   `gorm:"column:user_id"`
		Quota    int64 `gorm:"column:quota"`
		Requests int64 `gorm:"column:requests"`
	}
	var rows []rankRow
	// 按 user_id 分组（稳定身份；username 可能被改名），ORDER BY 用完整表达式而非输出
	// 别名以规避方言差异（与 model/usage_report.go 同法）。
	q := model.LOG_DB.Model(&model.Log{}).
		Select("user_id AS user_id",
			"COALESCE(SUM(quota), 0) AS quota",
			"COUNT(*) AS requests").
		Where("type = ? AND created_at >= ?", model.LogTypeConsume, start)

	inlineAdmins := len(adminIds) > 0 && len(adminIds) <= opsMaxInlineAdminIds
	if inlineAdmins {
		q = q.Where("user_id NOT IN ?", adminIds)
	}
	// 回退路径必须**多取** `len(adminIds)` 行再过滤。
	//
	// 只按 limit 取再过滤是错的：管理员若消耗大，会占满前 limit 个位置，
	// 过滤后剩下 0 行 —— 真正的消耗大户**完全不可见**，
	// 而这恰恰是运营者最需要看到的人。多取的余量刚好覆盖「排在前面的管理员数」，
	// 因此过滤后仍能凑满 limit（除非普通用户总数本就不足）。
	// 上限兜底：管理员数量异常大时不能把日志库拖进一次巨量扫描。
	fetchLimit := limit
	if !inlineAdmins && len(adminIds) > 0 {
		fetchLimit = limit + len(adminIds)
		if fetchLimit > opsMaxRankingOverscan {
			fetchLimit = opsMaxRankingOverscan
		}
	}
	if err := q.Group("user_id").
		Order("COALESCE(SUM(quota), 0) DESC, user_id ASC").
		Limit(fetchLimit).
		Scan(&rows).Error; err != nil {
		common.ApiErrorMsg(c, "failed to query consumption ranking: "+err.Error())
		return
	}

	// 回退路径：管理员过多，无法安全内联 → 在 Go 侧剔除。
	if !inlineAdmins && len(adminIds) > 0 {
		adminSet := make(map[int]bool, len(adminIds))
		for _, id := range adminIds {
			adminSet[id] = true
		}
		kept := make([]rankRow, 0, len(rows))
		for _, r := range rows {
			if !adminSet[r.UserID] {
				kept = append(kept, r)
			}
		}
		rows = kept
		if len(rows) > limit {
			rows = rows[:limit]
		}
		common.SysLog(fmt.Sprintf(
			"ops ranking: %d admin accounts exceeded inline limit %d; filtered in memory (fetched %d rows, kept %d)",
			len(adminIds), opsMaxInlineAdminIds, fetchLimit, len(rows)))
	}

	// 用 users 表当前用户名覆盖日志里固化的旧名（改名后仍显示当前名）。
	// 已删除用户不在 users 结果中，其用户名为空，前端回退显示用户 ID。
	usernames := make(map[int]string, len(rows))
	if len(rows) > 0 {
		ids := make([]int, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.UserID)
		}
		var users []struct {
			Id       int    `gorm:"column:id"`
			Username string `gorm:"column:username"`
		}
		if err := model.DB.Model(&model.User{}).
			Where("id IN ?", ids).Select("id, username").Scan(&users).Error; err == nil {
			for _, u := range users {
				usernames[u.Id] = u.Username
			}
		}
	}

	items := make([]opsRankingItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, opsRankingItem{
			UserID:   r.UserID,
			Username: usernames[r.UserID],
			Quota:    r.Quota,
			Requests: r.Requests,
		})
	}
	common.ApiSuccess(c, gin.H{"days": days, "limit": limit, "items": items})
}

// GrantUserQuota POST /api/ops/grant-quota
// body: {"user_id": 12, "quota": 500000}（quota 为原始额度单位，正整数）
//
// 复用既有「管理员加额度」实现（controller.manageUserQuota → model.AdjustUserQuota）：
// 权限层级校验、钱包上限、审计日志、Topup 日志与 i18n 错误均由既有路径处理，这里不写 UPDATE。
func GrantUserQuota(c *gin.Context) {
	var req struct {
		UserID int `json:"user_id"`
		Quota  int `json:"quota"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.UserID <= 0 || req.Quota <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	manageUserQuota(c, ManageRequest{
		Id:     req.UserID,
		Action: "add_quota",
		Mode:   "add",
		Value:  req.Quota,
	})
}

// GetOpsDailySeries GET /api/admin/ops/daily?days=30
//
// 逐日营收/成本/利润（服务器本地时区）。用于观察调价前后的波动：
// 每一天只 SUM 当天日志里的 quota / cost_quota 快照，历史不会被新价格改写。
func GetOpsDailySeries(c *gin.Context) {
	days := parseOpsIntParam(c, "days", 30, opsMaxDailySeriesDays)
	cacheKey := "ops:daily:" + strconv.Itoa(days)
	if common.RedisEnabled {
		if cached, err := common.RedisGet(cacheKey); err == nil && cached != "" {
			var hit []opsDaySeriesItem
			if common.Unmarshal([]byte(cached), &hit) == nil {
				common.ApiSuccess(c, gin.H{"days": days, "items": hit})
				return
			}
		}
	}

	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	items := make([]opsDaySeriesItem, 0, days)
	for i := days - 1; i >= 0; i-- {
		dayStart := startOfToday.AddDate(0, 0, -i)
		dayEnd := dayStart.AddDate(0, 0, 1)
		agg, err := opsAggregateRange(dayStart.Unix(), dayEnd.Unix())
		if err != nil {
			common.ApiErrorMsg(c, "failed to query daily aggregates: "+err.Error())
			return
		}
		items = append(items, opsDaySeriesItem{
			Date:         dayStart.Format("2006-01-02"),
			RevenueQuota: agg.Revenue,
			CostQuota:    agg.Cost,
			ProfitQuota:  agg.Revenue - agg.Cost,
			Requests:     agg.Requests,
		})
	}

	if common.RedisEnabled {
		if raw, err := common.Marshal(items); err == nil {
			_ = common.RedisSet(cacheKey, string(raw), opsOverviewCacheTTL)
		}
	}
	common.ApiSuccess(c, gin.H{"days": days, "items": items})
}

// GetModelCosts GET /api/admin/ops/model-costs?channel_id=&model=&limit=
// 返回成本配置历史（append-only；调价只新增，不修改旧行）。
func GetModelCosts(c *gin.Context) {
	channelId, _ := strconv.Atoi(c.Query("channel_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	rows, err := model.ListModelCosts(channelId, c.Query("model"), limit)
	if err != nil {
		common.ApiErrorMsg(c, "failed to list model costs: "+err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

// CreateModelCost POST /api/admin/ops/model-costs
// body: {channel_id, model, billing_mode, cost_per_call_cny, cost_input_per_million_cny,
//
//	cost_output_per_million_cny, effective_from?, note?}
//
// **只追加**：同一渠道/模型再次提交会生成新的历史行，旧行保持不变。
func CreateModelCost(c *gin.Context) {
	var in model.ModelCostInput
	if err := common.DecodeJson(c.Request.Body, &in); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.ValidateModelCostInput(&in, time.Now()); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	row, err := model.CreateModelCost(in, c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, "failed to create model cost: "+err.Error())
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "admin.model_cost.create", map[string]any{
		"channel_id":     row.ChannelId,
		"model":          row.Model,
		"billing_mode":   row.BillingMode,
		"effective_from": row.EffectiveFrom,
		"rule_id":        row.Id,
	})
	common.ApiSuccess(c, row)
}
