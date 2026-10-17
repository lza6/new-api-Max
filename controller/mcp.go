package controller

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// MCP Gateway（G11 / Streamable HTTP）—— JSON-RPC 2.0 over HTTP。
//
// 设计要点：
//   - **不绕过 relay**：所有 tool/resource 调用都通过本机 HTTP 调回 `/v1/*`，
//     复用同一套鉴权、选渠道、预扣费、结算与日志；MCP 层不自行计费。
//   - **deny-by-default**：只读工具/资源直接放行；会产生额外副作用或费用的
//     tool（create_image）每次调用都需要一条管理员审批（service.AuthorizeAction）。
//   - **会话落库**：Mcp-Session-Id 存 DB，蓝绿部署期间跨实例可用。
//   - **能力开关**：MCP_GATEWAY_ENABLED=false 时端点返回 404，连协议握手都不可达。
//
// 传输：支持普通 JSON 响应；当客户端 `Accept: text/event-stream` 且请求
// chat_completion 且 `stream=true` 时，用 SSE 发送 progress notification + 最终结果。
// 未实现的旧式 HTTP+SSE、stdio 不在此端点提供（会返回明确错误）。

const (
	mcpProtocolVersion = "2025-06-18"
	mcpSessionTTL      = 30 * time.Minute
	mcpMaxBodyBytes    = 8 << 20
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpIdentity struct {
	UserId   int
	TokenId  int
	TokenKey string
	UserName string
}

func mcpEnabled() bool {
	return common.FeatureFlagValue(common.FlagMcpGatewayEnabled,
		common.GetEnvOrDefaultBool(common.FlagMcpGatewayEnabled, false))
}

func newMcpSessionID() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func mcpInternalBaseURL() string {
	port := common.GetEnvOrDefault("PORT", 3000)
	if common.Port != nil && *common.Port > 0 {
		port = *common.Port
	}
	return strings.TrimRight(common.GetEnvOrDefaultString("MCP_INTERNAL_BASE_URL",
		fmt.Sprintf("http://127.0.0.1:%d", port)), "/")
}

// mcpValidateInternalBaseURL 要求内部回环地址，除非管理员显式允许远程。
// 否则配置错误会把调用者的 API key 发到外部主机。
func mcpValidateInternalBaseURL() error {
	if strings.EqualFold(os.Getenv("MCP_INTERNAL_ALLOW_REMOTE"), "true") {
		return nil
	}
	u, err := url.Parse(mcpInternalBaseURL())
	if err != nil {
		return err
	}
	host := u.Hostname()
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("MCP_INTERNAL_BASE_URL must be loopback unless MCP_INTERNAL_ALLOW_REMOTE=true (got %s)", host)
}

func mcpIdentityFromContext(c *gin.Context) mcpIdentity {
	return mcpIdentity{
		UserId:   c.GetInt("id"),
		TokenId:  c.GetInt("token_id"),
		TokenKey: c.GetString("token_key"),
		UserName: c.GetString("username"),
	}
}

// McpHandler 是 `/mcp` 的唯一入口。
func McpHandler(c *gin.Context) {
	if !mcpEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "mcp gateway is disabled"})
		return
	}
	switch c.Request.Method {
	case http.MethodPost:
		mcpHandlePost(c)
	case http.MethodDelete:
		mcpHandleDelete(c)
	default:
		c.Header("Allow", "POST, DELETE")
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
	}
}

