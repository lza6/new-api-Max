package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/relay_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useRateLimitMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, redisClient.Ping(context.Background()).Err())

	common.RedisEnabled = true
	common.RDB = redisClient
	t.Cleanup(func() {
		_ = redisClient.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})

	return redisServer, redisClient
}

func performRateLimitRequest(router http.Handler, path string, remoteAddr string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = remoteAddr
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestRedisIPRateLimiterThresholdTTLAndNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/limited", rateLimitFactory(2, 37, "TEST"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "192.0.2.10:12345"
	legacyKey := "rateLimit:TEST192.0.2.10"
	_, err := redisServer.Push(legacyKey, "legacy-list-entry")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", remoteAddr).Code)
	limitedResponse := performRateLimitRequest(router, "/limited", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, limitedResponse.Code)
	assert.Equal(t, "37", limitedResponse.Header().Get("Retry-After"))
	// B6-2：限流响应携带机器可读 error.type=rate_limited。
	assert.JSONEq(t, `{"error":{"message":"rate limited, please retry later","type":"rate_limited","code":"rate_limited"}}`, limitedResponse.Body.String())

	key := redisIPRateLimitKey("TEST", "192.0.2.10")
	count, err := redisServer.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "3", count)
	assert.Equal(t, 37*time.Second, redisServer.TTL(key))
	assert.True(t, redisServer.Exists(legacyKey), "the v2 counter must not touch an old list key")
}

func TestRedisUserRateLimiterUsesSharedFixedWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	router.GET(
		"/limited",
		func(c *gin.Context) { c.Set("id", 42) },
		userRateLimitFactory(1, 23, "USER"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "192.0.2.20:12345").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/limited", "198.51.100.20:12345").Code)

	key := redisUserRateLimitKey("USER", 42)
	assert.True(t, redisServer.Exists(key))
	assert.Equal(t, 23*time.Second, redisServer.TTL(key))
}

func TestRedisEmailVerificationRateLimiterPreservesResponseAndTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/verify", EmailVerificationRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "192.0.2.30:12345"
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/verify", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/verify", remoteAddr).Code)
	response := performRateLimitRequest(router, "/verify", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.JSONEq(t, `{"success":false,"message":"发送过于频繁，请等待 30 秒后再试"}`, response.Body.String())

	key := redisIPRateLimitKey(EmailVerificationRateLimitMark, "192.0.2.30")
	assert.True(t, redisServer.Exists(key))
	assert.Equal(t, time.Duration(EmailVerificationDuration)*time.Second, redisServer.TTL(key))
}

func TestRedisFixedWindowIsAtomicUnderConcurrency(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const (
		requestCount = 20
		maximumCount = 7
		duration     = int64(41)
	)
	key := redisIPRateLimitKey("CONCURRENT", "192.0.2.40")

	var allowedCount atomic.Int64
	errorsFound := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)
	for range requestCount {
		go func() {
			defer waitGroup.Done()
			allowed, _, _, err := redisFixedWindowTake(context.Background(), key, maximumCount, duration)
			if err != nil {
				errorsFound <- err
				return
			}
			if allowed {
				allowedCount.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}

	assert.Equal(t, int64(maximumCount), allowedCount.Load())
	count, err := redisServer.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "20", count)
	assert.Equal(t, time.Duration(duration)*time.Second, redisServer.TTL(key))
}

