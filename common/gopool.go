package common

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bytedance/gopkg/util/gopool"
)

// DefaultRelayPoolMaxWorkers 中继 gopool 的默认 worker 上限。
//
// [修复防御] 4.2.4：库默认 NewPool(..., math.MaxInt32, ...) 无上界，worker 数按
// 「积压任务数 ≥ ScaleThreshold(=1)」无限增长。中继热路径（stream_scanner 的
// scanner/data-handler/ping goroutine、SSE pinger、异步计费/额度落库）每请求都会
// 提交任务，2C2G 小机上缺少显式上界会造成一次性并发尖峰下 goroutine/内存放大。
// gopool 的 Go 是「排队而非拒绝」，给一个足够大的上界不会拒绝任务，只是限制同时
// 运行的 worker 数。默认 512，可用 GOPOOL_MAX_WORKERS 覆盖（<=0 回退默认）。
const DefaultRelayPoolMaxWorkers = 512

var (
	relayGoPoolOnce sync.Once
	relayGoPool     gopool.Pool
	// relayPoolWorkerCount 缓存 worker 计数（惰性构建后读取）。
	relayPoolMaxWorkers atomic.Int32
)

// ensureRelayGoPool 惰性构建中继 gopool。
//
// [修复防御] 启动期依赖 env 的共享对象不得在包 init() 里读 env —— init() 早于
// main() 里的 InitEnv()（.env 加载点），此时 GOPOOL_MAX_WORKERS 尚不可见。改为
// 首个实际使用点（RelayCtxGo）惰性构建，保证读到最终生效的配置。
func ensureRelayGoPool() {
	relayGoPoolOnce.Do(func() {
		maxWorkers := GetEnvOrDefault("GOPOOL_MAX_WORKERS", DefaultRelayPoolMaxWorkers)
		if maxWorkers <= 0 {
			SysError(fmt.Sprintf("GOPOOL_MAX_WORKERS must be positive, using default %d: configured=%d", DefaultRelayPoolMaxWorkers, maxWorkers))
			maxWorkers = DefaultRelayPoolMaxWorkers
		}
		relayPoolMaxWorkers.Store(int32(maxWorkers))
		relayGoPool = gopool.NewPool("gopool.RelayPool", int32(maxWorkers), gopool.NewConfig())
		relayGoPool.SetPanicHandler(func(ctx context.Context, i any) {
			if stopChan, ok := ctx.Value("stop_chan").(chan bool); ok {
				SafeSendBool(stopChan, true)
			}
			SysError(fmt.Sprintf("panic in gopool.RelayPool: %v", i))
		})
	})
}

func RelayCtxGo(ctx context.Context, f func()) {
	ensureRelayGoPool()
	relayGoPool.CtxGo(ctx, f)
}

// RelayWorkerCount 返回当前中继 gopool 的运行中 worker 数（可观测：高并发烟雾
// 测试后可据此确认 worker 数被上界约束）。未初始化时返回 0。
func RelayWorkerCount() int32 {
	if relayGoPool == nil {
		return 0
	}
	return relayGoPool.WorkerCount()
}

// RelayPoolMaxWorkers 返回生效的 worker 上界（未初始化时返回默认值）。
func RelayPoolMaxWorkers() int32 {
	if v := relayPoolMaxWorkers.Load(); v > 0 {
		return v
	}
	return DefaultRelayPoolMaxWorkers
}
