package router

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/middleware"

	"github.com/gin-gonic/gin"
)

func SetRouter(router *gin.Engine, assets WebAssets) {
	// §4.1.4：Prometheus 文本格式指标端点（env METRICS_ENABLED=true 开放，默认关）。
	if strings.EqualFold(os.Getenv("METRICS_ENABLED"), "true") {
		router.GET("/metrics", func(c *gin.Context) {
			c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			c.String(http.StatusOK, common.RenderPrometheusMetrics())
		})
	}
	SetApiRouter(router)
	SetWebProtectionRouter(router)
	SetWebhookRouter(router)
	SetDashboardRouter(router)
	SetRelayRouter(router)
	SetTaskPluginProtocolRouter(router)
	SetVideoRouter(router)
	SetTaskRouter(router)
	pluginDispatcher := SetPluginRouter(router)
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if common.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		common.SysLog("FRONTEND_BASE_URL is ignored on master node")
	}
	if frontendBaseUrl == "" {
		SetWebRouter(router, assets, pluginDispatcher)
	} else {
		frontendBaseUrl = strings.TrimSuffix(frontendBaseUrl, "/")
		router.NoRoute(
			pluginDispatcher,
			middleware.RouteTag("web"),
			middleware.AccessTokenAudit(),
			func(c *gin.Context) {
				c.Redirect(http.StatusMovedPermanently, fmt.Sprintf("%s%s", frontendBaseUrl, c.Request.RequestURI))
			},
		)
	}
}
