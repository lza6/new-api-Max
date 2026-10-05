package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"

	"github.com/gin-gonic/gin"
)

// usageReportColumns 是报表 CSV 的列头（与下方每行写入顺序一一对应）。
var usageReportColumns = []string{
	"dimension", "requests", "prompt_tokens", "completion_tokens",
	"total_tokens", "quota", "request_bytes", "response_bytes", "total_bytes",
}

// parseUsageReportParams 解析并校验报表查询参数（group_by/start/end）。
// start 默认近 30 天；end 默认当前时间。时间戳为 Unix 秒。
func parseUsageReportParams(c *gin.Context) (groupBy string, start, end int64, ok bool) {
	groupBy = c.DefaultQuery("group_by", model.UsageReportGroupByModel)
	if !model.IsValidUsageReportGroupBy(groupBy) {
		common.ApiErrorMsg(c, "invalid group_by (expected model|channel|day)")
		return "", 0, 0, false
	}
	now := time.Now().Unix()
	end = now
	if raw := c.Query("end"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			common.ApiErrorMsg(c, "invalid end (expected positive unix seconds)")
			return "", 0, 0, false
		}
		end = v
	}
	start = now - 30*24*3600
	if raw := c.Query("start"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			common.ApiErrorMsg(c, "invalid start (expected positive unix seconds)")
			return "", 0, 0, false
		}
		start = v
	}
	// 显式参数非法：不静默交换（掩盖调用方 bug），返回 400。
	if start > end {
		common.ApiErrorMsg(c, "start must not be after end")
		return "", 0, 0, false
	}
	return groupBy, start, end, true
}

// GetUsageReport 管理端用量/成本聚合报表（按模型/渠道/日）。
// GET /api/log/report?group_by=model|channel|day&start=&end=
func GetUsageReport(c *gin.Context) {
	groupBy, start, end, ok := parseUsageReportParams(c)
	if !ok {
		return
	}
	rows, err := model.GetUsageReport(groupBy, start, end)
	if err != nil {
		common.SysError("usage report build failed: " + err.Error())
		common.ApiErrorMsg(c, "failed to build usage report")
		return
	}
	var totals model.UsageReportRow
	for _, r := range rows {
		totals.Requests += r.Requests
		totals.PromptTokens += r.PromptTokens
		totals.CompletionTokens += r.CompletionTokens
		totals.TotalTokens += r.TotalTokens
		totals.Quota += r.Quota
		totals.RequestBytes += r.RequestBytes
		totals.ResponseBytes += r.ResponseBytes
		totals.TotalBytes += r.TotalBytes
	}
	common.ApiSuccess(c, gin.H{
		"group_by":  groupBy,
		"start":     start,
		"end":       end,
		"rows":      rows,
		"totals":    totals,
		"totals_mb": float64(totals.TotalBytes) / (1024 * 1024),
		"row_count": len(rows),
	})
}

// ExportUsageReportCSV 用量/成本报表 CSV 流式导出（同样支持 group_by/start/end）。
// GET /api/log/report/export?group_by=...&start=&end=
//
// 流式写：聚合在 DB 端完成（每组一行），导出行数 = 分组数（≤ 模型/渠道数或天数），
// 内存不随日志总量增长，可安全导出长窗口。
func ExportUsageReportCSV(c *gin.Context) {
	groupBy, start, end, ok := parseUsageReportParams(c)
	if !ok {
		return
	}
	rows, err := model.GetUsageReport(groupBy, start, end)
	if err != nil {
		common.SysError("usage report build failed: " + err.Error())
		common.ApiErrorMsg(c, "failed to build usage report")
		return
	}

	filename := fmt.Sprintf("usage-report-%s-%s.csv", groupBy, time.Now().Format("20060102"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	// UTF-8 BOM，便于 Excel 正确识别中文/编码。
	if _, err := c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}

	w := csv.NewWriter(c.Writer)
	_ = w.Write(usageReportColumns)
	for _, r := range rows {
		_ = w.Write([]string{
			csvSafeCell(r.Key),
			strconv.FormatInt(r.Requests, 10),
			strconv.FormatInt(r.PromptTokens, 10),
			strconv.FormatInt(r.CompletionTokens, 10),
			strconv.FormatInt(r.TotalTokens, 10),
			strconv.FormatInt(r.Quota, 10),
			strconv.FormatInt(r.RequestBytes, 10),
			strconv.FormatInt(r.ResponseBytes, 10),
			strconv.FormatInt(r.TotalBytes, 10),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		// 头已发送，无法改状态码；记录即可。
		common.SysError("usage report csv flush failed: " + err.Error())
	}
	// gin 会在 handler 返回后按状态码结束；显式 200 确保浏览器按附件处理。
	c.Status(http.StatusOK)
}

// csvSafeCell 防 CSV 公式注入（CSV injection）：以 = + - @ 或制表/回车开头的文本单元格
// 前置单引号，避免在 Excel/Sheets 中被当作公式执行（模型/渠道名可能含此类首字符）。
func csvSafeCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
