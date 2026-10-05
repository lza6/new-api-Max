package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// GetChannelHealthScores 返回所有渠道的 B3-3 健康分快照 + §4.8.2 熔断状态（管理端）。
// 数据源为进程内滑动窗口（近 1h），无样本渠道返回零值快照。
// 每渠道附 circuit 字段（closed/open/half_open + 连续失败次数），供运维观测熔断行为。
func GetChannelHealthScores(c *gin.Context) {
	var ids []int
	if err := model.DB.Model(&model.Channel{}).Pluck("id", &ids).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	type channelHealthEntry struct {
		service.ChannelHealthSnapshot
		CircuitState     string `json:"circuit_state"`
		ConsecutiveFails int    `json:"consecutive_failures"`
	}
	scores := make(map[int]channelHealthEntry, len(ids))
	for _, id := range ids {
		state, fails := service.GetChannelCircuitState(id)
		scores[id] = channelHealthEntry{
			ChannelHealthSnapshot: service.GetChannelHealthSnapshot(id),
			CircuitState:          string(state),
			ConsecutiveFails:      fails,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    scores,
	})
}
