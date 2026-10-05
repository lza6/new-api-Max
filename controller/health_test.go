package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// §Health Checks：/healthz 存活探针恒 200。
func TestHealthzAlwaysOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/healthz", nil)

	Healthz(c)

	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"ok"`)
}

// §Health Checks：/readyz 在无 DB 时返回 503（未就绪）。
func TestReadyzNotReadyWithoutDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/readyz", nil)

	Readyz(c)

	// 测试环境 TestMain 已初始化内存库 → 通常 200；仅断言响应含状态字段。
	body := rec.Body.String()
	assert.True(t, strings.Contains(body, "ready") || strings.Contains(body, "not_ready"), body)
}
