package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/dto"
	"github.com/lza6/new-api-Max/logger"
	pluginruntime "github.com/lza6/new-api-Max/pkg/jsplugin"
	"github.com/lza6/new-api-Max/relaykit/types"
)

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	userId := c.GetInt("id")
	_, preparedPluginRoute := c.Get(pluginruntime.ContextKeyRouteRequest)
	if !preparedPluginRoute || !RespondTaskPluginError(c, &dto.TaskError{
		Code:       codeStr,
		Message:    message,
		StatusCode: statusCode,
	}) {
		c.JSON(statusCode, gin.H{
			"error": gin.H{
				"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
				"type":    "new_api_error",
				"code":    codeStr,
			},
		})
	}
	c.Abort()
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
}

func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "new_api_error",
		"code":        code,
	})
	c.Abort()
	logger.LogError(c.Request.Context(), description)
}
