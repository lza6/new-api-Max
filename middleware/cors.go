package middleware

import (
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
)

// CORSAllowedOriginsEnv 是允许跨域访问的来源白名单（逗号分隔）。留空 = 仅同源
// （不发送任何 Access-Control-Allow-* 头），是本项目的安全默认。
const CORSAllowedOriginsEnv = "CORS_ALLOWED_ORIGINS"

// CORS 返回跨域中间件。
//
// 修复说明：原实现同时设置 AllowAllOrigins=true 与 AllowCredentials=true，
// 二者语义冲突——按 CORS 规范 `Access-Control-Allow-Origin: *` 与
// `Access-Control-Allow-Credentials: true` 不能共存，浏览器会拒绝该组合，
// 导致 AllowCredentials 成为误导性死配置（详见 §4.9.1）。
//
// 现策略：由 env CORS_ALLOWED_ORIGINS 显式控制白名单。
//   - 设置白名单：仅这些来源被允许，并启用凭证（Cookie）——此时必须回显具体来源，
//     故用 AllowOriginFunc 而非 `*`，符合规范。
//   - 未设置：仅同源（拒绝所有跨域），不发送任何 CORS 头。
//
// 网关的主认证是 `Authorization: Bearer`（header），会话刷新走 SameSite=Strict
// 的 Cookie，均不受此收紧影响；第三方 API 客户端为服务端调用、不发送 Origin，亦不受影响。
func CORS() gin.HandlerFunc {
	config := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           cors.DefaultConfig().MaxAge,
	}

	allowed := parseAllowedOrigins(common.GetEnvOrDefaultString(CORSAllowedOriginsEnv, ""))
	if len(allowed) == 0 {
		// 仅同源：AllowOriginFunc 恒 false（Validate 不会因全禁而 panic），
		// 绝不设置 AllowAllOrigins，从而不产生与凭证冲突的头。
		config.AllowCredentials = false
		config.AllowOriginFunc = func(string) bool { return false }
		return cors.New(config)
	}

	config.AllowOrigins = allowed
	return cors.New(config)
}

// parseAllowedOrigins 拆分、去空白、去空项（保留大小写，Origins 比较区分大小写）。
func parseAllowedOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func Version() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-New-Api-Version", common.Version)
		c.Next()
	}
}
