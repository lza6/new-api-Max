package training_capture

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lza6/new-api-Max/relaykit/types"
)

// 高并发压测（用户明确要求：「方便极端以及高并发的话你还要对该模块进行压测」）。
//
// 压测的目标**不是吞吐**（采集是尽力而为的辅助功能），而是三条安全性：
//  1. **绝不 panic**：请求路径上的 panic 会杀掉整个网关进程；
//  2. **绝不阻塞**：队列满必须立即丢弃，不能反压到用户请求；
//  3. **计数守恒**：enqueued + dropped 必须等于投递尝试数
//     （不守恒说明丢了计数，运营方会误判「采全了」）。
//
// 同时跑真实 worker + 真实 sink，让「生产端与消费端并发」这一
// 最容易出问题的组合被覆盖到。

// captureStressConfig 压测参数。
type captureStressConfig struct {
	writers        int
	perWriter      int
	queueCapacity  int
	textBytes      int
	withWorker     bool
	expectDropMiss bool // 是否允许「零丢弃」（容量足够时）
}

func runCaptureStress(t *testing.T, cfg captureStressConfig) (enqueued, dropped, attempted int64) {
	t.Helper()
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}

	sink := newMemSink()
	restoreSink := SetSinkForTest(sink)
	t.Cleanup(restoreSink)
	restoreGateway := SetGatewayEnabledForTest(true)
	t.Cleanup(restoreGateway)
	defer ResetConsentCacheForTest()

	originalQueue := captureQueue
	captureQueue = NewCaptureQueue(cfg.queueCapacity)
	t.Cleanup(func() { captureQueue = originalQueue })

	// 用**真实 worker**：生产端与消费端并发是压测的核心，
	// 只压队列不压 worker 会漏掉「worker 处理慢导致队列满」这条真实路径。
	if cfg.withWorker {
		// 走**真实启停 API**：手工重置内部状态会绕过生产路径，
		// 而那正是「关后再开 worker 永久死亡」这类缺陷的藏身处。
		if !StartWorker() {
			t.Fatal("StartWorker 应实际启动 worker")
		}
		t.Cleanup(waitWorkerStopped)
	}

	// 所有写者共用一个授权用户（授权判定走缓存，不会成为瓶颈）。
	const userId = 70001
	grantForTest(t, userId)

	meta := &types.TokenCountMeta{CombineText: strings.Repeat("x", cfg.textBytes)}

	var attemptedCount atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for w := range cfg.writers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range cfg.perWriter {
				// 每轮都重新构造信号（模拟真实请求各自的 meta）。
				signal := BuildSignal(CaptureInput{
					UserId: userId, Username: "stress", Model: "m",
					// 分散会话 id：避免所有信号挤在同一条水位上。
					ConversationID: "stress-" + string(rune('a'+worker%26)) + "-" + itoaSmall(i%8),
					Meta:           meta,
				})
				if signal == nil {
					continue
				}
				attemptedCount.Add(1)
				MaybeCapture(*signal)
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)

	stats := captureQueue.Stats()
	t.Logf("压测：%d 写者 × %d 次 = %d 投递，耗时 %v（%.0f 次/秒）",
		cfg.writers, cfg.perWriter, attemptedCount.Load(), elapsed,
		float64(attemptedCount.Load())/elapsed.Seconds())
	t.Logf("      队列：容量=%d 积压=%d 入队=%d 丢弃=%d",
		stats.Capacity, stats.Len, stats.Enqueued, stats.Dropped)

	return stats.Enqueued, stats.Dropped, attemptedCount.Load()
}

// TestStressHotPathNeverPanicsOrBlocks 是压测主用例。
//
// 在「写者远超队列容量 + worker 同时在消费」的最恶劣组合下，
// 验证计数守恒 —— 这是「非阻塞投递」正确性的机器可验证形式：
// 若投递会阻塞，写者数 × 次数就会在超时前跑不完。
func TestStressHotPathNeverPanicsOrBlocks(t *testing.T) {
	enqueued, dropped, attempted := runCaptureStress(t, captureStressConfig{
		writers:       32,
		perWriter:     5000,
		queueCapacity: 64, // 故意远小于写入量，逼出「队列满立即丢弃」路径
		textBytes:     512,
		withWorker:    true,
	})

	if attempted == 0 {
		t.Fatal("没有任何投递尝试 —— 数据形态或开关有问题")
	}
	// **计数守恒**：入队 + 丢弃 == 尝试数。
	// 不守恒意味着有投递被静默吞掉（既没入队也没计丢弃）。
	if enqueued+dropped != attempted {
		t.Fatalf("计数不守恒：入队 %d + 丢弃 %d = %d，但尝试了 %d 次",
			enqueued, dropped, enqueued+dropped, attempted)
	}

	// 写者停下的**那一刻**队列必然是满的（64 容量扛不住 16 万次投递），
	// 不能立刻断言「未满」——那是在要求 worker 追平瞬间洪峰，不合理。
	// 要验证的是「worker 确实在消费」，即队列会**排空**。
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if captureQueue.Stats().Len == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if final := captureQueue.Stats(); final.Len != 0 {
		t.Fatalf("worker 未在 20s 内排空队列（剩余 %d/%d）—— 消费者没在工作",
			final.Len, final.Capacity)
	}
}

