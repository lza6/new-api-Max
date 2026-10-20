package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
)

func newModelLimitRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

// 旧实现是「LLen 判断 + 请求后 LPUSH」，并发下同时看到未达上限就全部放行。
// 本测试并发抢同一批名额，必须**恰好**只有 maxCount 个成功。
func TestModelSuccessReservationIsAtomicUnderConcurrency(t *testing.T) {
	_, client := newModelLimitRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:v2:test:1"
	const maxSlots = 10
	const attempts = 60

	var wg sync.WaitGroup
	results := make(chan bool, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := reserveRedisSuccessSlot(ctx, client, key, maxSlots, 60)
			if err != nil {
				results <- false
				return
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)

	granted := 0
	for ok := range results {
		if ok {
			granted++
		}
	}
	assert.Equal(t, maxSlots, granted, "并发预留必须精确停在限额上")
	count, err := client.ZCard(ctx, key).Result()
	require.NoError(t, err)
	assert.EqualValues(t, maxSlots, count)
}

// 请求失败必须退还名额，否则失败请求会永久烧掉用户的成功额度。
func TestModelSuccessSlotRefundRestoresQuota(t *testing.T) {
	_, client := newModelLimitRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:v2:test:2"

	member, ok, err := reserveRedisSuccessSlot(ctx, client, key, 1, 60)
	require.NoError(t, err)
	require.True(t, ok)

	_, blocked, err := reserveRedisSuccessSlot(ctx, client, key, 1, 60)
	require.NoError(t, err)
	assert.False(t, blocked, "名额已满时必须拒绝")

	refundRedisSuccessSlot(ctx, client, key, member)
	_, allowedAfterRefund, err := reserveRedisSuccessSlot(ctx, client, key, 1, 60)
	require.NoError(t, err)
	assert.True(t, allowedAfterRefund, "退还后应能再次预留")
}

// 滑动窗口：超过窗口的旧记录要被清理，否则用户会被永久限流。
func TestModelSuccessSlotsExpireWithWindow(t *testing.T) {
	_, client := newModelLimitRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:v2:test:3"

	_, ok, err := reserveRedisSuccessSlot(ctx, client, key, 1, 1)
	require.NoError(t, err)
	require.True(t, ok)
	time.Sleep(1100 * time.Millisecond)
	_, ok, err = reserveRedisSuccessSlot(ctx, client, key, 1, 1)
	require.NoError(t, err)
	assert.True(t, ok, "窗口过期后应重新放行")
}

// 键名升级必须彻底：旧版本把同一语义存成 LIST，若新代码复用旧键会 WRONGTYPE。
func TestModelSuccessReservationIgnoresLegacyListKey(t *testing.T) {
	server, client := newModelLimitRedis(t)
	ctx := context.Background()
	legacyKey := "rateLimit:MRRLS:group:42"
	server.Push(legacyKey, "2026-01-01T00:00:00.000Z")

	_, ok, err := reserveRedisSuccessSlot(ctx, client, "rateLimit:MRRLS:v2:group:42", 5, 60)
	require.NoError(t, err)
	assert.True(t, ok)
}

// 中间件层：失败请求必须退还，成功请求占用名额。
func TestRedisRateLimitHandlerRefundsFailedRequest(t *testing.T) {
	_, client := newModelLimitRedis(t)
	previous := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previous })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 7)
		c.Next()
	})
	fail := true
	router.GET("/v1/chat/completions", redisRateLimitHandler(60, 0, 1, "default"), func(c *gin.Context) {
		if fail {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	// 第一次失败：预占后必须退还，否则接下来的成功请求会被自己上次的失败拦住。
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))
	require.Equal(t, http.StatusInternalServerError, w1.Code)

	fail = false
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusOK, w2.Code, "失败请求不应占用成功名额")

	// 额度用尽后必须 429。
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusTooManyRequests, w3.Code)
}
