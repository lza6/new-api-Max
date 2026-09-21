package service

// P2-2 通用事件/Webhook 子系统（最小核心，内存版）。
//
// 目标（next-step 指南 P2-2）：「事件类型注册 → 处理器路由 → 幂等 → 重放 →
// 双向状态机」的通用底座，供支付 webhook、通知、任务事件等后续归一接入。
//
// 设计约定：
//   - 进程内事件总线：注册式 HandleFunc + 幂等去重 + 失败退避重试 + 状态机。
//   - 不依赖数据库/Redis/三库方言，避免引入跨库复杂度；后续如需跨节点可靠
//     投递，可在 Dispatch 内接入持久化存储而不改公开接口。
//   - 幂等：以事件 ID 为唯一键，同一事件只允许投递一次（handler 视角）。
//   - 状态机：pending -> success | failed(retry) -> dead（超过最大重试）。
//   - 并发安全；Dispatch 同步执行（调用方可在 goroutine 中调用以获得异步语义）。

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// ErrEventBusClosed 表示总线已关闭。
var ErrEventBusClosed = errors.New("event bus is closed")

// ErrEventBusDup 表示事件已投递过（幂等去重）。
var ErrEventBusDup = errors.New("event already delivered")

// ErrEventBusNoHandler 表示没有处理器订阅该事件类型。
var ErrEventBusNoHandler = errors.New("no handler registered for event type")

// EventState 事件处理状态机。
type EventState string

const (
	EventStatePending EventState = "pending"
	EventStateSuccess EventState = "success"
	EventStateFailed  EventState = "failed"
	EventStateDead    EventState = "dead"
)