func TestRedisFixedWindowResetsAtBoundary(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const duration = int64(10)
	key := redisIPRateLimitKey("BOUNDARY", "192.0.2.50")

	for range 2 {
		allowed, _, _, err := redisFixedWindowTake(context.Background(), key, 2, duration)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
	allowed, _, _, err := redisFixedWindowTake(context.Background(), key, 2, duration)
	require.NoError(t, err)
	assert.False(t, allowed)

	// This reset is intentional fixed-window behavior. A client can consume one
	// full allowance immediately before and another immediately after a boundary.
	redisServer.FastForward(time.Duration(duration) * time.Second)
	for range 2 {
		allowed, _, _, err = redisFixedWindowTake(context.Background(), key, 2, duration)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
}

func TestRedisFixedWindowRepairsCounterWithoutTTL(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const duration = int64(29)
	key := redisIPRateLimitKey("MISSING-TTL", "192.0.2.51")
	redisServer.Set(key, "5")

	allowed, count, ttl, err := redisFixedWindowTake(context.Background(), key, 3, duration)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, int64(6), count)
	assert.Equal(t, duration, ttl)
	assert.Equal(t, time.Duration(duration)*time.Second, redisServer.TTL(key))

	redisServer.FastForward(time.Duration(duration) * time.Second)
	assert.False(t, redisServer.Exists(key), "a recovered counter must not remain permanently rate-limited")
}

func TestRedisFailurePolicies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, redisClient := useRateLimitMiniRedis(t)
	require.NoError(t, redisClient.Close())

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/ip", rateLimitFactory(1, 30, "FAIL-IP"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET(
		"/user",
		func(c *gin.Context) { c.Set("id", 7) },
		userRateLimitFactory(1, 30, "FAIL-USER"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	router.GET("/email", EmailVerificationRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	ipResponse := performRateLimitRequest(router, "/ip", "192.0.2.60:12345")
	assert.Equal(t, http.StatusInternalServerError, ipResponse.Code)
	assert.Empty(t, ipResponse.Body.String())
	userResponse := performRateLimitRequest(router, "/user", "192.0.2.61:12345")
	assert.Equal(t, http.StatusInternalServerError, userResponse.Code)
	assert.Empty(t, userResponse.Body.String())
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/email", "192.0.2.62:12345").Code)
}

func TestRequestModelNamePeekRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw := []byte(`{"model":"google-translate","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	model := requestModelNamePeek(c)
	assert.Equal(t, "google-translate", model)

	// 请求体必须完整还原，后续 relay 解析不受影响。
	restored, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	assert.Equal(t, raw, restored)

	// 二次调用走 context 缓存，不重复读体。
	c.Request.Body = io.NopCloser(bytes.NewReader([]byte(`{"model":"other"}`)))
	assert.Equal(t, "google-translate", requestModelNamePeek(c))
}

func TestIsRateLimitExemptModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := relay_setting.GetRelaySetting().UserRateLimitExemptModels
	t.Cleanup(func() { relay_setting.GetRelaySetting().UserRateLimitExemptModels = prev })

	newCtx := func(body string) *gin.Context {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		return c
	}

	// 空清单：任何模型都不豁免，且不读体。
	relay_setting.GetRelaySetting().UserRateLimitExemptModels = nil
	assert.False(t, isRateLimitExemptModel(newCtx(`{"model":"google-translate"}`)))

	// 命中豁免模型。
	relay_setting.GetRelaySetting().UserRateLimitExemptModels = []string{"google-translate"}
	assert.True(t, isRateLimitExemptModel(newCtx(`{"model":"google-translate"}`)))
	assert.False(t, isRateLimitExemptModel(newCtx(`{"model":"deepseek-v4-flash"}`)))
}

func TestIsHealthProbePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/api/status", true},
		{"/api/uptime/status", true},
		{"/api/status/", false},
		{"/api/status/test", false},
		{"/api/models", false},
		{"/", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, isHealthProbePath(tc.path), "path=%q", tc.path)
	}
}

// TestGlobalAPIRateLimitSkipsHealthProbe 生产回归：Caddy 1s 活跃健康检查打
// /api/status 曾打爆 GA 限流桶→429→后端全部被摘→公网 503。健康探针路径
// 必须豁免全局 API 限流；非探针路径限流行为保持不变。
func TestGlobalAPIRateLimitSkipsHealthProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevEnable := common.GlobalApiRateLimitEnable
	prevNum := common.GlobalApiRateLimitNum
	prevDuration := common.GlobalApiRateLimitDuration
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1 // 窗口内只放 1 次：探针必须仍无限通过
	common.GlobalApiRateLimitDuration = 60
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = prevEnable
		common.GlobalApiRateLimitNum = prevNum
		common.GlobalApiRateLimitDuration = prevDuration
	})

	_, _ = useRateLimitMiniRedis(t) // Redis 限流桶（与包内其他限流测试一致）
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/api/status", GlobalAPIRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/api/limited", GlobalAPIRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for range 3 {
		assert.Equal(t, http.StatusNoContent,
			performRateLimitRequest(router, "/api/status", "10.0.0.1:1234").Code,
			"健康探针路径必须豁免 GA 全局限流")
	}
	assert.Equal(t, http.StatusNoContent,
		performRateLimitRequest(router, "/api/limited", "10.0.0.2:1234").Code)
	assert.Equal(t, http.StatusTooManyRequests,
		performRateLimitRequest(router, "/api/limited", "10.0.0.2:1234").Code)
}
