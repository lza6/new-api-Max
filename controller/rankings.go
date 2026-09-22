package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
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
