package service

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/model"
)

// §客户端识别：从 User-Agent 归一到稳定客户端标识，供日志展示与排行榜统计。
// 归一为有限集合，避免 UA 噪声导致统计爆炸；无法识别时归 "other"。
//
// 记录字段（消费日志 other，public 作用域）：
//   - client:        归一后的客户端标识（如 claude-code / cursor / node / python）
//   - user_agent:    原始 UA（截断到 200 字符，去敏——UA 不含凭据）
func appendClientInfo(ctx *gin.Context, other *model.LogOther) {
	if ctx == nil || ctx.Request == nil || other == nil {
		return
	}
	ua := ctx.Request.UserAgent()
	if ua == "" {
		return
	}
	other.SetPublic("client", IdentifyClient(ua))
	// UA 截断防超长；不含敏感信息（HTTP 头 UA 是客户端自报标识）。
	if len(ua) > 200 {
		ua = ua[:200]
	}
	other.SetPublic("user_agent", ua)
}

// IdentifyClient 把 User-Agent 归一为稳定客户端标识。
// 覆盖常见 AI 编码工具 / SDK / HTTP 客户端；未识别返回 "other"。
func IdentifyClient(ua string) string {
	u := strings.ToLower(ua)
	switch {
	case u == "":
		return "unknown"
	// AI 编码 / 代理工具（优先匹配，避免被通用 SDK 规则抢先）
	case strings.Contains(u, "claude-code") || strings.Contains(u, "claude code"):
		return "claude-code"
	case strings.Contains(u, "cursor"):
		return "cursor"
	case strings.Contains(u, "cline"):
		return "cline"
	case strings.Contains(u, "roo-code") || strings.Contains(u, "roocode"):
		return "roo-code"
	case strings.Contains(u, "windsurf"):
		return "windsurf"
	case strings.Contains(u, "continue"):
		return "continue"
	case strings.Contains(u, "aider"):
		return "aider"
	case strings.Contains(u, "openwebui") || strings.Contains(u, "open-webui"):
		return "open-webui"
	case strings.Contains(u, "lobe") || strings.Contains(u, "lobechat"):
		return "lobe-chat"
	case strings.Contains(u, "cherry") && strings.Contains(u, "studio"):
		return "cherry-studio"
	case strings.Contains(u, "chatbox"):
		return "chatbox"
	case strings.Contains(u, "nextchat") || strings.Contains(u, "chatgpt-next-web"):
		return "nextchat"
	case strings.Contains(u, "immersivetranslate") || strings.Contains(u, "immersive-translate"):
		return "immersive-translate"
	case strings.Contains(u, "cherry"):
		return "cherry-studio"
	// 官方 SDK / 语言运行时
	case strings.Contains(u, "anthropic"): // anthropic SDK
		return "anthropic-sdk"
	case strings.Contains(u, "openai-python") || strings.Contains(u, "openai/python"):
		return "openai-python"
	case strings.Contains(u, "openai-node") || strings.Contains(u, "openai/node"):
		return "openai-node"
	case strings.Contains(u, "python-requests") || strings.Contains(u, "python-urllib") || strings.Contains(u, "aiohttp") || strings.Contains(u, "httpx"):
		return "python"
	case strings.Contains(u, "axios") || strings.Contains(u, "node-fetch") || strings.Contains(u, "undici") || strings.Contains(u, "node.js") || strings.Contains(u, "nodejs"):
		return "node"
	case strings.Contains(u, "golang") || strings.Contains(u, "go-http-client"):
		return "go"
	case strings.Contains(u, "curl"):
		return "curl"
	case strings.Contains(u, "okhttp"):
		return "okhttp"
	// 通用浏览器/系统（非程序化客户端，通常较少见）
	case strings.Contains(u, "postman"):
		return "postman"
	case strings.Contains(u, "mozilla"):
		return "browser"
	default:
		return "other"
	}
}
