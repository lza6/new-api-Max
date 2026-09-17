package router

import (
	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/controller"
	"github.com/lza6/new-api-Max/middleware"
)

// SetWebProtectionRouter 注册 Web 防刷/日志/封禁管理 API（仅管理员）。
func SetWebProtectionRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	admin := apiRouter.Group("/admin")
	admin.Use(middleware.AdminAuth())
	admin.GET("/web-protection/settings", controller.GetWebProtectionSettings)
	admin.PUT("/web-protection/settings", controller.UpdateWebProtectionSettings)
	admin.GET("/web-request-logs", controller.ListWebRequestLogs)
	admin.GET("/web-request-logs/detail", controller.ListWebRequestLogDetail)
	admin.GET("/banned-ips", controller.ListBannedIPsController)
	admin.POST("/banned-ips", controller.BanIPController)
	admin.POST("/banned-ips/unban", controller.UnbanIPController)
	admin.DELETE("/banned-ips/:id", controller.DeleteBannedIPController)
}
