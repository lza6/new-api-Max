package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// GetSiteSubscriptionStats T15-A：公开只读站点订阅运营统计。
// 只返回聚合计数（无用户/订单明细），供站点运营透明度展示。
// §4.1.5：响应走 30s 短缓存（env SUBSCRIPTION_STATS_CACHE_SECONDS 可调，0=关）。
func GetSiteSubscriptionStats(c *gin.Context) {
	stats, err := model.GetSiteSubscriptionStatsCached()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": stats})
}
