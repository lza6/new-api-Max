package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func corsTestRouter(t *testing.T, origin string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	_ = origin
	return r
}

func doCORSPreflight(t *testing.T, r *gin.Engine, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestCORSNoWildcardWithCredentials 断言修复的核心：响应绝不出现
// `Access-Control-Allow-Origin: *` 与 `Access-Control-Allow-Credentials: true`
// 的非法组合（规范禁止，浏览器会拒）。
func TestCORSNoWildcardWithCredentials(t *testing.T) {
	t.Setenv(CORSAllowedOriginsEnv, "https://app.example.com")
	r := corsTestRouter(t, "https://app.example.com")
	w := doCORSPreflight(t, r, "https://app.example.com")

	assert.NotEqual(t, "*", w.Header().Get("Access-Control-Allow-Origin"),
		"wildcard origin must never be sent together with credentials")
	if w.Header().Get("Access-Control-Allow-Credentials") == "true" {
		assert.NotEqual(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestCORSWhitelistedOriginEchoed 白名单来源被回显，且凭证头存在。
func TestCORSWhitelistedOriginEchoed(t *testing.T) {
	t.Setenv(CORSAllowedOriginsEnv, "https://app.example.com, https://admin.example.com")
	r := corsTestRouter(t, "https://app.example.com")

	w := doCORSPreflight(t, r, "https://admin.example.com")
	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://admin.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

// TestCORSDisallowedOriginRejected 非白名单来源被拒绝，且不回显来源。
func TestCORSDisallowedOriginRejected(t *testing.T) {
	t.Setenv(CORSAllowedOriginsEnv, "https://app.example.com")
	r := corsTestRouter(t, "https://app.example.com")

	w := doCORSPreflight(t, r, "https://evil.example.com")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORSDefaultSameOriginOnly 未配置白名单时仅同源：跨域预检被拒，且不产生
// 任何 Access-Control-Allow-* 头（安全默认）。
func TestCORSDefaultSameOriginOnly(t *testing.T) {
	t.Setenv(CORSAllowedOriginsEnv, "")
	r := corsTestRouter(t, "https://app.example.com")

	w := doCORSPreflight(t, r, "https://app.example.com")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestParseAllowedOrigins(t *testing.T) {
	assert.Nil(t, parseAllowedOrigins(""))
	assert.Nil(t, parseAllowedOrigins("   "))
	assert.Equal(t, []string{"https://a.com", "https://b.com"},
		parseAllowedOrigins(" https://a.com , https://b.com "))
}
