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
	// CreatedAt 请求完成时间（unix 秒），用于目录日期归属。
	CreatedAt int64
}

// CaptureQueue 有界、非阻塞投递的采集队列。
type CaptureQueue struct {
	ch chan CaptureSignal

	// enqueued/dropped 用原子计数，读侧（指标）不加锁。
	enqueued atomic.Int64
	dropped  atomic.Int64

	// closed 标记队列已关闭，之后投递直接丢弃（避免 close 后 send panic）。
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
	return &CaptureQueue{ch: make(chan CaptureSignal, capacity)}
}

// TryEnqueue **非阻塞**投递。返回 false 表示队列已满（已计入丢弃）或已关闭。
//
// **这是请求路径上唯一被调用的方法**，因此它必须极快：
// 只有一次 `select` + 一次 channel send（或 default 分支）。
// 没有锁、没有分配、没有日志。
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
func (q *CaptureQueue) Receive() (CaptureSignal, bool) {
	if q == nil {
		return CaptureSignal{}, false
	}
	signal, ok := <-q.ch
	return signal, ok
}

// Close 关闭队列，使 worker 的 Receive 返回 ok=false 以退出。
// 幂等；关闭后 TryEnqueue 一律返回 false（不 panic）。
func (q *CaptureQueue) Close() {
	if q == nil {
		return
	}
	q.closeOnce.Do(func() {
		q.closed.Store(true)
		close(q.ch)
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
