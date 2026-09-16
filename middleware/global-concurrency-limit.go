package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/setting/relay_setting"

	"github.com/gin-gonic/gin"
)

// T6 全局真实并发桶。
//
// 行为：管理员在"relay"设置里开启 global_concurrency 后，整个网关同时处理
// 中的模型请求数被限制在 GlobalConcurrencyLimit 之内；满时新请求进入有界
// FIFO 队列排队（而不是直接 429），一旦有请求完成释放并发即放行下一个。
// 排队超过 GlobalConcurrencyWaitTimeout 或队列已满则返回 429。
//
// 统计：active（当前处理中）+ waiting（排队中）可随时读取，供前端展示并发
// 水位。实现为"计数信号量 + 通道 FIFO 队列"；单一实例进程内有效，
// 多实例需接 Redis（本期不做，文档注明）。

type globalConcurrencyGate struct {
	mu      sync.Mutex
	active  int
	waiting int
	limit   int
	queue   int
	// fifo 保存等待中的通知通道，按入队顺序放行（先入先出）。
	fifo []chan struct{}
}

var globalConcurrency = &globalConcurrencyGate{}

// GlobalConcurrencyStats 并发水位快照。
type GlobalConcurrencyStats struct {
	Enabled bool `json:"enabled"`
	Active  int  `json:"active"`
	Waiting int  `json:"waiting"`
	Limit   int  `json:"limit"`
}

// GetGlobalConcurrencyStats 返回当前全局并发桶状态（开关/当前并发/排队数/上限）。
func GetGlobalConcurrencyStats() GlobalConcurrencyStats {
	cfg := relay_setting.GetGlobalConcurrencyGate()
	globalConcurrency.mu.Lock()
	defer globalConcurrency.mu.Unlock()
	return GlobalConcurrencyStats{
		Enabled: cfg.Enabled && cfg.Limit > 0,
		Active:  globalConcurrency.active,
		Waiting: globalConcurrency.waiting,
		Limit:   globalConcurrency.limit,
	}
}

// syncLimit 按最新设置刷新并发桶容量；不重置 active（避免打断进行中的请求）。
func (g *globalConcurrencyGate) syncLimit() {
	cfg := relay_setting.GetGlobalConcurrencyGate()
	g.mu.Lock()
	g.limit = cfg.Limit
	g.queue = cfg.Queue
	g.mu.Unlock()
}

// acquire 尝试获取并发令牌：
//   - 未开启或 limit<=0：直接放行
//   - 未满：占用一个槽位立即返回 true
//   - 已满：入 FIFO 队列等待；队列满返回 false（立即 429）；超时返回 false
func (g *globalConcurrencyGate) acquire(timeout time.Duration) (acquired bool, queueLimited bool) {
	g.mu.Lock()

	cfg := relay_setting.GetGlobalConcurrencyGate()
	if !cfg.Enabled || g.limit <= 0 {
		g.mu.Unlock()
		return true, false
	}

	if g.active < g.limit {
		g.active++
		g.mu.Unlock()
		return true, false
	}

	// 已满：队列也满 → 直接拒绝。
	if g.queue > 0 && g.waiting >= g.queue {
		g.mu.Unlock()
		return false, true
	}

	// 入队。
	g.waiting++
	ch := make(chan struct{})
	g.fifo = append(g.fifo, ch)
	g.mu.Unlock()

	// 等待通知或超时。
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
		// 已被唤醒：出队并占用槽位。此时信号量已在 release 中转移。
		return true, false
	case <-timer.C:
		// 超时：尝试把自己从队列移除（可能已被唤醒但仍未 receive）。
		g.mu.Lock()
		for i, waiting := range g.fifo {
			if waiting == ch {
				g.fifo = append(g.fifo[:i], g.fifo[i+1:]...)
				g.waiting--
				break
			}
		}
		g.mu.Unlock()
		return false, false
	}
}

// release 释放并发令牌；若有等待者则唤醒队首并在被唤醒者接管并发槽位。
func (g *globalConcurrencyGate) release() {
	g.mu.Lock()
	if g.active > 0 {
		g.active--
	}
	if len(g.fifo) > 0 {
		ch := g.fifo[0]
		g.fifo = g.fifo[1:]
		// 并发槽位直接转移给被唤醒者。
		g.active++
		g.waiting--
		g.mu.Unlock()
		close(ch)
		return
	}
	g.mu.Unlock()
}

// GlobalConcurrencyLimit 全局并发桶中间件。
func GlobalConcurrencyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		globalConcurrency.syncLimit()
		cfg := relay_setting.GetGlobalConcurrencyGate()
		if !cfg.Enabled || cfg.Limit <= 0 {
			c.Next()
			return
		}

		acquired, _ := globalConcurrency.acquire(time.Duration(cfg.WaitTimeout) * time.Second)
		if !acquired {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, "网关并发已满，请稍后再试（全局并发上限 "+itoa(cfg.Limit)+"）")
			return
		}
		defer globalConcurrency.release()
		c.Next()
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
