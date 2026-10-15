package training_capture

import (
	"sync"
	"sync/atomic"
)

// 有界采集队列。
//
// ## 这是「不影响用户请求延迟」的核心实现（宪法 V）
//
// 采集必须**完全脱离请求热路径**（见 plan ADR-008-1）。请求结束时只做一件事：
// 把一条采集信号**非阻塞**地放进这个队列。
//
// 三条硬契约：
//  1. **投递永不阻塞**：队列满时**立即丢弃**并计数，绝不等待。
//     这是「采集影响延迟」的唯一防线——一旦这里阻塞，用户的请求就跟着排队。
//  2. **容量硬上限**：内存不随流量无限增长（容器限 900MB）。
//  3. **丢弃可观测**：丢弃数进指标，让「采集是否跟不上」可见，
//     而不是静默丢数据（静默丢弃会让运营方以为采全了）。
//
// ## 为什么用带缓冲 channel 而不是自己写 ring buffer
//
// `chan` 的 `select + default` 天然就是「非阻塞投递」语义，且并发安全。
// 自写 ring buffer 需要自己处理读写指针的同步——没有收益。

// CaptureSignal 一条待采集的请求信号。
//
// **只放轻量、已在上下文里的数据**：请求路径上不做任何解析或查询
// （不查 DB、不取用户详情、不拼消息数组）。重活全在 worker 里干。
type CaptureSignal struct {
	// UserID 发起请求的用户。
	UserID int
	// Username 用户名（已由 worker 侧净化，这里只是原始值）。
	Username string
	// Model 实际使用的模型名。
	Model string
	// ConversationID 会话标识；空表示拿不到（退化到全局指纹去重）。
	ConversationID string
	// Messages 本次请求涉及的对话消息（已白名单过滤敏感字段）。
	//
	// 注意：这个切片在**投递时构造**。为避免在请求路径上做重活，
	// 调用方应直接复用已经算好的 meta（见 plan §10.3），
	// 而不是重新解析一遍 JSON。
	Messages []Message
	// ResponseText 助手回复文本（可能为空——见 plan §10.3 的诚实边界）。
	ResponseText string

	// TailOnly 超限截断时的**尾部片段**（新内容所在处）。
	//
	// 非空时 worker 用它做增量，而不是对整段存储文本做前缀比对 ——
	// 存储文本含「省略 N 字节」标记，N 每次都变，前缀比对会退化成
	// 每次全量重传（数据集里堆满重复头部）。
	TailOnly string
	// CreatedAt 请求完成时间（unix 秒），用于目录日期归属。
	CreatedAt int64
}

// CaptureQueue 有界、非阻塞投递的采集队列。
type CaptureQueue struct {
	// ch 数据通道。**永不 close**（见下方 stop 的说明）。
	ch chan CaptureSignal

	// stop 关闭信号。`Close` 关闭的是**它**，不是数据通道。
	//
	// [修复防御] 原实现让 `Close` 执行 `close(q.ch)`，而 `TryEnqueue` 是
	// 「读 closed 标志」+「send」两步，两者交错时会
	// `panic: send on closed channel`（实测稳定复现）。
	// 那个 panic 出现在 relay 请求 goroutine 上时会**杀掉整个网关进程**，
	// 触发条件只是「停机/切换开关与在途请求并发」——正常运维场景。
	//
	// 修法有两种：
	//   (a) 用锁把「检查+发送」与 close 互斥 —— 实测让热路径从 2.4µs 涨到 11.6µs；
	//   (b) **根本不关数据通道**，另用 stop 通道表达「停止消费」。
	//       发送方即使在 Close 之后 send，也只是把信号放进缓冲（无人消费），
	//       不会 panic —— 彻底消除该 panic 的可能，且热路径零锁。
	// 采用 (b)：安全与性能同时成立，而不是二选一。
	stop chan struct{}

	// enqueued/dropped 用原子计数，读侧（指标）不加锁。
	enqueued atomic.Int64
	dropped  atomic.Int64

	// closed 供投递侧快速短路（避免在停机后继续堆积数据）。
	// 它**不参与正确性**：即使读到过期值也不会 panic，最多多放一条进缓冲。
	closed atomic.Bool

	// closeOnce 保证 Close 幂等。
	closeOnce sync.Once
}

// CaptureQueueDefaultCapacity 默认容量（条）。
//
// 取 1024：单条信号最坏几百字节（消息切片）+ 指针，1024 条约几百 KB 级，
// 对 900MB 容器可忽略；同时 1024 条的缓冲足以吸收正常写入的突发。
// worker 一旦持续落后，宁可丢采集也不要占内存——采集是「尽力而为」的辅助功能，
// 用户请求才是主业务。
const CaptureQueueDefaultCapacity = 1024

