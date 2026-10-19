package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/types"
)

// SystemPerformanceCheck 检查系统性能中间件
func SystemPerformanceCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅检查 Relay 接口 (/v1, /v1beta 等)
		// 这里简单判断路径前缀，可以根据实际路由调整
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/v1/messages") {
			if err := checkSystemPerformance(); err != nil {
				c.JSON(err.StatusCode, gin.H{
					"error": err.ToClaudeError(),
				})
				c.Abort()
				return
			}
		} else {
			if err := checkSystemPerformance(); err != nil {
				c.JSON(err.StatusCode, gin.H{
					"error": err.ToOpenAIError(),
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// checkSystemPerformance 检查系统性能是否超过阈值。
//
// 过载标记来自**连续采样**（约 15 秒）且要求采样新鲜（<=30 秒），
// 单一瞬时尖峰或采样停滞都不会熔断流量；阈值仍按当前配置实时复核。
func checkSystemPerformance() *types.NewAPIError {
	config := common.GetPerformanceMonitorConfig()
	overload := common.CheckSystemOverload(common.GetSystemStatus(), config, time.Now())
	if overload == nil {
		return nil
	}
	return types.NewErrorWithStatusCode(
		errors.New(overload.Error()),
		types.ErrorCode(overload.Code),
		http.StatusServiceUnavailable)
}
