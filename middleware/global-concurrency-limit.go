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
	// fifo 保存等待中的队列元素，按入队顺序放行（先入先出）。
	fifo []*concurrencyWaiter
}

// concurrencyWaiter 队首等待者。
//
// [修复防御] 4.2.1：旧实现 release() 在看到队列非空时**无条件**把槽位「转移」
// 给队首（active++ 后 close(ch)）。但队首可能已在 acquire 的 timer.C 分支离场
// （已把自己从 fifo 移除并返回 false，再也不会 receive 该 ch）——此时槽位被凭空
// +1 且永无人 release：净泄漏 1 个槽位，长跑累积后 active 恒 >= limit，新请求全部
// 排队直至超时 429（桶自我堵死）。
//
// 现引入「归属仲裁」：granted 由 gate.mu 保护，仅当该元素确为队首、被 release
// 选中交接时才置位。等待者若发现「已被授予槽位却选择超时放弃」，必须在锁内
// 归还槽位（active--）并把槽位顺延给下一个等待者，杜绝幽灵占用。
type concurrencyWaiter struct {
	ch      chan struct{}
	granted bool // gate.mu 保护：release 已在锁内把槽位交予本等待者
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
//   - 已满：入 FIFO 队列等待；队列满返回 false（立即 429）；超时若尚未被交接则
//     出队返回 false，若恰在超时瞬间被 release 交接了槽位，则**在锁内归还**并顺延
//     给下一个等待者后返回 false（杜绝幽灵占用）。
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
	w := &concurrencyWaiter{ch: make(chan struct{})}
	g.fifo = append(g.fifo, w)
	g.mu.Unlock()

	// 等待通知或超时。
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-w.ch:
		// 已被 release 交接：槽位已在 release 锁内计入 active，直接接管。
		return true, false
	case <-timer.C:
		// 超时：在锁内与 release 做归属仲裁。
		g.mu.Lock()
		if w.granted {
			// 竞态：release 恰在超时瞬间把槽位交接给了本等待者（active 已被 release
			// 计入本等待者名下），但本 goroutine 已走超时分支、不再接收。
			// 槽位必须顺延给下一个等待者（active 保持不变，仅换持有者）；若无人
			// 接手则回收槽位（active--）。否则该槽位永久泄漏（4.2.1 根因）。
			if !g.grantSlotLocked() && g.active > 0 {
				g.active--
			}
		} else {
			// 常规超时：把自己从队列移除（此时必仍在队列中；release 未选中本元素）。
			for i, item := range g.fifo {
				if item == w {
					last := len(g.fifo) - 1
					copy(g.fifo[i:], g.fifo[i+1:])
					// 清尾部槽位，避免底层数组残留对末元素的引用（slice 泄漏）。
					g.fifo[last] = nil
					g.fifo = g.fifo[:last]
					g.waiting--
					break
				}
			}
		}
		g.mu.Unlock()
		return false, false
	}
}

// grantSlotLocked 在**已持有 g.mu** 的前提下，把队首等待者标记为已获槽位并发信号。
// 仅当队首存在时调用；不在此处增减 active（由调用方决定语义：release 场景由
// release 负责 active 的平衡，超时归还场景由调用方先 active-- 再顺延）。
// 返回是否成功交接。
func (g *globalConcurrencyGate) grantSlotLocked() bool {
	if len(g.fifo) == 0 {
		return false
	}
	w := g.fifo[0]
	g.fifo = g.fifo[1:]
	g.waiting--
	w.granted = true
	// close 放在锁内：保证「置 granted」与「通知」原子可见——等待者收到信号时
	// granted 必为 true；而 release 先置位后 close，等待者超时分支读到 granted
	// 也一定与 close 顺序一致（同一把锁串行化）。
	close(w.ch)
	return true
}

// release 释放并发令牌：若队首等待者仍在，则把槽位交接给它；否则回收槽位。
//
// [修复防御] 4.2.1：不再「出队后无条件 active++」。active 的平衡完全由本函数
// 的 active-- 与「等待者是否真正接收」决定——交接出去的槽位由被唤醒方持有，
// 若对方已超时离开则由其超时分支归还（见 acquire）。
func (g *globalConcurrencyGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active > 0 {
		g.active--
	}
	// 尝试把槽位交接给队首等待者：交接成功则槽位由对方持有 → active++。
	if g.grantSlotLocked() {
		g.active++
	}
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
