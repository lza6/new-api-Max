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
		// 全流量吞吐统计（覆盖所有请求，含 /v1；仅计量不限流）。
		if c.Request != nil {
			if cl := c.Request.ContentLength; cl > 0 {
				service.RecordNetworkBytes(cl, 0)
			}
		}
		if !service.TrackWebRequestBegin(c) {
			service.RecordNetworkBytes(0, int64(c.Writer.Size()))
			return
		}
		c.Next()
		service.TrackWebRequestEnd(c, c.Writer.Status())
		if c.Writer != nil {
			service.RecordNetworkBytes(0, int64(c.Writer.Size()))
		}
	}
}
