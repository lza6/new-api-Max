package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/service"
)

// GetPublicStatus 公开状态页数据端点（G7）。
//
// GET /api/status/page
//
// ## 为什么另起一个路径，而不是复用 /api/status
//
// `/api/status` 是 SPA 启动时读取的**配置端点**（GetStatus：站点名、开关、OAuth 配置…），
// 已登录/未登录前端都依赖它的响应结构；往里塞运行状态会同时踩两个坑：把配置响应变大，
// 且一旦将来调整状态字段就可能影响前端启动。因此状态页用独立子路径，契约互不干扰。
//
// ## 为什么公开
//
// 与 `/api/site/stats`、`/api/site/visitors` 同级：只输出**可证明的运行事实**
// （依赖探测结果、近 24h 日志计数、后台心跳），不含任何用户数据、渠道信息、
// 内网地址或错误文本。聚合全部经 SWR 缓存（见 service.GetPublicStatus），
// 请求路径上不做同步聚合。
func GetPublicStatus(c *gin.Context) {
	status, err := service.GetPublicStatus()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}