func mcpHandleDelete(c *gin.Context) {
	// 与会话使用路径一致：只能关闭属于自己的会话。
	if _, err := mcpRequireSession(c, mcpIdentityFromContext(c)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	id := c.GetHeader("Mcp-Session-Id")
	_ = model.CloseMcpSession(id, common.GetTimestamp())
	c.Status(http.StatusNoContent)
}

func mcpHandlePost(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, mcpMaxBodyBytes+1))
	if err != nil || len(body) > mcpMaxBodyBytes {
		mcpWriteError(c, nil, -32700, "request body too large or unreadable", nil)
		return
	}
	var req mcpRequest
	if err := common.Unmarshal(body, &req); err != nil {
		mcpWriteError(c, nil, -32700, "parse error", nil)
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		mcpWriteError(c, req.ID, -32600, "invalid JSON-RPC request", nil)
		return
	}

	identity := mcpIdentityFromContext(c)
	switch req.Method {
	case "initialize":
		mcpHandleInitialize(c, req, identity)
		return
	case "notifications/initialized", "notifications/cancelled":
		c.Status(http.StatusAccepted)
		return
	}

	session, err := mcpRequireSession(c, identity)
	if err != nil {
		mcpWriteError(c, req.ID, -32001, "valid Mcp-Session-Id is required", nil)
		return
	}
	_ = model.TouchMcpSession(session.Id, common.GetTimestamp(), common.GetTimestamp()+int64(mcpSessionTTL.Seconds()))

	switch req.Method {
	case "ping":
		mcpWriteResult(c, req.ID, map[string]any{})
	case "tools/list":
		mcpWriteResult(c, req.ID, mcpToolsList())
	case "tools/call":
		mcpHandleToolCall(c, req, identity)
	case "resources/list":
		mcpWriteResult(c, req.ID, mcpResourcesList())
	case "resources/read":
		mcpHandleResourceRead(c, req, identity)
	default:
		mcpWriteError(c, req.ID, -32601, "method not found", map[string]any{"method": req.Method})
	}
}

func mcpHandleInitialize(c *gin.Context, req mcpRequest, identity mcpIdentity) {
	var params struct {
		ProtocolVersion string         `json:"protocolVersion"`
		ClientInfo      map[string]any `json:"clientInfo"`
	}
	_ = common.Unmarshal(req.Params, &params)
	now := common.GetTimestamp()
	// 每用户活跃会话上限：超过时淘汰最旧会话，而不是直接拒绝握手
	// （MCP 客户端可能不显式 DELETE；直接拒绝会让正常客户端也无法重连）。
	if active, countErr := model.CountActiveMcpSessions(identity.UserId, now); countErr == nil && active >= 20 {
		_, _ = model.CloseOldestMcpSessions(identity.UserId, 19, now)
	}
	id, err := newMcpSessionID()
	if err != nil {
		mcpWriteError(c, req.ID, -32603, "failed to create session", nil)
		return
	}
	session := &model.McpSession{
		Id:              id,
		UserId:          identity.UserId,
		TokenId:         identity.TokenId,
		ProtocolVersion: firstNonEmpty(params.ProtocolVersion, mcpProtocolVersion),
		CreatedAt:       now,
		LastSeenAt:      now,
		ExpiresAt:       now + int64(mcpSessionTTL.Seconds()),
	}
	if err := model.CreateMcpSession(session); err != nil {
		mcpWriteError(c, req.ID, -32603, "failed to persist session", nil)
		return
	}
	_, _ = model.CleanupExpiredMcpSessions(now, 100)
	c.Header("Mcp-Session-Id", id)
	mcpWriteResult(c, req.ID, map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities": map[string]any{
			"tools":     map[string]any{"listChanged": false},
			"resources": map[string]any{"subscribe": false, "listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    "new-api",
			"version": common.Version,
		},
	})
}

func mcpRequireSession(c *gin.Context, identity mcpIdentity) (*model.McpSession, error) {
	id := c.GetHeader("Mcp-Session-Id")
	session, err := model.GetMcpSession(id, common.GetTimestamp())
	if err != nil {
		return nil, err
	}
	if session.UserId != identity.UserId || session.TokenId != identity.TokenId {
		return nil, errors.New("session does not belong to this credential")
	}
	return session, nil
}

