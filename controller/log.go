package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/middleware"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"

	"github.com/gin-gonic/gin"
)

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if c.GetInt("role") < common.RoleRootUser {
		model.FormatAdminLogs(logs)
	} else {
		model.FormatRootLogs(logs)
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	httpStats := middleware.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota":                 stat.Quota,
			"rpm":                   stat.Rpm,
			"tpm":                   stat.Tpm,
			"concurrent_requests":   httpStats.ActiveConnections,
			"completed_last_minute": httpStats.CompletedLastMinute,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum.Quota,
			"rpm":   quotaNum.Rpm,
			"tpm":   quotaNum.Tpm,
			//"token": tokenNum,
		},
	})
	return
}

// GetLogsTraffic 管理端流量统计：按日聚合每请求 request_bytes+response_bytes。
// GET /api/log/traffic?days=1|7|30
func GetLogsTraffic(c *gin.Context) {
	days := 1
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = min(n, 90)
		}
	}
	start := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	var rows []service.TrafficRecord
	if err := model.LOG_DB.Model(&model.Log{}).
		Select("created_at", "other").
		Where("type = ? AND created_at >= ?", model.LogTypeConsume, start).
		Scan(&rows).Error; err != nil {
		common.ApiErrorMsg(c, "failed to query traffic: "+err.Error())
		return
	}
	byDay := service.AggregateTrafficByDay(rows, time.Local)
	var total int64
	for i := range byDay {
		total += byDay[i].Bytes
	}
	common.ApiSuccess(c, gin.H{
		"days":           days,
		"total_requests": len(rows),
		"total_bytes":    total,
		"total_mb":       float64(total) / (1024 * 1024),
		"by_day":         byDay,
	})
}

// GetSiteOverview 站点权威统计：累计处理请求数 / 提供总带宽 / token 总数 / 总消耗额度。
// GET /api/log/overview?days=0|N（0=累计至今，N=近 N 天）。
// 带宽与 token 直接对持久化列 SUM，避免全表扫 other JSON（慢查询友好）。
func GetSiteOverview(c *gin.Context) {
	days := 0
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = min(n, 3650)
		}
	}
	tx := model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume)
	if days > 0 {
		tx = tx.Where("created_at >= ?", time.Now().Add(-time.Duration(days)*24*time.Hour).Unix())
	}
	var agg struct {
		TotalRequests int64
		TotalBytes    int64
		TotalTokens   int64
		TotalQuota    int64
	}
	if err := tx.Select(
		"COUNT(*) AS total_requests, " +
			"COALESCE(SUM(request_bytes),0)+COALESCE(SUM(response_bytes),0) AS total_bytes, " +
			"COALESCE(SUM(prompt_tokens),0)+COALESCE(SUM(completion_tokens),0) AS total_tokens, " +
			"COALESCE(SUM(quota),0) AS total_quota",
	).Scan(&agg).Error; err != nil {
		common.ApiErrorMsg(c, "failed to query site overview: "+err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{
		"days":             days,
		"total_requests":   agg.TotalRequests,
		"total_bytes":      agg.TotalBytes,
		"total_bytes_text": common.FormatBytes(agg.TotalBytes),
		"total_tokens":     agg.TotalTokens,
		"total_quota":      agg.TotalQuota,
	})
}

// GetBandwidthLeaderboard 管理端带宽日排行：按日分组、按带宽降序、限量。
// GET /api/log/bandwidth/leaderboard?days=30&limit=10
func GetBandwidthLeaderboard(c *gin.Context) {
	days := 30
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = min(n, 3650)
		}
	}
	limit := 10
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = min(n, 1000)
		}
	}
	var rows []service.TrafficBytesRecord
	if err := model.LOG_DB.Model(&model.Log{}).
		Select("created_at", "request_bytes", "response_bytes").
		Where("type = ? AND created_at >= ?", model.LogTypeConsume,
			time.Now().Add(-time.Duration(days)*24*time.Hour).Unix()).
		Scan(&rows).Error; err != nil {
		common.ApiErrorMsg(c, "failed to query bandwidth leaderboard: "+err.Error())
		return
	}
	byDay := service.AggregateBandwidthByDay(rows, time.Local, limit)
	type row struct {
		Date      string `json:"date"`
		Requests  int64  `json:"requests"`
		Bytes     int64  `json:"bytes"`
		BytesText string `json:"bytes_text"`
	}
	out := make([]row, 0, len(byDay))
	for _, d := range byDay {
		out = append(out, row{Date: d.Date, Requests: d.Requests, Bytes: d.Bytes, BytesText: common.FormatBytes(d.Bytes)})
	}
	common.ApiSuccess(c, gin.H{"days": days, "limit": limit, "leaderboard": out})
}

// GetModelStats 模型广场卡片统计：每个模型今日/近 30 天调用总数与成功数（站点级聚合，
// 无用户维度、无敏感字段）。成功 = consume 计费日志数；总数 = consume + error。
// GET /api/model/stats
func GetModelStats(c *gin.Context) {
	now := time.Now()
	loc := now.Location()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Unix()
	start30d := now.AddDate(0, 0, -30).Unix()
	groupCount := func(logType int, from int64) (map[string]int64, error) {
		var rows []struct {
			ModelName string
			C         int64
		}
		if err := model.LOG_DB.Model(&model.Log{}).
			Select("model_name, COUNT(*) AS c").
			Where("type = ? AND created_at >= ? AND model_name <> ''", logType, from).
			Group("model_name").Scan(&rows).Error; err != nil {
			return nil, err
		}
		out := make(map[string]int64, len(rows))
		for _, r := range rows {
			out[r.ModelName] = r.C
		}
		return out, nil
	}
	consumeToday, err := groupCount(model.LogTypeConsume, startOfToday)
	if err != nil {
		common.ApiErrorMsg(c, "failed to query model stats: "+err.Error())
		return
	}
	errorToday, err := groupCount(model.LogTypeError, startOfToday)
	if err != nil {
		common.ApiErrorMsg(c, "failed to query model stats: "+err.Error())
		return
	}
	consume30, err := groupCount(model.LogTypeConsume, start30d)
	if err != nil {
		common.ApiErrorMsg(c, "failed to query model stats: "+err.Error())
		return
	}
	error30, err := groupCount(model.LogTypeError, start30d)
	if err != nil {
		common.ApiErrorMsg(c, "failed to query model stats: "+err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"stats": service.MergeModelStats(consumeToday, errorToday, consume30, error30)})
}
