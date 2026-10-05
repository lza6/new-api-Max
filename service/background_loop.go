package service

import (
	"context"
	"sync"

	"github.com/bytedance/gopkg/util/gopool"
)

// backgroundLoop 管理一个常驻后台周期任务的生命周期。start 幂等：运行中重复
// 调用为 no-op；stop 取消 ctx 并等待 goroutine 退出后再返回。供优雅关闭
// （main.go）在 HTTP Shutdown 之前按序停止，避免停机期间后台任务继续写库。
// stop 之后可再次 start（便于测试与重启场景）；未 start 时 stop 为安全 no-op。
type backgroundLoop struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// start 启动后台 goroutine 并执行 run。run 必须响应 ctx.Done() 返回，
// 且不得 panic（与 gopool.Go 的约定一致）。
func (l *backgroundLoop) start(run func(ctx context.Context)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.wg.Add(1)
	gopool.Go(func() {
		defer l.wg.Done()
		run(ctx)
	})
}

// stop 取消后台 goroutine 并阻塞等待其退出。未运行时直接返回。
func (l *backgroundLoop) stop() {
	l.mu.Lock()
	cancel := l.cancel
	l.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	l.wg.Wait()
	l.mu.Lock()
	l.cancel = nil
	l.mu.Unlock()
}

// running 报告后台 goroutine 当前是否在运行（测试与健康观测用）。
func (l *backgroundLoop) running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cancel != nil
}
