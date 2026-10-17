package router

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/controller"
	"github.com/lza6/new-api-Max/middleware"

	"github.com/gin-gonic/gin"
)

func SetRouter(router *gin.Engine, assets WebAssets) {
	// §4.1.4 / B3-1：Prometheus 文本格式指标端点（env METRICS_ENABLED=true 开放，默认关）。
	// 鉴权：仅可信来源（环回/私网）或 root 会话可读，避免指标泄漏到公网。
	if middleware.MetricsEnabled() {
		router.GET("/metrics", middleware.MetricsAuth(), func(c *gin.Context) {
			c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			c.String(http.StatusOK, common.RenderPrometheusMetrics())
		})
	}
	// §Health Checks 服务探活（根级、无认证）：
	//   GET /healthz 存活探针（liveness，进程活着即 200）
	//   GET /readyz  就绪探针（readiness，主库+日志库可达才 200，否则 503）
	// 供 K8s/Caddy/Docker 健康检查使用；Caddy 现用 /api/status（较重），可改用 /healthz。
	router.GET("/healthz", controller.Healthz)
	router.GET("/readyz", controller.Readyz)

	// G11 / MCP Gateway（Streamable HTTP）。POST/DELETE 需要 API token；
	// GET 由 handler 返回 405（旧式 HTTP+SSE 未实现，明确拒绝而非静默降级）。
	// 能力开关 MCP_GATEWAY_ENABLED=false 时 handler 返回 404。
	router.POST("/mcp", middleware.TokenAuth(), middleware.TokenRateLimit(), controller.McpHandler)
	router.DELETE("/mcp", middleware.TokenAuth(), middleware.TokenRateLimit(), controller.McpHandler)
	router.GET("/mcp", controller.McpHandler)
	router.OPTIONS("/mcp", controller.McpHandler)

	SetApiRouter(router)
	SetWebProtectionRouter(router)
	SetTrainingDataRouter(router)
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
