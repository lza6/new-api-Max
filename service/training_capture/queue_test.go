package training_capture

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 队列是「不影响用户请求延迟」的核心防线（宪法 V）。
// 一旦投递阻塞，用户的请求就跟着排队——所以「满队列投递必须立即返回」
// 是本文件最关键的断言。

func TestTryEnqueueSucceedsWhenNotFull(t *testing.T) {
	q := NewCaptureQueue(4)
	defer q.Close()

	assert.True(t, q.TryEnqueue(CaptureSignal{UserID: 1}))
	assert.True(t, q.TryEnqueue(CaptureSignal{UserID: 2}))

	stats := q.Stats()
	assert.Equal(t, 2, stats.Len)
	assert.Equal(t, int64(2), stats.Enqueued)
	assert.Zero(t, stats.Dropped)
}

func TestTryEnqueueNeverBlocksWhenFull(t *testing.T) {
	// **最关键的一条**：队列满时必须立刻返回 false，绝不能等待 worker。
	// 若这里阻塞，采集就直接变成用户请求的延迟。
	q := NewCaptureQueue(2)
	defer q.Close()

	require.True(t, q.TryEnqueue(CaptureSignal{UserID: 1}))
	require.True(t, q.TryEnqueue(CaptureSignal{UserID: 2}))

	// 已满：第 3 次投递必须在**微秒级**返回 false。
	done := make(chan bool, 1)
	start := time.Now()
	go func() { done <- q.TryEnqueue(CaptureSignal{UserID: 3}) }()

	select {
	case ok := <-done:
		assert.False(t, ok, "队列满时应返回 false")
		assert.Less(t, time.Since(start), 100*time.Millisecond,
			"投递必须立即返回，不得等待 worker（等待 = 用户的请求在排队）")
	case <-time.After(2 * time.Second):
		t.Fatal("投递阻塞了——这会让采集变成请求延迟，违反宪法 V")
	}

	assert.Equal(t, int64(1), q.DroppedCount(), "丢弃必须被计数（不能静默丢）")
}

func TestDroppedIsCountedNotSilent(t *testing.T) {
	// 静默丢弃会让运营方以为「采全了」，因此必须可观测。
	q := NewCaptureQueue(1)
	defer q.Close()

	require.True(t, q.TryEnqueue(CaptureSignal{UserID: 1}))
	for range 10 {
		assert.False(t, q.TryEnqueue(CaptureSignal{UserID: 2}))
	}

	stats := q.Stats()
	assert.Equal(t, int64(1), stats.Enqueued)
	assert.Equal(t, int64(10), stats.Dropped, "10 次满队列投递都必须被计数")
}

func TestReceiveDrainsInOrder(t *testing.T) {
	q := NewCaptureQueue(4)
	defer q.Close()

	for i := range 3 {
		require.True(t, q.TryEnqueue(CaptureSignal{UserID: i + 1}))
	}

	for i := range 3 {
		signal, ok := q.Receive()
		require.True(t, ok)
		assert.Equal(t, i+1, signal.UserID, "FIFO 顺序必须保持")
	}
}

func TestReceiveReturnsFalseAfterClose(t *testing.T) {
	q := NewCaptureQueue(4)
	q.TryEnqueue(CaptureSignal{UserID: 1})
	q.Close()

	// 关闭后仍能取出缓冲中的剩余元素（平滑停机，不丢已入队数据）。
	signal, ok := q.Receive()
	require.True(t, ok)
	assert.Equal(t, 1, signal.UserID)

	// 取空后返回 false，worker 据此退出。
	_, ok = q.Receive()
	assert.False(t, ok, "队列关闭且取空后必须返回 false 让 worker 退出")
}

func TestCloseIsIdempotent(t *testing.T) {
	q := NewCaptureQueue(2)
	assert.NotPanics(t, func() {
		q.Close()
		q.Close()
		q.Close()
	})
}

func TestTryEnqueueAfterCloseDoesNotPanic(t *testing.T) {
	// close(ch) 之后再 send 会 panic。必须靠 closed 标志挡住——
	// 否则运行中关闭采集功能会让网关崩溃。
	q := NewCaptureQueue(2)
	q.Close()

	assert.NotPanics(t, func() {
		for range 5 {
			assert.False(t, q.TryEnqueue(CaptureSignal{UserID: 1}))
		}
	})
}

func TestNilQueueIsSafe(t *testing.T) {
	// 采集功能关闭时调用方可能持有 nil 队列；所有方法都必须容忍。
	var q *CaptureQueue
	assert.False(t, q.TryEnqueue(CaptureSignal{}))
	_, ok := q.Receive()
	assert.False(t, ok)
	assert.NotPanics(t, func() { q.Close() })
	assert.Zero(t, q.DroppedCount())
	assert.Equal(t, QueueStats{}, q.Stats())
}