func mcpToolsList() map[string]any {
	tools := []map[string]any{
		{
			"name":        "list_models",
			"description": "List models available to this API key, filtered by the key's group.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "chat_completion",
			"description": "Send a chat completion request through the gateway. Billing and rate limits are identical to calling /v1/chat/completions over HTTP.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"model":       map[string]any{"type": "string"},
					"messages":    map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
					"temperature": map[string]any{"type": "number"},
					"max_tokens":  map[string]any{"type": "integer"},
					"stream":      map[string]any{"type": "boolean"},
				},
				"required": []string{"model", "messages"},
			},
		},
		{
			"name":        "embed_text",
			"description": "Create embeddings through /v1/embeddings.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"model": map[string]any{"type": "string"},
					"input": map[string]any{},
				},
				"required": []string{"model", "input"},
			},
		},
		{
			"name":        "create_image",
			"description": "Generate an image. This tool has an upstream cost and requires an administrator approval for every call.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"model":  map[string]any{"type": "string"},
					"prompt": map[string]any{"type": "string"},
					"n":      map[string]any{"type": "integer"},
					"size":   map[string]any{"type": "string"},
				},
				"required": []string{"prompt"},
			},
		},
	}
	if mcpAlphaSearchEnabled() {
		tools = append(tools, map[string]any{
			"name":        "web_search",
			"description": "Run an alpha web search through the gateway (only when this deployment enables it).",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		})
	}
	return map[string]any{"tools": tools}
}

func mcpAlphaSearchEnabled() bool {
	return common.GetEnvOrDefaultBool("ALPHA_SEARCH_ENABLED", false)
}

func mcpResourcesList() map[string]any {
	return map[string]any{"resources": []map[string]any{
		{"uri": "newapi://me/quota", "name": "My quota", "description": "Current quota and used quota for this credential.", "mimeType": "application/json"},
		{"uri": "newapi://me/logs", "name": "My recent logs", "description": "The 20 most recent consume logs for this user.", "mimeType": "application/json"},
		{"uri": "newapi://models", "name": "Available models", "description": "Models available to this API key's group.", "mimeType": "application/json"},
		{"uri": "newapi://pricing", "name": "Public pricing", "description": "Public price list for this deployment.", "mimeType": "application/json"},
	}}
}

func mcpHandleToolCall(c *gin.Context, req mcpRequest, identity mcpIdentity) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Meta      map[string]any  `json:"_meta,omitempty"`
	}
	if err := common.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		mcpWriteError(c, req.ID, -32602, "invalid tools/call params", nil)
		return
	}
	switch params.Name {
	case "list_models":
		mcpProxyTool(c, req.ID, identity, http.MethodGet, "/v1/models", nil)
	case "chat_completion":
		mcpHandleChatTool(c, req, identity, params.Arguments, params.Meta)
	case "embed_text":
		mcpProxyTool(c, req.ID, identity, http.MethodPost, "/v1/embeddings", params.Arguments)
	case "create_image":
		// 图像端点的 model 是必填项；MCP 客户端通常只给 prompt。
		// 用 MCP_IMAGE_MODEL（默认 dall-e）补齐，管理员可改成本部署实际配置的模型。
		var imageArgs map[string]any
		if len(params.Arguments) > 0 {
			if err := common.Unmarshal(params.Arguments, &imageArgs); err != nil {
				mcpWriteError(c, req.ID, -32602, "arguments must be a JSON object", map[string]any{"error": err.Error()})
				return
			}
		}
		if imageArgs == nil {
			imageArgs = map[string]any{}
		}
		if _, ok := imageArgs["model"]; !ok {
			imageArgs["model"] = common.GetEnvOrDefaultString("MCP_IMAGE_MODEL", "dall-e")
		}
		// 审批必须绑定**实际请求内容**：否则管理员批准一张便宜图后，用户可换
		// 最贵模型/最大张数执行。resource 里带 model/n/size 与参数哈希。
		approvalResource := mcpImageApprovalResource(imageArgs)
		decision, err := service.AuthorizeAction(identity.UserId, service.ActionMcpPaidTool, approvalResource)
		if err != nil {
			mcpWriteError(c, req.ID, -32603, "approval check failed", map[string]any{"error": err.Error()})
			return
		}
		if !decision.Allowed {
			mcpWriteError(c, req.ID, -32003, "administrator approval required", map[string]any{
				"approval_id": decision.ApprovalId,
				"reason":      decision.Reason,
				"resource":    approvalResource,
			})
			return
		}
		imageBody, err := common.Marshal(imageArgs)
		if err != nil {
			mcpWriteError(c, req.ID, -32602, "failed to encode image arguments", nil)
			return
		}
		mcpProxyTool(c, req.ID, identity, http.MethodPost, "/v1/images/generations", imageBody)
	case "web_search":
		if !mcpAlphaSearchEnabled() {
			mcpWriteError(c, req.ID, -32601, "web_search is not enabled on this deployment", nil)
			return
		}
		mcpProxyTool(c, req.ID, identity, http.MethodPost, "/v1/alpha/search", params.Arguments)
	default:
		mcpWriteError(c, req.ID, -32602, "unknown tool", map[string]any{"name": params.Name})
	}
}

