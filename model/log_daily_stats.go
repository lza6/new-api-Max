package model

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/common"
)

// 日志日汇总的维度。当前只落地 total 与 model 两个：
// 它们覆盖了最热门的四个接口（日流量、带宽日排行、模型带宽排行、站点权威统计），
// 且维度值是字符串，不需要 CAST，天然满足 SQLite/MySQL/PostgreSQL 三库一致。
// channel/group/user/token 等数字维度暂不预聚合——数字转文本的 CAST 三库语法不同，
// 强行做会引入方言分支；六维报表（/api/log/report）继续走原查询。
const (
	LogDimTotal = "total"
	LogDimModel = "model"
)

// logDailyStatDimLabel 用于日志与错误信息，避免魔法字符串散落。
var logDailyStatDimLabel = map[string]string{
	LogDimTotal: "total",
	LogDimModel: "by-model",
}

// LogDailyStat 是按天预聚合的消费日志统计。
//
// 为什么需要它：日流量、带宽排行、站点权威统计都要按时间窗口聚合 logs 表，而
// logs 每天新增约 7 万行、已 184 万行 / 3.5GB。即使有覆盖索引，30 天窗口仍要扫
// 130 万索引项（实测 1.8s），且随天数线性恶化——一年后会回到 4-5s。
// 预聚合后这些查询退化为读 ≤ 天数×维度 行（30 天 × 2 维 ≈ 60 行），毫秒级返回。
//
// 数据新鲜度：由 RefreshLogDailyStats 定时重算最近若干天，延迟上限 = 刷新间隔。
// 查询侧有兜底：表为空、或最新一天未覆盖时，回退到直接扫 logs 的原路径。
type LogDailyStat struct {
	Id       int    `json:"id" gorm:"primaryKey"`
	DayKey   int64  `json:"day_key" gorm:"bigint;not null;uniqueIndex:idx_lds_dim,priority:1"`
	DimType  string `json:"dim_type" gorm:"type:varchar(16);not null;uniqueIndex:idx_lds_dim,priority:2"`
	DimValue string `json:"dim_value" gorm:"type:varchar(191);not null;default:'';uniqueIndex:idx_lds_dim,priority:3"`
	Requests int64  `json:"requests" gorm:"bigint;default:0"`
	Bytes    int64  `json:"bytes" gorm:"bigint;default:0"`
	// PromptTokens/CompletionTokens/Quota 供站点权威统计与财务口径使用。
	PromptTokens     int64 `json:"prompt_tokens" gorm:"bigint;default:0"`
	CompletionTokens int64 `json:"completion_tokens" gorm:"bigint;default:0"`
	Quota            int64 `json:"quota" gorm:"bigint;default:0"`
	// UpdatedAt 是本次重算的 Unix 秒，便于排查「汇总落后」问题。
	UpdatedAt int64 `json:"updated_at" gorm:"bigint;default:0"`
}

// LogDailyAggRow 是聚合查询的扫描目标（既是 logs 的聚合结果，也是汇总表的内容）。
type LogDailyAggRow struct {
	DayKey           int64  `gorm:"column:day_key"`
	DimValue         string `gorm:"column:dim_value"`
	Requests         int64  `gorm:"column:requests"`
	Bytes            int64  `gorm:"column:bytes"`
	PromptTokens     int64  `gorm:"column:prompt_tokens"`
	CompletionTokens int64  `gorm:"column:completion_tokens"`
	Quota            int64  `gorm:"column:quota"`
}

// LogDayKeyExpr 返回「本地日期零点」的 SQL 表达式。
//
// 用 `x - (x % 86400)` 而不是 `x / 86400`：MySQL 的 `/` 是小数除法，结果扫进
// int64 会失败。取模写法在 SQLite/MySQL/PostgreSQL 上都是整数运算且结果一致，
// 详见 model/usage_report.go 的同名说明。
func LogDayKeyExpr(tzOffset int64) string {
	return fmt.Sprintf("(created_at + %d) - ((created_at + %d) %% 86400)", tzOffset, tzOffset)
}