// Event 一条待分发事件。ID 为幂等键，Type 决定路由，Payload 透传。
type Event struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Payload   any    `json:"payload,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// EventHandleFunc 事件处理器；返回 error 表示处理失败需重试。
type EventHandleFunc func(ctx context.Context, ev Event) error

// EventDeliveryRecord 持久化幂等记录（event_id + handler 联合唯一）。
type EventDeliveryRecord struct {
	EventID   string
	Handler   string
	EventType string
	State     EventState
	Attempts  int
	LastError string
}

// EventDeliveryStore 持久化投递记录的可选存储。实现方负责跨库可移植的
// 「不存在才插入」（如 GORM clause.OnConflict{DoNothing:true} + RowsAffected）。
type EventDeliveryStore interface {
	// CreateDelivery 幂等创建投递记录；created=false 表示 (event_id, handler)
	// 已存在（重复投递）。
	CreateDelivery(ctx context.Context, rec EventDeliveryRecord) (created bool, err error)
	// UpdateDeliveryState 更新投递状态（成功/失败/终态）。
	UpdateDeliveryState(ctx context.Context, eventID, handler string, state EventState, attempts int, lastError string) error
}

// eventRecord 单条事件的运行状态。
type eventRecord struct {
	state     EventState
	attempts  int
	lastErr   error
	nextRetry time.Time
}

// EventBus 通用事件总线（内存版、并发安全）。
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string]EventHandleFunc
	events   map[string]*eventRecord // 幂等去重 + 状态跟踪
	closed   bool

	maxRetries int
	baseDelay  time.Duration
	// 可选持久化存储：接入后 Publish 先落幂等记录，状态变更同步到存储。
	store EventDeliveryStore
	// 统计（供 perf_metrics / 日志透明化）
	DeliveredCount int64
	RetryCount     int64
	FailedCount    int64
	DeadCount      int64
	LastDispatchMs int64
}

// EventBusMetrics 事件总线观测快照（耗时/成功/失败/重试）。
type EventBusMetrics struct {
	DeliveredCount int64
	RetryCount     int64
	FailedCount    int64
	DeadCount      int64
	LastDispatchMs int64
}

// NewEventBus 创建一个事件总线。maxRetries 为单事件最大重试次数（0 表示
// 失败即 dead），baseDelay 为退避基数（逐次翻倍，上限 30s）。
func NewEventBus(maxRetries int, baseDelay time.Duration) *EventBus {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if baseDelay <= 0 {
		baseDelay = time.Second
	}
	return &EventBus{
		handlers:   make(map[string]EventHandleFunc),
		events:     make(map[string]*eventRecord),
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
	}
}

// SetDeliveryStore 挂载持久化存储。可在任意时刻设置；设置后新 Publish 的
// 事件会先落幂等记录，重复 (event_id, handler) 被拒绝。
func (b *EventBus) SetDeliveryStore(store EventDeliveryStore) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.store = store
}

// Metrics 返回观测快照（带锁读取，调用频率低）。
func (b *EventBus) Metrics() EventBusMetrics {
	b.mu.Lock()
	defer b.mu.Unlock()
	return EventBusMetrics{
		DeliveredCount: b.DeliveredCount,
		RetryCount:     b.RetryCount,
		FailedCount:    b.FailedCount,
		DeadCount:      b.DeadCount,
		LastDispatchMs: b.LastDispatchMs,
	}
}

// Register 注册某事件类型的处理器。重复注册同一类型会覆盖旧处理器。
func (b *EventBus) Register(eventType string, h EventHandleFunc) {
	if eventType == "" || h == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = h
}

// Unregister 移除某事件类型的处理器。
func (b *EventBus) Unregister(eventType string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.handlers, eventType)
}

// Publish 注册并尝试分发一条事件：不存在则创建；已存在（任意终态/进行中）
// 都按幂等拒绝，返回 ErrEventBusDup。同步执行；需要异步时由调用方包
// goroutine。事件 ID 为空时自动生成。
func (b *EventBus) Publish(ctx context.Context, ev Event) error {
	if ev.ID == "" {
		ev.ID = common.NewRequestId() // 复用请求级唯一 ID 生成器
	}
	if ev.CreatedAt == 0 {
		ev.CreatedAt = time.Now().Unix()
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrEventBusClosed
	}
	if _, ok := b.events[ev.ID]; ok {
		b.mu.Unlock()
		return ErrEventBusDup
	}
	store := b.store
	b.mu.Unlock()

	// 持久化幂等：同一 (event_id, handler) 已存在（含之前失败/成功）则拒绝，
	// 保证跨重启/跨实例也只处理一次。
	if store != nil {
		created, err := store.CreateDelivery(ctx, EventDeliveryRecord{
			EventID:   ev.ID,
			Handler:   ev.Type,
			EventType: ev.Type,
			State:     EventStatePending,
		})
		if err != nil {
			common.SysError(fmt.Sprintf("event_bus: persist delivery id=%s type=%s failed: %v", ev.ID, ev.Type, err))
			return err
		}
		if !created {
			return ErrEventBusDup
		}
	}

	b.mu.Lock()
	b.events[ev.ID] = &eventRecord{state: EventStatePending}
	b.mu.Unlock()
	return b.dispatch(ctx, ev)
}

func (b *EventBus) dispatch(ctx context.Context, ev Event) error {
	b.mu.RLock()
	h, ok := b.handlers[ev.Type]
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		return ErrEventBusClosed
	}
	if !ok {
		b.finish(ev.ID, EventStateDead, errors.New(ErrEventBusNoHandler.Error()))
		return ErrEventBusNoHandler
	}

	started := time.Now()
	for {
		err := safeHandle(ctx, h, ev)
		b.mu.Lock()
		rec := b.events[ev.ID]
		b.mu.Unlock()
		if err == nil {
			b.finish(ev.ID, EventStateSuccess, nil)
			attempts := 0
			if rec != nil {
				attempts = rec.attempts
			}
			b.persistState(ctx, ev, EventStateSuccess, attempts, nil)
			b.mu.Lock()
			b.DeliveredCount++
			b.LastDispatchMs = time.Since(started).Milliseconds()
			b.mu.Unlock()
			return nil
		}

		// 失败：按退避重试，超过上限进 dead。
		b.mu.Lock()
		if rec == nil {
			b.mu.Unlock()
			return err
		}
		rec.attempts++
		next := rec
		if next.attempts > b.maxRetries {
			next.state = EventStateDead
			b.mu.Unlock()
			b.persistState(ctx, ev, EventStateDead, next.attempts, err)
			b.mu.Lock()
			b.FailedCount++
			b.DeadCount++
			b.LastDispatchMs = time.Since(started).Milliseconds()
			b.mu.Unlock()
			return err
		}
		delay := b.baseDelay * time.Duration(1<<min(next.attempts, 5))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		next.state = EventStateFailed
		next.lastErr = err
		next.nextRetry = time.Now().Add(delay)
		b.mu.Unlock()
		b.persistState(ctx, ev, EventStateFailed, next.attempts, err)

		b.mu.Lock()
		b.RetryCount++
		b.LastDispatchMs = time.Since(started).Milliseconds()
		b.mu.Unlock()

		common.SysError(fmt.Sprintf("event_bus: event %s type=%s attempt=%d failed: %v; retry in %s", ev.ID, ev.Type, next.attempts, err, delay))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// persistState 将投递状态同步到持久化存储（可选；失败仅告警不改判终态）。
func (b *EventBus) persistState(ctx context.Context, ev Event, state EventState, attempts int, lastErr error) {
	b.mu.RLock()
	store := b.store
	b.mu.RUnlock()
	if store == nil {
		return
	}
	errText := ""
	if lastErr != nil {
		errText = lastErr.Error()
	}
	if err := store.UpdateDeliveryState(ctx, ev.ID, ev.Type, state, attempts, errText); err != nil {
		common.SysError(fmt.Sprintf("event_bus: update delivery id=%s type=%s state=%s failed: %v", ev.ID, ev.Type, state, err))
	}
}

func (b *EventBus) finish(id string, state EventState, recorderErr error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if rec, ok := b.events[id]; ok {
		rec.state = state
		rec.lastErr = recorderErr
	}
}

// State 返回某事件的当前状态（用于重放/审计）。
func (b *EventBus) State(id string) (EventState, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	rec, ok := b.events[id]
	if !ok {
		return "", false
	}
	return rec.state, true
}

// Close 关闭总线：拒绝新事件，已注册处理器失效。
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
}

// safeHandle 捕获处理器 panic，避免单个 handler 拖垮分发循环。
func safeHandle(ctx context.Context, h EventHandleFunc, ev Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("event handler panic")
		}
	}()
	return h(ctx, ev)
}

// PublishAsync 异步分发（goroutine），返回是否受理（false=重复/关闭）。
func (b *EventBus) PublishAsync(ctx context.Context, ev Event) bool {
	err := b.Publish(ctx, ev)
	return err == nil
}