// mcpHandleChatTool 处理 chat_completion；支持 SSE progress（仅在客户端要求时）。
func mcpHandleChatTool(c *gin.Context, req mcpRequest, identity mcpIdentity, arguments json.RawMessage, meta map[string]any) {
	var args map[string]any
	if err := common.Unmarshal(arguments, &args); err != nil {
		mcpWriteError(c, req.ID, -32602, "arguments must be a JSON object", nil)
		return
	}
	stream, _ := args["stream"].(bool)
	if !stream || !strings.Contains(c.GetHeader("Accept"), "text/event-stream") {
		args["stream"] = false
		body, _ := common.Marshal(args)
		mcpProxyTool(c, req.ID, identity, http.MethodPost, "/v1/chat/completions", body)
		return
	}
	mcpProxyChatStream(c, req, identity, args, meta)
}

func mcpProxyTool(c *gin.Context, id json.RawMessage, identity mcpIdentity, method, path string, body []byte) {
	status, respBody, err := mcpInternalRequest(c, identity, method, path, body, false)
	if err != nil {
		mcpWriteError(c, id, -32603, "upstream request failed", map[string]any{"error": err.Error()})
		return
	}
	text := string(respBody)
	result := map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": status < 200 || status >= 300,
	}
	mcpWriteResult(c, id, result)
}

func mcpProxyChatStream(c *gin.Context, req mcpRequest, identity mcpIdentity, args map[string]any, meta map[string]any) {
	progressToken := any(nil)
	if meta != nil {
		progressToken = meta["progressToken"]
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)

	emit := func(payload any) {
		raw, err := common.Marshal(payload)
		if err != nil {
			return
		}
		_, _ = c.Writer.WriteString("event: message\ndata: " + string(raw) + "\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}

	args["stream"] = true
	body, _ := common.Marshal(args)
	resp, err := mcpInternalStream(c, identity, http.MethodPost, "/v1/chat/completions", body)
	if err != nil {
		emit(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpRPCError{Code: -32603, Message: "upstream request failed", Data: err.Error()}})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		emit(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpRPCError{Code: -32603, Message: "upstream returned an error", Data: string(raw)}})
		return
	}

	var full strings.Builder
	progress := 0
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if common.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		delta := mcpChatDelta(chunk)
		if delta == "" {
			continue
		}
		full.WriteString(delta)
		progress++
		if progressToken != nil {
			emit(map[string]any{
				"jsonrpc": "2.0",
				"method":  "notifications/progress",
				"params": map[string]any{
					"progressToken": progressToken,
					"progress":      progress,
					"message":       delta,
				},
			})
		}
	}
	if err := scanner.Err(); err != nil {
		emit(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpRPCError{Code: -32603, Message: "stream read failed", Data: err.Error()}})
		return
	}
	emit(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": full.String()}},
		"isError": false,
	}})
}

// mcpImageApprovalResource 把图像请求的关键参数绑定进审批键。
// 管理员在审批页能看到 model/n/size 与参数哈希；换参数会得到不同的 pending，
// 无法用一次批准执行另一笔更贵的请求。
func mcpImageApprovalResource(args map[string]any) string {
	raw, _ := common.Marshal(args)
	sum := sha256.Sum256(raw)
	model, _ := args["model"].(string)
	size, _ := args["size"].(string)
	// 哈希放在最前：即使 resource 被 191 字节截断，参数绑定也不会丢失。
	resource := fmt.Sprintf("create_image:sha256=%s:model=%s:n=%v:size=%s",
		hex.EncodeToString(sum[:6]), model, args["n"], size)
	if len(resource) > 191 {
		resource = resource[:191]
	}
	return resource
}

