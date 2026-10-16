package controller

import (
	"fmt"
	"strconv"
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
	opsOverviewCacheTTL   = time.Minute

	// opsMaxInlineAdminIds 排行榜排除管理员时允许内联的最大 ID 数。
	// 超过则退化为 Go 侧过滤（结果仍正确，但可能少于 limit 条并记日志）。
	// 取 500：本站管理员是个位数到几十的量级，内联 `NOT IN (...)` 的
	// 解析与传输代价可忽略；而设成无上限会让一个被误配成管理员的大批量
	// 账号把 SQL 撑成几千个占位符。
	opsMaxInlineAdminIds = 500

	// opsMaxRankingOverscan Go 侧过滤时的取行上限（防止一次巨量扫描）。
	// 超限时榜单可能少于 limit 条 —— 那时记日志说明，比拖垮日志库好。
	opsMaxRankingOverscan = 2000
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

// opsConsumeQuotaSince 返回 since（Unix 秒；<=0 表示不限起点）之后 consume 日志的额度整数之和。
// SQL 侧 SUM，不在 Go 侧用 float 累加 quota。
func opsConsumeQuotaSince(since int64) (int64, error) {
	var agg struct {
		Quota int64 `gorm:"column:quota"`
	}
	tx := model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume)
	if since > 0 {
		tx = tx.Where("created_at >= ?", since)
	}
	if err := tx.Select("COALESCE(SUM(quota), 0) AS quota").Scan(&agg).Error; err != nil {
		return 0, err
	}
	return agg.Quota, nil
}

// GetOpsOverview GET /api/ops/overview?days=14
// 当日/累计消耗、非管理员用户剩余额度合计、日均消耗与「还能运营多少天」。
func GetOpsOverview(c *gin.Context) {
	days := parseOpsIntParam(c, "days", opsDefaultAverageDays, opsMaxAverageDays)

	// 全表 SUM 属慢查（与 /api/log/overview 同类），60s 结果缓存根治重复扫描；
	// 缓存值仅 7 个标量，无 OOM 风险。Redis 未启用时静默跳过。
	cacheKey := "ops:overview:" + strconv.Itoa(days)
	if common.RedisEnabled {
		if cached, err := common.RedisGet(cacheKey); err == nil && cached != "" {
			var hit opsOverviewData
			if common.Unmarshal([]byte(cached), &hit) == nil {
				common.ApiSuccess(c, hit)
				return
			}
		}
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	windowStart := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()

	totalQuota, err := opsConsumeQuotaSince(0)
	if err != nil {
		common.ApiErrorMsg(c, "failed to query total consumption: "+err.Error())
		return
	}
	todayQuota, err := opsConsumeQuotaSince(todayStart)
	if err != nil {
		common.ApiErrorMsg(c, "failed to query today consumption: "+err.Error())
		return
	}
	windowQuota, err := opsConsumeQuotaSince(windowStart)
	if err != nil {
		common.ApiErrorMsg(c, "failed to query window consumption: "+err.Error())
		return
	}

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

	dailyAverage := float64(windowQuota) / float64(days)
	data := opsOverviewData{
		TodayQuota: todayQuota,
		// 明确标注口径：前端并排展示两个「今日」时不会误导（盲点 B15）。
		TodayQuotaTimezone:  now.Location().String(),
		TotalQuota:          totalQuota,
		TotalRemainingQuota: remaining.Total,
		UserCount:           remaining.UserCount,
		DailyAverageQuota:   dailyAverage,
		AverageOverDays:     days,
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