// TestStressWithoutWorkerQueueStaysBounded 验证「worker 挂了」时
// 队列仍是硬上限，不会因积压而吃光内存。
func TestStressWithoutWorkerQueueStaysBounded(t *testing.T) {
	enqueued, dropped, attempted := runCaptureStress(t, captureStressConfig{
		writers:       16,
		perWriter:     3000,
		queueCapacity: 128,
		textBytes:     4096, // 4KB/条，最坏内存占用 = 128 × 4KB = 512KB
		withWorker:    false,
	})

	if enqueued+dropped != attempted {
		t.Fatalf("计数不守恒：%d + %d != %d", enqueued, dropped, attempted)
	}
	if enqueued > 128 {
		t.Fatalf("无消费者时入队数超过容量 %d：%d", 128, enqueued)
	}
	if dropped == 0 {
		t.Fatal("无消费者且写入量远超容量，必然有丢弃；丢弃数为 0 说明计数失效")
	}

	// 最坏内存占用必须在容器预算内（700MB 安全线）。
	worstCaseBytes := int64(128) * (maxCapturedTextBytes + 256)
	if worstCaseBytes > 700<<20 {
		t.Fatalf("队列最坏占用 %d 字节，超出预算", worstCaseBytes)
	}
	t.Logf("无消费者时队列最坏占用 %d 字节（容量 128 × 单条上限 %d）",
		worstCaseBytes, maxCapturedTextBytes)
}

// TestStressTogglingGatewayDuringLoad 覆盖「运维在高压下改开关」。
//
// 这正是 B1（send on closed channel → 杀进程）的触发场景：
// 停机/切换开关与在途请求并发。压测必须在**开关反复切换**下仍然无 panic。
func TestStressTogglingGatewayDuringLoad(t *testing.T) {
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	sink := newMemSink()
	restoreSink := SetSinkForTest(sink)
	t.Cleanup(restoreSink)
	defer ResetConsentCacheForTest()

	originalQueue := captureQueue
	captureQueue = NewCaptureQueue(256)
	t.Cleanup(func() { captureQueue = originalQueue })

	if !StartWorker() {
		t.Fatal("StartWorker 应实际启动 worker")
	}
	t.Cleanup(waitWorkerStopped)

	const userId = 70002
	SetGatewayEnabled(true)
	grantForTest(t, userId)

	meta := &types.TokenCountMeta{CombineText: strings.Repeat("y", 256)}
	stop := make(chan struct{})

	// 写者：持续投递。
	var writers sync.WaitGroup
	for w := range 16 {
		writers.Add(1)
		go func(worker int) {
			defer writers.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				signal := BuildSignal(CaptureInput{
					UserId: userId, Username: "toggle", Model: "m",
					ConversationID: "toggle-" + itoaSmall(worker) + "-" + itoaSmall(i%16),
					Meta:           meta,
				})
				if signal != nil {
					MaybeCapture(*signal)
				}
			}
		}(w)
	}

	// 切换者：反复开/关总开关（同时会重建限速、同步热路径标志）。
	// 这一步是本用例的关键：它触发 `SyncHotPathFlag` 与队列/worker 的交互。
	for i := range 100 {
		SetGatewayEnabled(i%2 == 0)
		time.Sleep(2 * time.Millisecond)
	}
	SetGatewayEnabled(true)

	close(stop)
	writers.Wait() // 若投递会阻塞，这里会超时

	// 无 panic 即通过（panic 会直接终止测试进程）。
	enqueued, dropped := captureQueue.Stats().Enqueued, captureQueue.Stats().Dropped
	t.Logf("开关切换 100 次后：入队 %d，丢弃 %d（无 panic、无死锁）", enqueued, dropped)
	if enqueued == 0 {
		t.Fatal("压测期间一次都没入队 —— 开关同步可能失效")
	}
}

