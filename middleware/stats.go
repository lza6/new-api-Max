package middleware

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// completedWindowSize 覆盖最近一分钟的按秒分桶数量（61 格，多出一格避免整分边界竞态）。
const completedWindowSize = 61

// completedWindow 近一分钟已完成请求的滑动窗口（按秒分桶，互斥锁保护）。
type completedWindow struct {
	mu     sync.Mutex
	marks  [completedWindowSize]int64 // 每个桶对应的 unix 秒
	counts [completedWindowSize]int64 // 每个桶内的完成数量
}

func (w *completedWindow) add(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	sec := now.Unix()
	idx := sec % completedWindowSize
	if w.marks[idx] != sec {
		w.marks[idx] = sec
		w.counts[idx] = 0
	}
	w.counts[idx]++
}

func (w *completedWindow) sumLastMinute(now time.Time) int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	sec := now.Unix()
	var sum int64
	for i := range w.counts {
		if w.marks[i] >= sec-59 {
			sum += w.counts[i]
		}
	}
	return sum
}

// HTTPStats 存储HTTP统计信息
type HTTPStats struct {
	activeConnections int64
	completed         completedWindow
}

var globalStats = &HTTPStats{}

// StatsMiddleware 统计中间件
func StatsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 增加活跃连接数
		atomic.AddInt64(&globalStats.activeConnections, 1)

		// 确保在请求结束时减少连接数，并记录完成数
		defer func() {
			atomic.AddInt64(&globalStats.activeConnections, -1)
			globalStats.completed.add(time.Now())
		}()

		c.Next()
	}
}

// StatsInfo 统计信息结构
type StatsInfo struct {
	ActiveConnections   int64 `json:"active_connections"`
	CompletedLastMinute int64 `json:"completed_last_minute"`
}

// GetStats 获取统计信息
func GetStats() StatsInfo {
	return StatsInfo{
		ActiveConnections:   atomic.LoadInt64(&globalStats.activeConnections),
		CompletedLastMinute: globalStats.completed.sumLastMinute(time.Now()),
	}
}