func mcpChatDelta(chunk map[string]any) string {
	choices, ok := chunk["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	if content, ok := delta["content"].(string); ok {
		return content
	}
	return ""
}

func mcpHandleResourceRead(c *gin.Context, req mcpRequest, identity mcpIdentity) {
	var params struct {
		URI string `json:"uri"`
	}
	if err := common.Unmarshal(req.Params, &params); err != nil || params.URI == "" {
		mcpWriteError(c, req.ID, -32602, "invalid resources/read params", nil)
		return
	}
	var payload any
	var err error
	switch params.URI {
	case "newapi://me/quota":
		payload, err = mcpReadQuota(identity.UserId)
	case "newapi://me/logs":
		payload, err = mcpReadLogs(identity.UserId)
	case "newapi://models":
		status, body, reqErr := mcpInternalRequest(c, identity, http.MethodGet, "/v1/models", nil, false)
		if reqErr != nil || status < 200 || status >= 300 {
			err = firstError(reqErr, fmt.Errorf("upstream status %d: %s", status, string(body)))
		} else {
			payload = json.RawMessage(body)
		}
	case "newapi://pricing":
		status, body, reqErr := mcpInternalRequest(c, identity, http.MethodGet, "/v1/pricing", nil, false)
		if reqErr != nil || status < 200 || status >= 300 {
			err = firstError(reqErr, fmt.Errorf("upstream status %d: %s", status, string(body)))
		} else {
			payload = json.RawMessage(body)
		}
	default:
		mcpWriteError(c, req.ID, -32602, "unknown resource URI", map[string]any{"uri": params.URI})
		return
	}
	if err != nil {
		mcpWriteError(c, req.ID, -32603, "resource read failed", map[string]any{"error": err.Error()})
		return
	}
	raw, marshalErr := common.Marshal(payload)
	if marshalErr != nil {
		mcpWriteError(c, req.ID, -32603, "failed to serialize resource", nil)
		return
	}
	mcpWriteResult(c, req.ID, map[string]any{
		"contents": []map[string]any{{
			"uri":      params.URI,
			"mimeType": "application/json",
			"text":     string(raw),
		}},
	})
}

func mcpReadQuota(userId int) (any, error) {
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return nil, err
	}
	quota, err := model.GetUserQuota(userId, false)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"quota":         quota,
		"used_quota":    user.UsedQuota,
		"request_count": user.RequestCount,
	}, nil
}

func mcpReadLogs(userId int) (any, error) {
	logs, _, err := model.GetUserLogs(userId, model.LogTypeConsume, 0, 0, "", "", 0, 20, "", "", "", 0)
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func firstError(err error, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func mcpWriteResult(c *gin.Context, id json.RawMessage, result any) {
	c.JSON(http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func mcpWriteError(c *gin.Context, id json.RawMessage, code int, message string, data any) {
	c.JSON(http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpRPCError{Code: code, Message: message, Data: data}})
}

// mcpInternalStream 发起流式内部请求（chat_completion stream=true）。
func mcpInternalStream(c *gin.Context, identity mcpIdentity, method, path string, body []byte) (*http.Response, error) {
	if err := mcpValidateInternalBaseURL(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), method, mcpInternalBaseURL()+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+identity.TokenKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second}}
	return client.Do(req)
}

// mcpInternalRequest 发起内部 HTTP 请求，复用完整 relay 链路。
func mcpInternalRequest(c *gin.Context, identity mcpIdentity, method, path string, body []byte, stream bool) (int, []byte, error) {
	if err := mcpValidateInternalBaseURL(); err != nil {
		return 0, nil, err
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), method, mcpInternalBaseURL()+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+identity.TokenKey)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second}, Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, mcpMaxBodyBytes))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}