// aggregateLogsByDay 按天（可选再加一个字符串维度）聚合消费日志。
// dimExpr 传 SQL 表达式；空字符串表示不分组维度（口径 = total）。
func aggregateLogsByDay(db *gorm.DB, tzOffset int64, fromTimestamp int64, dimExpr string) ([]LogDailyAggRow, error) {
	dimSelect := "'' AS dim_value"
	groupBy := "day_key"
	if dimExpr != "" {
		dimSelect = dimExpr + " AS dim_value"
		groupBy = "day_key, dim_value"
	}
	var rows []LogDailyAggRow
	err := db.Table("logs").
		Select(LogDayKeyExpr(tzOffset)+" AS day_key, "+dimSelect+", "+
			"COUNT(*) AS requests, "+
			"COALESCE(SUM(request_bytes),0) + COALESCE(SUM(response_bytes),0) AS bytes, "+
			"COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, "+
			"COALESCE(SUM(completion_tokens),0) AS completion_tokens, "+
			"COALESCE(SUM(quota),0) AS quota",
		).
		Where("type = ? AND created_at >= ?", LogTypeConsume, fromTimestamp).
		Group(groupBy).
		Scan(&rows).Error
	return rows, err
}

// RefreshLogDailyStats 重算最近 days 天（含今天）的日汇总。
//
// 幂等：先删除窗口内旧行，再整批写入。用「删+插」而不是 UPSERT，是因为
// GORM 的 OnConflict 在 MySQL 上渲染不稳定（历史踩过空子句的坑），删+插在三库
// 行为一致且容易推理。窗口很小（默认 2 天 × 2 维），代价可忽略。
func RefreshLogDailyStats(days int) error {
	if LOG_DB == nil {
		return nil
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil
	}
	if days <= 0 {
		days = 2
	}
	_, tzOffsetInt := time.Now().In(time.Local).Zone()
	tzOffset := int64(tzOffsetInt)
	fromTs := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	fromDay := (fromTs + tzOffset) - ((fromTs + tzOffset) % 86400)

	dims := []struct{ dimType, expr string }{
		{LogDimTotal, ""},
		{LogDimModel, "COALESCE(NULLIF(model_name, ''), '(unknown)')"},
	}

	now := time.Now().Unix()
	for _, d := range dims {
		rows, err := aggregateLogsByDay(LOG_DB, tzOffset, fromTs, d.expr)
		if err != nil {
			return fmt.Errorf("refresh log daily stats (%s): aggregate: %w", d.dimType, err)
		}

		err = LOG_DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("dim_type = ? AND day_key >= ?", d.dimType, fromDay).
				Delete(&LogDailyStat{}).Error; err != nil {
				return fmt.Errorf("delete old rows: %w", err)
			}
			if len(rows) == 0 {
				return nil
			}
			records := make([]LogDailyStat, 0, len(rows))
			for _, r := range rows {
				records = append(records, LogDailyStat{
					DayKey:           r.DayKey,
					DimType:          d.dimType,
					DimValue:         r.DimValue,
					Requests:         r.Requests,
					Bytes:            r.Bytes,
					PromptTokens:     r.PromptTokens,
					CompletionTokens: r.CompletionTokens,
					Quota:            r.Quota,
					UpdatedAt:        now,
				})
			}
			if err := tx.CreateInBatches(&records, 200).Error; err != nil {
				return fmt.Errorf("insert rows: %w", err)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("refresh log daily stats (%s): %w", d.dimType, err)
		}
	}
	return nil
}

// DailyStatsEmpty 判断汇总表是否为空（首次部署 / 换了日志库）。
// 为空时刷新任务需要做历史回填，否则统计页会显示 0。
func DailyStatsEmpty() bool {
	if LOG_DB == nil || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return true
	}
	var count int64
	if err := LOG_DB.Model(&LogDailyStat{}).
		Where("dim_type = ?", LogDimTotal).
		Limit(1).
		Count(&count).Error; err != nil {
		return true
	}
	return count == 0
}