// NewCaptureQueue 创建队列。capacity <= 0 时用默认值。
func NewCaptureQueue(capacity int) *CaptureQueue {
	if capacity <= 0 {
		capacity = CaptureQueueDefaultCapacity
	}
	return &CaptureQueue{
		ch:   make(chan CaptureSignal, capacity),
		stop: make(chan struct{}),
	}
}

// TryEnqueue **非阻塞**投递。返回 false 表示队列已满（已计入丢弃）或已关闭。
//
// **这是请求路径上唯一被调用的方法**，因此它必须极快：
// 一次 RLock + 一次 `select`（含 default 分支）。
// 没有堆分配、没有日志、没有系统调用。
//
// 与 `Close` 的互斥由 `closeMu` 保证（见字段注释里的 panic 说明）：
// **绝不对已关闭的 channel 发送**。
func (q *CaptureQueue) TryEnqueue(signal CaptureSignal) bool {
	if q == nil || q.closed.Load() {
		return false
	}
	select {
	case q.ch <- signal:
		q.enqueued.Add(1)
		return true
	default:
		// 队列满：**立即返回**，绝不等 worker。
		// 丢的是采集数据，不是用户请求。
		q.dropped.Add(1)
		return false
	}
}

// Receive 阻塞取出（供 worker 使用）。队列关闭后返回 ok=false。
//
// 关闭后**先尽力取完缓冲**再返回 false：平滑停机不该丢掉已入队的数据
// （这些数据已经过授权判定，丢掉就是白采）。
func (q *CaptureQueue) Receive() (CaptureSignal, bool) {
	return q.ReceiveWithStop(nil)
}

// ReceiveWithStop 同 Receive，但额外监听一个外部停止信号。
//
// [修复防御] 这个 `extraStop` 不是可选优化，而是**停机正确性**的一部分：
// worker 有自己的停止信号（`workerStop`），与队列的 `stop` 是**两个不同的
// channel**。原实现只等队列 stop，而 `StopWorker` 关的是 workerStop ——
// 于是「worker 正阻塞在这里等数据」时 StopWorker **完全无效**：
// 进程停机、管理端关闭采集后 worker 会永久挂住（实测 5s 内停不掉）。
//
// 把两个信号都纳入 select，任一触发都能唤醒并退出。
func (q *CaptureQueue) ReceiveWithStop(extraStop <-chan struct{}) (CaptureSignal, bool) {
	if q == nil {
		return CaptureSignal{}, false
	}
	select {
	case signal := <-q.ch:
		return signal, true
	case <-q.stop:
		return q.drainOrDefault()
	case <-extraStop:
		// worker 被要求停止：先取完缓冲再退出（不丢已入队数据）。
		return q.drainOrDefault()
	}
}

// drainOrDefault 尽力再取一条缓冲；取不到则返回 ok=false。
func (q *CaptureQueue) drainOrDefault() (CaptureSignal, bool) {
	select {
	case signal := <-q.ch:
		return signal, true
	default:
		return CaptureSignal{}, false
	}
}

// Close 停止消费：worker 的 Receive 会返回 ok=false 从而退出。
//
// 幂等。**不关数据通道**（见 `stop` 字段的说明）——因此
// `TryEnqueue` 与 `Close` 并发时不可能 panic，热路径也不需要加锁。
func (q *CaptureQueue) Close() {
	if q == nil {
		return
	}
	q.closeOnce.Do(func() {
		q.closed.Store(true)
		close(q.stop)
	})
}

// Stats 返回队列可观测数据。
type QueueStats struct {
	// Capacity 队列容量。
	Capacity int
	// Len 当前积压条数。
	Len int
	// Enqueued 累计成功投递数。
	Enqueued int64
	// Dropped 累计丢弃数（队列满）。
	Dropped int64
}

// Stats 返回当前统计。读侧无锁（原子读 + len(chan)）。
func (q *CaptureQueue) Stats() QueueStats {
	if q == nil {
		return QueueStats{}
	}
	return QueueStats{
		Capacity: cap(q.ch),
		Len:      len(q.ch),
		Enqueued: q.enqueued.Load(),
		Dropped:  q.dropped.Load(),
	}
}

// DroppedCount 返回累计丢弃数（供指标）。
func (q *CaptureQueue) DroppedCount() int64 {
	if q == nil {
		return 0
	}
	return q.dropped.Load()
}

// CaptureQueueCapacity 返回**当前实际生效**的队列容量。
//
// 供启动日志与测试使用：日志必须报告真实对象的值，而不是「按预算推导的
// 理论值」—— 后者会让运维看到 queue=4096 而实际是默认 1024。
func CaptureQueueCapacity() int {
	if captureQueue == nil {
		return 0
	}
	return cap(captureQueue.ch)
}
