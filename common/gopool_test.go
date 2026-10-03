package common

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRelayGoPoolWorkerCountBounded 4.2.4 回归：中继 gopool 的 worker 数必须被
// GOPOOL_MAX_WORKERS 上界约束 —— 高并发提交大量阻塞任务时，运行中 worker 数不得
// 超过上界（旧实现 math.MaxInt32 无上界）。
//
// 注意：gopool 的 Go 语义是「排队而非拒绝」，上界限制的是**同时运行**的 worker 数，
// 不丢任务。本测试用阻塞任务制造积压，读取峰值 worker 数断言 ≤ 上界。
func TestRelayGoPoolWorkerCountBounded(t *testing.T) {
	// 设置较小上界（一次性 env + 惰性构建；本进程首次触发 ensureRelayGoPool）。
	t.Setenv("GOPOOL_MAX_WORKERS", "4")
	// 强制惰性构建（若已被其它测试触发过，重置 once 以读新上界）。
	relayGoPoolOnce = sync.Once{}
	relayGoPool = nil

	ensureRelayGoPool()
	maxWorkers := RelayPoolMaxWorkers()
	require.Equal(t, int32(4), maxWorkers, "上界应读取 env GOPOOL_MAX_WORKERS")

	const tasks = 64
	release := make(chan struct{})
	var started atomic.Int32
	var peak atomic.Int32
	var wg sync.WaitGroup
	wg.Add(tasks)

	for range tasks {
		RelayCtxGo(context.Background(), func() {
			defer wg.Done()
			cur := started.Add(1)
			// 记录峰值并发 worker 数。
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			<-release // 阻塞直到测试放行，制造积压。
			started.Add(-1)
		})
	}

	// 等待一批 worker 起来（数量应被上界约束）。
	time.Sleep(200 * time.Millisecond)
	peakObserved := peak.Load()
	assert.LessOrEqual(t, peakObserved, maxWorkers,
		"运行中 worker 数不得超过上界 %d（实测峰值 %d）", maxWorkers, peakObserved)

	close(release)
	wg.Wait()
}
