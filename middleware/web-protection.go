package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/service"
)

// WebProtection Web 层防刷/限流/封禁中间件。
// 仅作用于非 /v1 前缀请求；/v1 模型中继 API 完全跳过（内部判定，零开销）。
func WebProtection() gin.HandlerFunc {
	service.InitWebProtectionTracker()
	return func(c *gin.Context) {
		if !service.TrackWebRequestBegin(c) {
			return
		}
		c.Next()
		service.TrackWebRequestEnd(c, c.Writer.Status())
	}
}
