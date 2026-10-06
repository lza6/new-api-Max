package router

import (
	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/controller"
	"github.com/lza6/new-api-Max/middleware"
)

// SetWebhookRouter 注册全局 Webhook 事件通知配置 API（仅管理员）。
func SetWebhookRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	admin := apiRouter.Group("/admin")
	admin.Use(middleware.AdminAuth())
	admin.GET("/webhook/settings", controller.GetWebhookSettings)
	admin.PUT("/webhook/settings", controller.UpdateWebhookSettings)
	// B2-3 多端点管理。
	admin.GET("/webhook/endpoints", controller.GetWebhookEndpoints)
	admin.POST("/webhook/endpoints", controller.CreateWebhookEndpoint)
	admin.PUT("/webhook/endpoints/:id", controller.UpdateWebhookEndpoint)
	admin.DELETE("/webhook/endpoints/:id", controller.DeleteWebhookEndpoint)
}
