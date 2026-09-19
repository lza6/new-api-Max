package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 滑动窗口：同秒累加、一分钟窗口求和。
func TestCompletedWindowAddAndSum(t *testing.T) {
	var w completedWindow
	base := time.Unix(1_700_000_000, 0)

	// 同一秒内 3 次完成
	for i := 0; i < 3; i++ {
		w.add(base)
	}
	require.Equal(t, int64(3), w.sumLastMinute(base))

	// 10 秒后完成 2 次 → 窗口共 5
	t10 := base.Add(10 * time.Second)
	w.add(t10)
	w.add(t10)
	assert.Equal(t, int64(5), w.sumLastMinute(t10))

	// 61 秒后：base 桶被复用（mod 61 相同），新增 1 次计入；base 已在窗口外
	t61 := base.Add(61 * time.Second)
	w.add(t61)
	// 窗口内只剩 t10(2) + t61(1) = 3
	assert.Equal(t, int64(3), w.sumLastMinute(t61))
}

// 滑动窗口：超过 60 秒的完成数必须被淘汰。
func TestCompletedWindowEvictsOld(t *testing.T) {
	var w completedWindow
	start := time.Unix(1_700_000_000, 0)
	w.add(start)
	w.add(start)

	// 70 秒后，start 秒的 2 次已超出 1 分钟窗口
	after := start.Add(70 * time.Second)
	assert.Equal(t, int64(0), w.sumLastMinute(after))
}

// 中间件：请求进入时并发 +1，结束时 -1，并累计完成数。
func TestStatsMiddlewareTracksConcurrency(t *testing.T) {
	before := GetStats().ActiveConnections

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(StatsMiddleware())
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	r.GET("/slow", func(c *gin.Context) {
		entered <- struct{}{} // 通知测试：请求已进入 handler（并发已 +1）
		<-release
		c.String(http.StatusOK, "ok")
	})

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/slow", nil)
			r.ServeHTTP(rec, req)
		}()
	}

	// 等两个请求都进入 handler
	<-entered
	<-entered
	inFlight := GetStats().ActiveConnections - before
	assert.Equal(t, int64(2), inFlight)

	close(release)
	wg.Wait()

	// 全部结束后：并发归零，完成数 >= 2
	after := GetStats()
	assert.Equal(t, before, after.ActiveConnections)
	assert.GreaterOrEqual(t, after.CompletedLastMinute, int64(2))
}
