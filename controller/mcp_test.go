package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

func seedMcpDB(t *testing.T) {
	t.Helper()
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.McpSession{}, &model.ActionApproval{}, &model.User{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
}

func enableMcpForTest(t *testing.T) {
	t.Helper()
	enabled := true
	common.SetFeatureFlagOverride(common.FlagMcpGatewayEnabled, common.BoolFeatureFlagOverride(&enabled))
	t.Cleanup(func() { common.SetFeatureFlagOverride(common.FlagMcpGatewayEnabled, nil) })
}

func mcpTestContext(t *testing.T, body string, sessionID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		c.Request.Header.Set("Mcp-Session-Id", sessionID)
	}
	c.Set("id", 1)
	c.Set("token_id", 2)
	c.Set("token_key", "test-token-key")
	c.Set("username", "tester")
	return c, rec
}

func mcpInitialize(t *testing.T) string {
	t.Helper()
	c, rec := mcpTestContext(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test"}}}`, "")
	McpHandler(c)
	require.Equal(t, http.StatusOK, rec.Code)
	session := rec.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, session)
	var resp struct {
		Result map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, mcpProtocolVersion, resp.Result["protocolVersion"])
	return session
}

func TestMcpDisabledReturns404(t *testing.T) {
	seedMcpDB(t)
	c, rec := mcpTestContext(t, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, "")
	McpHandler(c)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMcpInitializeAndToolsList(t *testing.T) {
	seedMcpDB(t)
	enableMcpForTest(t)
	session := mcpInitialize(t)

	c, rec := mcpTestContext(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, session)
	McpHandler(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	names := make([]string, 0, len(resp.Result.Tools))
	for _, tool := range resp.Result.Tools {
		names = append(names, tool["name"].(string))
	}
	assert.Contains(t, names, "list_models")
	assert.Contains(t, names, "chat_completion")
	assert.Contains(t, names, "create_image")
}

func TestMcpRequiresSessionForTools(t *testing.T) {
	seedMcpDB(t)
	enableMcpForTest(t)
	c, rec := mcpTestContext(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, "")
	McpHandler(c)
	var resp struct {
		Error *mcpRPCError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, -32001, resp.Error.Code)
}

func TestMcpPaidToolRequiresApprovalThenProxies(t *testing.T) {
	seedMcpDB(t)
	enableMcpForTest(t)
	restoreTTL := service.SetActionApprovalTTLForTest(5 * time.Minute)
	t.Cleanup(restoreTTL)
	session := mcpInitialize(t)

	// 第一次：默认拒绝，返回 approval_id。
	c, rec := mcpTestContext(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_image","arguments":{"prompt":"a cat"}}}`, session)
	McpHandler(c)
	var denied struct {
		Error *mcpRPCError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &denied))
	require.NotNil(t, denied.Error)
	assert.Equal(t, -32003, denied.Error.Code)
	data, _ := denied.Error.Data.(map[string]any)
	require.NotNil(t, data)
	approvalID := int64(data["approval_id"].(float64))

	// 管理员批准后，第二次调用必须真的转发到内部上游。
	require.NoError(t, service.ApproveAction(approvalID, 99, "test"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/images/generations", r.URL.Path)
		assert.Equal(t, "Bearer test-token-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"url":"https://example.test/img.png"}]}`))
	}))
	defer upstream.Close()
	t.Setenv("MCP_INTERNAL_BASE_URL", upstream.URL)

	c, rec = mcpTestContext(t, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_image","arguments":{"prompt":"a cat"}}}`, session)
	McpHandler(c)
	var ok struct {
		Result map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ok))
	content, _ := ok.Result["content"].([]any)
	require.Len(t, content, 1)
	text := content[0].(map[string]any)["text"].(string)
	assert.Contains(t, text, "example.test/img.png")
}
