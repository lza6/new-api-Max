package middleware

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
)

const RouteTagKey = "route_tag"

// slowRequestThresholdMs 慢请求阈值（env SLOW_REQUEST_THRESHOLD_MS，默认 3000ms）；
// 超过阈值时输出带 request-id 的采样日志，供按 request-id 聚合慢链路。
func slowRequestThresholdMs() int64 {
	raw := os.Getenv("SLOW_REQUEST_THRESHOLD_MS")
	if raw == "" {
		return 3000
	}
	if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v >= 0 {
		return v
	}
	return 3000
}

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	server.Use(redactTaskArtifactAccessQuery())
	server.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		path := param.Path
		// OAuth callbacks carry one-time codes and state in the query string.
		// Redact the log value only; the handler still needs the original query.
		if strings.HasPrefix(path, "/api/oauth/") || strings.HasPrefix(path, "/oauth/") {
			path, _, _ = strings.Cut(path, "?")
		}

		// §4.1.4 指标：请求量 + 延迟直方图（Prometheus 文本，/metrics 输出）。
		common.MetricsInc("http_requests_total", map[string]string{
			"method": param.Method,
			"status": strconv.Itoa(param.StatusCode),
			"route":  tag,
		}, 1)
		common.MetricsObserve("http_request_duration_seconds", param.Latency.Seconds())

		// §4.1.4 慢链路采样：超过阈值输出带 request-id 的日志，供聚合慢请求。
		if ms := param.Latency.Milliseconds(); ms > slowRequestThresholdMs() {
			common.SysLog(fmt.Sprintf("[SLOW] request-id=%s route=%s method=%s path=%s latency=%v status=%d ip=%s",
				requestID, tag, param.Method, path, param.Latency, param.StatusCode, param.ClientIP))
		}

		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			tag,
			requestID,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
		)
	}))
}
