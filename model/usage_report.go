package model

import (
	"fmt"
	"strconv"
	"time"
)

// 用量/成本报表分组维度（仅这三个白名单值会进入 SQL，无注入面）。
const (
	UsageReportGroupByModel   = "model"
	UsageReportGroupByChannel = "channel"
	UsageReportGroupByDay     = "day"
)

// UsageReportRow 是用量/成本报表的一行聚合结果（按维度分组）。
type UsageReportRow struct {
	Key              string `json:"key"`
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	Quota            int64  `json:"quota"`
	RequestBytes     int64  `json:"request_bytes"`
	ResponseBytes    int64  `json:"response_bytes"`
	TotalBytes       int64  `json:"total_bytes"`
}

// IsValidUsageReportGroupBy 校验分组维度。
func IsValidUsageReportGroupBy(groupBy string) bool {
	switch groupBy {
	case UsageReportGroupByModel, UsageReportGroupByChannel, UsageReportGroupByDay:
		return true
	}
	return false
}

// usageReportAggRow 是聚合查询的中间扫描结构（key 或 int_key 二者其一按维度填充）。
type usageReportAggRow struct {
	Key              string `gorm:"column:dim_key"`
	IntKey           int64  `gorm:"column:int_key"`
	Requests         int64  `gorm:"column:requests"`
	PromptTokens     int64  `gorm:"column:prompt_tokens"`
	CompletionTokens int64  `gorm:"column:completion_tokens"`
	TotalTokens      int64  `gorm:"column:total_tokens"`
	Quota            int64  `gorm:"column:quota"`
	RequestBytes     int64  `gorm:"column:request_bytes"`
	ResponseBytes    int64  `gorm:"column:response_bytes"`
	TotalBytes       int64  `gorm:"column:total_bytes"`
}

// GetUsageReport 按维度聚合 consume 日志的用量/成本（报表展示与 CSV 导出共用）。
// groupBy ∈ {model,channel,day}；start/end 为 Unix 秒（end<=0 表示不限上界）。
//
// 三库兼容：只按白名单维度拼 SQL；SUM/COUNT/COALESCE/NULLIF/GROUP BY 为 SQL 标准。
// 日维度用整数运算取本地日期起点——`x - (x % 86400)`（x = created_at + 时区偏移），
// MySQL/SQLite/PostgreSQL 结果一致；刻意不用 `x / 86400`，因为 MySQL 的 `/` 是小数
// 除法（返回 20729.5 而非 20729），跨库不可靠。
func GetUsageReport(groupBy string, start, end int64) ([]UsageReportRow, error) {
	if !IsValidUsageReportGroupBy(groupBy) {
		return nil, fmt.Errorf("invalid group_by: %s", groupBy)
	}

	_, offsetSeconds := time.Now().In(time.Local).Zone()
	offset := int64(offsetSeconds)

	// keyExpr 带别名用于 SELECT；groupExpr/orderExpr 使用完整表达式，避免依赖
	// 「GROUP BY/ORDER BY 是否接受输出别名」的方言差异。
	var keyExpr, groupExpr, orderExpr string
	switch groupBy {
	case UsageReportGroupByModel:
		expr := "COALESCE(NULLIF(model_name, ''), '(unknown)')"
		keyExpr, groupExpr = expr+" AS dim_key", expr
		orderExpr = "COALESCE(SUM(request_bytes), 0) + COALESCE(SUM(response_bytes), 0) DESC"
	case UsageReportGroupByChannel:
		keyExpr, groupExpr = "channel_id AS int_key", "channel_id"
		orderExpr = "COALESCE(SUM(request_bytes), 0) + COALESCE(SUM(response_bytes), 0) DESC"
	case UsageReportGroupByDay:
		expr := fmt.Sprintf("(created_at + %d) - ((created_at + %d) %% 86400)", offset, offset)
		keyExpr, groupExpr = expr+" AS int_key", expr
		orderExpr = groupExpr + " ASC"
	}

	tx := LOG_DB.Model(&Log{}).
		Select(keyExpr,
			"COUNT(*) AS requests",
			"COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens",
			"COALESCE(SUM(completion_tokens), 0) AS completion_tokens",
			"COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS total_tokens",
			"COALESCE(SUM(quota), 0) AS quota",
			"COALESCE(SUM(request_bytes), 0) AS request_bytes",
			"COALESCE(SUM(response_bytes), 0) AS response_bytes",
			"COALESCE(SUM(request_bytes), 0) + COALESCE(SUM(response_bytes), 0) AS total_bytes").
		Where("type = ? AND created_at >= ?", LogTypeConsume, start)
	if end > 0 {
		tx = tx.Where("created_at <= ?", end)
	}

	var aggs []usageReportAggRow
	if err := tx.Group(groupExpr).Order(orderExpr).Scan(&aggs).Error; err != nil {
		return nil, err
	}

	rows := make([]UsageReportRow, 0, len(aggs))
	for _, a := range aggs {
		key := a.Key
		switch groupBy {
		case UsageReportGroupByChannel:
			if a.IntKey == 0 {
				key = "(none)"
			} else {
				key = strconv.FormatInt(a.IntKey, 10)
			}
		case UsageReportGroupByDay:
			key = time.Unix(a.IntKey-offset, 0).In(time.Local).Format("2006-01-02")
		}
		rows = append(rows, UsageReportRow{
			Key:              key,
			Requests:         a.Requests,
			PromptTokens:     a.PromptTokens,
			CompletionTokens: a.CompletionTokens,
			TotalTokens:      a.TotalTokens,
			Quota:            a.Quota,
			RequestBytes:     a.RequestBytes,
			ResponseBytes:    a.ResponseBytes,
			TotalBytes:       a.TotalBytes,
		})
	}
	return rows, nil
}
