package middleware

import (
	"net"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
)

// MetricsAuth B3-1：保护 /metrics 端点，避免把内部指标（渠道健康分、熔断状态、
// 队列深度、错误码分布）暴露给公网。放行条件（任一满足）：
//   - 请求来源可信（IsTrustedSourceIP：环回 / 链路本地 / RFC1918 私网）——供
//     同机/同容器内 Prometheus 抓取，无需凭据；
//   - 已通过管理会话鉴权（RootAuth）——供远程运维/仪表盘按需拉取。
//
// 否则 401。注意：这里**不**用 c.ClientIP() 判断来源——它受 TRUSTED_PROXIES 影响，
// 反代后可能被 X-Forwarded-For 覆盖；指标抓取通常直连内核，用 RemoteAddr 的原始
// 对端地址更稳妥（与 Web 防护信任来源判定同源）。
func MetricsAuth() func(c *gin.Context) {
	rootAuth := RootAuth()
	return func(c *gin.Context) {
		host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
		if err == nil {
			if ip := net.ParseIP(host); ip != nil && common.IsTrustedSourceIP(ip) {
				c.Next()
				return
			}
		}
		// 非可信来源：要求 root 会话（校验失败时 RootAuth 自行写 401/403 并 abort）。
		rootAuth(c)
	}
}

// MetricsEnabled 报告 /metrics 是否开放（env METRICS_ENABLED=true）。
func MetricsEnabled() bool {
	return strings.EqualFold(os.Getenv("METRICS_ENABLED"), "true")
}