// DailyStatsCovers 判断汇总表是否完整覆盖 [fromDay, 今天]。
//
// 只判断「表非空」是不够的：历史尚未回填时，30 天查询会静默漏掉前面若干天，
// 表现为统计数字莫名其妙变小——这比慢更糟。因此要求最早一天不晚于查询起点，
// 且最新一天已刷新（允许 1 天滞后，容忍跨零点与刷新窗口）。
func DailyStatsCovers(fromDay int64) bool {
	if LOG_DB == nil || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return false
	}
	var span struct {
		MinDay int64 `gorm:"column:min_day"`
		MaxDay int64 `gorm:"column:max_day"`
	}
	if err := LOG_DB.Model(&LogDailyStat{}).
		Select("COALESCE(MIN(day_key),0) AS min_day, COALESCE(MAX(day_key),0) AS max_day").
		Where("dim_type = ?", LogDimTotal).
		Scan(&span).Error; err != nil {
		return false
	}
	if span.MaxDay == 0 {
		return false
	}
	now := time.Now().Unix()
	today := DayKeyFor(now)
	return span.MaxDay >= today-86400 && span.MinDay <= fromDay
}

// DayKeyFor 返回某个 Unix 时间戳所属的「本地日期零点」day_key。
// 用纯整数运算，与 SQL 侧的 LogDayKeyExpr 保持同一口径。
func DayKeyFor(ts int64) int64 {
	_, tzOffsetInt := time.Now().In(time.Local).Zone()
	x := ts + int64(tzOffsetInt)
	return x - (x % 86400)
}

// DayKeyToDate 把 day_key 还原成本地日期字符串（2006-01-02）。
func DayKeyToDate(dayKey int64) string {
	_, tzOffsetInt := time.Now().In(time.Local).Zone()
	return time.Unix(dayKey-int64(tzOffsetInt), 0).In(time.Local).Format("2006-01-02")
}

// GetDailyStatsByDay 从汇总表读按天聚合（dim=total），返回按 day_key 升序。
func GetDailyStatsByDay(fromDay int64) ([]LogDailyAggRow, error) {
	var rows []LogDailyAggRow
	err := LOG_DB.Model(&LogDailyStat{}).
		Select("day_key, '' AS dim_value, SUM(requests) AS requests, SUM(bytes) AS bytes, "+
			"SUM(prompt_tokens) AS prompt_tokens, SUM(completion_tokens) AS completion_tokens, SUM(quota) AS quota").
		Where("dim_type = ? AND day_key >= ?", LogDimTotal, fromDay).
		Group("day_key").
		Order("day_key ASC").
		Scan(&rows).Error
	return rows, err
}

// GetModelStatsByDay 从汇总表读各模型按天的聚合（dim=model）。
func GetModelStatsByDay(fromDay int64) ([]LogDailyAggRow, error) {
	var rows []LogDailyAggRow
	err := LOG_DB.Model(&LogDailyStat{}).
		Select("day_key, dim_value, SUM(requests) AS requests, SUM(bytes) AS bytes, "+
			"SUM(prompt_tokens) AS prompt_tokens, SUM(completion_tokens) AS completion_tokens, SUM(quota) AS quota").
		Where("dim_type = ? AND day_key >= ?", LogDimModel, fromDay).
		Group("day_key, dim_value").
		Order("day_key ASC").
		Scan(&rows).Error
	return rows, err
}

// GetSiteTotalsFromDailyStats 从汇总表读站点权威统计的累计口径。
// fromDay=0 表示累计至今。
func GetSiteTotalsFromDailyStats(fromDay int64) (LogDailyAggRow, error) {
	var row LogDailyAggRow
	tx := LOG_DB.Model(&LogDailyStat{}).
		Select("COALESCE(SUM(requests),0) AS requests, COALESCE(SUM(bytes),0) AS bytes, "+
			"COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, "+
			"COALESCE(SUM(completion_tokens),0) AS completion_tokens, "+
			"COALESCE(SUM(quota),0) AS quota").
		Where("dim_type = ?", LogDimTotal)
	if fromDay > 0 {
		tx = tx.Where("day_key >= ?", fromDay)
	}
	err := tx.Scan(&row).Error
	return row, err
}
