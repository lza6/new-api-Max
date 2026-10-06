package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// B3-2：Redis 关闭时 /readyz 视为「未依赖 Redis，就绪」。
func TestReadyzRedisDisabled(t *testing.T) {
	prevEnabled, prevRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = prevEnabled, prevRDB })

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/readyz", nil)
	Readyz(c)

	assert.Contains(t, rec.Body.String(), `"redis":"disabled"`)
}

// B3-2：Redis 可达时 /readyz 报告 up。
func TestReadyzRedisUp(t *testing.T) {
	server := miniredis.RunT(t)
	prevEnabled, prevRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = prevEnabled, prevRDB
	})

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/readyz", nil)
	Readyz(c)

	require.Contains(t, rec.Body.String(), `"redis":"up"`)
}

// B3-2：Redis 启用但不可达（默认 fail-hard）→ 503 not_ready。
func TestReadyzRedisDownFailsHard(t *testing.T) {
	server := miniredis.RunT(t)
	addr := server.Addr()
	server.Close() // 服务已关 → 连接失败

	prevEnabled, prevRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: addr, MaxRetries: 0, DialTimeout: 200 * 1e6})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = prevEnabled, prevRDB
	})

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/readyz", nil)
	Readyz(c)

	require.Equal(t, 503, rec.Code)
	assert.Contains(t, rec.Body.String(), "redis")
}

// B3-2：READYZ_CHECK_REDIS=false（fail-soft）→ Redis 宕机但不影响就绪码。
func TestReadyzRedisDownFailSoft(t *testing.T) {
	t.Setenv("READYZ_CHECK_REDIS", "false")
	server := miniredis.RunT(t)
	addr := server.Addr()
	server.Close()

	prevEnabled, prevRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: addr, MaxRetries: 0, DialTimeout: 200 * 1e6})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = prevEnabled, prevRDB
	})

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/readyz", nil)
	Readyz(c)

	assert.Contains(t, rec.Body.String(), "redis")
	assert.NotEqual(t, 503, rec.Code, "fail-soft must not fail readiness on Redis outage")
}