func TestQueueCapacityIsBounded(t *testing.T) {
	// 内存不随流量无限增长（容器限 900MB）。
	q := NewCaptureQueue(8)
	defer q.Close()

	for i := range 1000 {
		q.TryEnqueue(CaptureSignal{UserID: i})
	}

	stats := q.Stats()
	assert.Equal(t, 8, stats.Capacity)
	assert.Equal(t, 8, stats.Len, "积压不得超过容量")
	assert.LessOrEqual(t, stats.Enqueued+stats.Dropped, int64(1000))
	assert.Equal(t, int64(1000), stats.Enqueued+stats.Dropped, "每次投递要么入队要么丢弃，不能凭空消失")
}

func TestDefaultCapacityWhenNonPositive(t *testing.T) {
	q := NewCaptureQueue(0)
	defer q.Close()
	assert.Equal(t, CaptureQueueDefaultCapacity, q.Stats().Capacity)

	q2 := NewCaptureQueue(-1)
	defer q2.Close()
	assert.Equal(t, CaptureQueueDefaultCapacity, q2.Stats().Capacity)
}

func TestConcurrentEnqueueIsSafeAndAccounted(t *testing.T) {
	// -race 下验证：并发投递不得丢计数、不得 panic。
	q := NewCaptureQueue(64)
	defer q.Close()

	const workers = 8
	const perWorker = 500
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range perWorker {
				q.TryEnqueue(CaptureSignal{UserID: worker*perWorker + i})
			}
		}(w)
	}
	wg.Wait()

	stats := q.Stats()
	assert.Equal(t, int64(workers*perWorker), stats.Enqueued+stats.Dropped,
		"并发下「入队 + 丢弃」必须等于总投递次数")
	assert.LessOrEqual(t, stats.Len, stats.Capacity)
}

func TestConcurrentEnqueueAndReceiveKeepsAccountingConsistent(t *testing.T) {
	// 生产者与消费者并发：不得 panic、不得丢计数。
	q := NewCaptureQueue(16)
	const total = 2000
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range total {
			q.TryEnqueue(CaptureSignal{UserID: i})
		}
		q.Close()
	}()

	received := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, ok := q.Receive()
			if !ok {
				return
			}
			received++
		}
	}()

	wg.Wait()

	stats := q.Stats()
	assert.Equal(t, int64(total), stats.Enqueued+stats.Dropped)
	assert.LessOrEqual(t, received, int(stats.Enqueued), "收到的不能多于入队的")
}

func TestEnqueueIsFastEnoughForHotPath(t *testing.T) {
	// 热路径调用开销的量级检查：10 万次投递应在远低于 1 秒内完成
	// （即单次 << 10µs，对 <10ms 的每请求开销预算可忽略）。
	q := NewCaptureQueue(64)
	defer q.Close()

	const n = 100_000
	start := time.Now()
	for i := range n {
		q.TryEnqueue(CaptureSignal{UserID: i})
	}
	elapsed := time.Since(start)

	assert.Less(t, elapsed, time.Second,
		"10 万次投递耗时 %v，过慢则不适合放在热路径", elapsed)
	t.Logf("10 万次投递耗时 %v（平均 %v/次）", elapsed, elapsed/n)
}

// 回归（B1）：`TryEnqueue` 与 `Close` **并发**时绝不能 panic。
//
// 这是原实现的一个真实 BLOCKING 缺陷：`TryEnqueue` 的「读 closed 标志」与
// 「send」是两步，`Close` 的「置标志」与 `close(ch)` 也是两步；交错时会
// `panic: send on closed channel`。
//
// 为什么这个 panic 是**进程级**事故而不是「丢一条采集」：
// 接线后 `TryEnqueue` 跑在 **relay 请求 goroutine** 上，panic 会杀掉整个网关。
// 而触发条件只是「停机/切换总开关与在途请求并发」——正常运维场景。
//
// 原测试 `TestTryEnqueueAfterCloseDoesNotPanic` 是**串行**的（先 Close 再 Enqueue），
// 恰好绕过并发窗口，所以没能发现。本用例补上这个窗口。
func TestTryEnqueueConcurrentWithCloseNeverPanics(t *testing.T) {
	for range 200 {
		q := NewCaptureQueue(4)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 200 {
				q.TryEnqueue(CaptureSignal{UserID: 1})
			}
		}()
		go func() {
			defer wg.Done()
			q.Close()
		}()
		wg.Wait()
	}
}

// 回归（B1 补充）：多个生产者与 Close 并发，且带 race 检测。
func TestTryEnqueueManyWritersConcurrentWithClose(t *testing.T) {
	for range 50 {
		q := NewCaptureQueue(8)
		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 100 {
					q.TryEnqueue(CaptureSignal{UserID: 2})
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Close()
		}()
		wg.Wait()
	}
}
