package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

func GetRankings(c *gin.Context) {
	result, err := service.GetRankingsSnapshot(c.DefaultQuery("period", "week"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetRankingsBandwidth 公开模型流量排行：按模型聚合 request/response 字节，降序限量。
// GET /api/rankings/bandwidth?days=30&limit=10
func GetRankingsBandwidth(c *gin.Context) {
	days := 30
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = min(n, 3650)
		}
	}
	limit := 10
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = min(n, 100)
		}
	}
	byModel, err := queryModelBandwidthLeaderboard(days, limit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "failed to query model bandwidth leaderboard: " + err.Error(),
		})
		return
	}
	type row struct {
		Model     string `json:"model"`
		Requests  int64  `json:"requests"`
		Bytes     int64  `json:"bytes"`
		BytesText string `json:"bytes_text"`
	}
	out := make([]row, 0, len(byModel))
	for _, m := range byModel {
		out = append(out, row{
			Model:     m.Model,
			Requests:  m.Requests,
			Bytes:     m.Bytes,
			BytesText: common.FormatBytes(m.Bytes),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"days":        days,
			"limit":       limit,
			"period_end":  time.Now().Unix(),
			"leaderboard": out,
		},
	})
}

// GetCompressionStats 返回按模型的出站压缩统计（次数/原始字节/压缩后/节省字节），
// 持久化到 DB，重启不丢（§用户需求）。公开只读，供排行榜与模型广场展示。
// GET /api/rankings/compression?limit=50
func GetCompressionStats(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = min(n, 500)
		}
	}
	rows, err := model.GetModelCompressionStats()
	if err != nil {
		common.ApiErrorMsg(c, "failed to load compression stats")
		return
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	type row struct {
		ModelName       string  `json:"model_name"`
		Count           int64   `json:"count"`
		OriginalBytes   int64   `json:"original_bytes"`
		CompressedBytes int64   `json:"compressed_bytes"`
		SavedBytes      int64   `json:"saved_bytes"`
		SavedGB         float64 `json:"saved_gb"`
		SavedMB         float64 `json:"saved_mb"`
		Ratio           float64 `json:"ratio"` // 压缩后/原始，越小越好
	}
	out := make([]row, 0, len(rows))
	for _, r := range rows {
		ratio := 0.0
		if r.OriginalBytes > 0 {
			ratio = float64(r.CompressedBytes) / float64(r.OriginalBytes)
		}
		out = append(out, row{
			ModelName:       r.ModelName,
			Count:           r.Count,
			OriginalBytes:   r.OriginalBytes,
			CompressedBytes: r.CompressedBytes,
			SavedBytes:      r.SavedBytes,
			SavedGB:         float64(r.SavedBytes) / (1024 * 1024 * 1024),
			SavedMB:         float64(r.SavedBytes) / (1024 * 1024),
			Ratio:           ratio,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// GetClientStats 返回客户端使用统计（各客户端占比 + 各模型客户端占比 + 平均缓存率）。
// GET /api/rankings/clients?days=7&models=20
func GetClientStats(c *gin.Context) {
	days := 7
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = min(n, 365)
		}
	}
	models := 20
	if v := c.Query("models"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			models = min(n, 100)
		}
	}
	res, err := service.GetClientStats(days, models)
	if err != nil {
		common.ApiErrorMsg(c, "failed to build client stats")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

// GetSavingsBaseline T1：统一节省口径 + 反事实基准。
//
// 基于已持久化的按模型压缩统计（model_compression_stats）计算「若未压缩」的
// 反事实：不压缩就要上传的原始字节数、按实测压缩率折算的额外上传量，以及
// 在观测到的上游带宽下可省的时间。口径统一为「节省字节 / 原始字节」。
// GET /api/rankings/savings-baseline?limit=50
func GetSavingsBaseline(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = min(n, 500)
		}
	}
	rows, err := model.GetModelCompressionStats()
	if err != nil {
		common.ApiErrorMsg(c, "failed to load compression stats")
		return
	}
	baseline := service.ComputeSavingsBaseline(rows)
	if len(baseline.Models) > limit {
		baseline.Models = baseline.Models[:limit]
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": baseline})
}
