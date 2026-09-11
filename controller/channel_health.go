package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// GetChannelHealthScores 返回所有渠道的 B3-3 健康分快照（管理端）。
// 数据源为进程内滑动窗口（近 1h），无样本渠道返回零值快照。
func GetChannelHealthScores(c *gin.Context) {
	var ids []int
	if err := model.DB.Model(&model.Channel{}).Pluck("id", &ids).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	scores := make(map[int]service.ChannelHealthSnapshot, len(ids))
	for _, id := range ids {
		scores[id] = service.GetChannelHealthSnapshot(id)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    scores,
	})
}