// TestStressUnboundedUsersDoNotExplodeMemory 覆盖「大量不同用户并发」。
//
// 每个用户一条水位（DB 行）+ 目录（云盘），这是最容易造成
// 「表/目录无界增长」的形态。这里验证**进程内**状态有界。
//
// 刻意**不**通过 DB 给大量用户授权：那要为每个用户做一次 upsert，
// 在 SQLite 上 5000 次要几分钟（实测把用例拖到超时）。
// 要验证的是「缓存/集合的容量保护生效」，直接驱动容量保护那段代码
// 更精确、也更快 —— 而且不依赖 DB 的实现细节。
func TestStressUnboundedUsersDoNotExplodeMemory(t *testing.T) {
	ResetConsentCacheForTest()
	t.Cleanup(ResetConsentCacheForTest)

	// 写入远超容量上限的条目，验证缓存被硬守住。
	// （容量保护是「满了整体清空」，因此最终大小在 [1, 上限] 之间。）
	overflow := consentCacheMaxEntries + consentCacheMaxEntries/2
	for i := range overflow {
		storeConsentCache(i+1, consentEntry{
			granted:       true,
			policyVersion: CurrentPolicyVersion,
			expiresAt:     time.Now().Add(time.Minute),
		})
	}

	size := consentCacheSizeForTest()
	if size > consentCacheMaxEntries {
		t.Fatalf("授权缓存 %d 条，超过上限 %d —— 无界增长会随用户数吃光内存",
			size, consentCacheMaxEntries)
	}
	if size == 0 {
		t.Fatal("写入后缓存仍为空 —— 容量保护把数据全清了，缓存完全失效")
	}
	t.Logf("写入 %d 条后缓存 %d 条（上限 %d）—— 有界", overflow, size, consentCacheMaxEntries)

	// 全局指纹集合同样有界（它按对话量增长，无界的话一年 2.3GB）。
	set := NewBoundedFingerprintSet(1000)
	for i := range 50_000 {
		set.Add(HashText(itoaSmall(i)))
	}
	if set.Len() > set.Capacity() {
		t.Fatalf("指纹集合 %d 条，超过容量 %d", set.Len(), set.Capacity())
	}
	t.Logf("写入 50000 条指纹后集合 %d 条（容量 %d）—— 有界", set.Len(), set.Capacity())
}

// TestStressManyDistinctUsersThroughHotPath 用**真实授权路径**覆盖多用户。
//
// 上一条直接驱动容量保护；这一条走 `IsGranted`（真实判定 + 缓存回源），
// 验证「大量不同用户」不会让内存或延迟劣化。用户数取较小值，
// 因为每个都要一次真实 DB 写入。
func TestStressManyDistinctUsersThroughHotPath(t *testing.T) {
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	// IsGranted 的三个前置条件之一就是「云盘已配置」——不注入 sink
	// 会让所有判定都返回 false（那是正确行为，但会让本用例测错东西）。
	defer SetSinkForTest(fakeSink{name: "stress"})()
	SetGatewayEnabled(true)
	defer SetGatewayEnabled(true)
	ResetConsentCacheForTest()

	// 用户数取足够小：每个都要一次真实 DB 写入，SQLite 下单次约 100ms
	// （300 个用户就让用例跑到 36s）。要验证的是「缓存命中快 + 有界」，
	// 100 个用户与 300 个对这个结论没有区别。
	const users = 100
	for i := range users {
		if err := GrantConsent(200000+i, CurrentPolicyVersion); err != nil {
			t.Fatalf("授权写入失败：%v", err)
		}
	}
	ResetConsentCacheForTest()

	// 全部走一次真实判定（第二次即为缓存命中）。
	for i := range users {
		if !IsGranted(200000 + i) {
			t.Fatalf("用户 %d 已授权却判为不可采集", 200000+i)
		}
	}
	// 再走一遍（此时全为缓存命中，不应回源）。
	start := time.Now()
	for range 10 {
		for i := range users {
			_ = IsGranted(200000 + i)
		}
	}
	elapsed := time.Since(start)
	perCall := elapsed / time.Duration(10*users)
	t.Logf("授权判定 %d 次耗时 %v（缓存命中 %v/次）", 10*users, elapsed, perCall)

	// 热路径要求：缓存命中时必须远快于一次函数调用级别（<10µs）。
	if perCall > 10*time.Microsecond {
		t.Fatalf("缓存命中耗时 %v/次，过慢 —— 会拖慢请求", perCall)
	}
	if size := consentCacheSizeForTest(); size > consentCacheMaxEntries {
		t.Fatalf("授权缓存 %d 条超过上限", size)
	}
}
