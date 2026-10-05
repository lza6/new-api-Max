package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lza6/new-api-Max/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackgroundLoopStopWaitsForExit 验证 stop 会取消 ctx 并阻塞到 goroutine
// 真正退出（停机不丢「后台仍在写库」的窗口）。
func TestBackgroundLoopStopWaitsForExit(t *testing.T) {
	var loop backgroundLoop
	var exited atomic.Bool

	loop.start(func(ctx context.Context) {
		<-ctx.Done()
		// 模拟「取消后仍需片刻完成在途写库再退出」的窗口。
		time.Sleep(20 * time.Millisecond)
		exited.Store(true)
	})

	require.Eventually(t, func() bool {
		return loop.running()
	}, time.Second, 5*time.Millisecond, "loop should be running after start")

	loop.stop()

	assert.True(t, exited.Load(), "stop must wait for the goroutine to exit")
	assert.False(t, loop.running(), "loop must be marked stopped after stop returns")
}

// TestBackgroundLoopStartIsIdempotent 验证重复 start 不会启动第二个 goroutine。
func TestBackgroundLoopStartIsIdempotent(t *testing.T) {
	var loop backgroundLoop
	var starts atomic.Int32

	body := func(ctx context.Context) {
		starts.Add(1)
		<-ctx.Done()
	}
	loop.start(body)
	loop.start(body)
	loop.start(body)

	require.Eventually(t, func() bool {
		return starts.Load() == 1
	}, time.Second, 5*time.Millisecond)

	// 给潜在的第二个 goroutine 一点时间暴露。
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), starts.Load(), "only one goroutine may run")

	loop.stop()
}

// TestBackgroundLoopStopWithoutStartIsNoop 验证未启动时 stop 为安全 no-op。
func TestBackgroundLoopStopWithoutStartIsNoop(t *testing.T) {
	var loop backgroundLoop
	assert.NotPanics(t, func() { loop.stop() })
	assert.False(t, loop.running())
}

// TestBackgroundLoopRestartAfterStop 验证 stop 后可再次 start（测试/重启场景）。
func TestBackgroundLoopRestartAfterStop(t *testing.T) {
	var loop backgroundLoop
	var runs atomic.Int32

	loop.start(func(ctx context.Context) { runs.Add(1); <-ctx.Done() })
	require.Eventually(t, func() bool { return runs.Load() == 1 }, time.Second, 5*time.Millisecond)
	loop.stop()
	assert.False(t, loop.running())

	loop.start(func(ctx context.Context) { runs.Add(1); <-ctx.Done() })
	require.Eventually(t, func() bool { return runs.Load() == 2 }, time.Second, 5*time.Millisecond, "loop must restart after stop")
	loop.stop()
}

// TestStopSystemTaskRunnerHaltsLoop 验证真实 runner 的 Start/Stop 往返：停机后
// runner 不再运行，且重复 Stop 安全。
func TestStopSystemTaskRunnerHaltsLoop(t *testing.T) {
	truncate(t)
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = wasMaster })

	StartSystemTaskRunner()
	require.Eventually(t, func() bool { return systemTaskRunner.running() }, time.Second, 5*time.Millisecond)

	StopSystemTaskRunner()
	assert.False(t, systemTaskRunner.running(), "runner must be stopped after StopSystemTaskRunner")

	assert.NotPanics(t, StopSystemTaskRunner, "second Stop must be safe")
}
