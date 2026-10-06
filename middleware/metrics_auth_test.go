package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestMetricsAuthAllowsTrustedSourceIP B3-1：可信来源（环回私网）无凭据即可读 /metrics。
func TestMetricsAuthAllowsTrustedSourceIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/metrics", MetricsAuth(), func(c *gin.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:54321" // loopback → trusted
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

// TestMetricsAuthRejectsPublicIP B3-1：公网来源无 root 会话 → 拒绝（非 200）。
func TestMetricsAuthRejectsPublicIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/metrics", MetricsAuth(), func(c *gin.Context) {
		c.String(200, "leaked")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.RemoteAddr = "203.0.113.9:40000" // public, no session
	router.ServeHTTP(w, req)

	assert.NotEqual(t, 200, w.Code, "public source without root session must not read metrics")
	assert.NotContains(t, w.Body.String(), "leaked")
}

// TestMetricsEnabledEnv B3-1：METRICS_ENABLED 大小写不敏感，"true" 才开放。
func TestMetricsEnabledEnv(t *testing.T) {
	t.Setenv("METRICS_ENABLED", "true")
	assert.True(t, MetricsEnabled())
	t.Setenv("METRICS_ENABLED", "TRUE")
	assert.True(t, MetricsEnabled())
	t.Setenv("METRICS_ENABLED", "false")
	assert.False(t, MetricsEnabled())
	t.Setenv("METRICS_ENABLED", "")
	assert.False(t, MetricsEnabled())
}
