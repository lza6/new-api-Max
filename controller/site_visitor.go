package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/service"
)

// GetSiteVisitorSummary 公开只读的站点统计端点（Batch-13）。
//
// GET /api/site/visitors
//
// 返回：访客数（今日 / 近 7 天 / 近 30 天 / 累计，按 IP 按天去重且排除爬虫）、
// 页面访问次数、注册用户总数、活跃用户数（近 24h / 近 30 天，以 API 消费为口径）、
// 以及近 30 天的流量来源分布。
//
// ## 为什么放在公开路由
//
// 与 `/api/site/stats`（已有）同级：都是**无 PII 的聚合量**，用于首页透明度展示。
// 响应中不含任何用户标识、IP、单个访客的行数据 —— IP 在入库前已做 HMAC 匿名化
// （见 model.SiteDailyVisitor 的隐私说明），本端点连哈希都不返回。
//
// ## 为什么带缓存
//
// 见 service.GetSiteVisitorSummary：60s 缓存把匿名爬虫的任意请求频率折叠成
// 每分钟一次查询。
func GetSiteVisitorSummary(c *gin.Context) {
	summary, err := service.GetSiteVisitorSummary()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}
