package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBusDeliversAndIdempotent(t *testing.T) {
	bus := NewEventBus(0, time.Millisecond)
	var calls atomic.Int64
	bus.Register("user.created", func(_ context.Context, _ Event) error {
		calls.Add(1)
		return nil
	})

	err := bus.Publish(context.Background(), Event{ID: "evt-1", Type: "user.created", Payload: map[string]any{"id": 1}})
	require.NoError(t, err)
	require.Equal(t, int64(1), calls.Load())

	// 同一事件重复投递必须幂等拒绝，不得二次执行。
	err = bus.Publish(context.Background(), Event{ID: "evt-1", Type: "user.created"})
	require.ErrorIs(t, err, ErrEventBusDup)
	require.Equal(t, int64(1), calls.Load())

	state, ok := bus.State("evt-1")
	require.True(t, ok)
	assert.Equal(t, EventStateSuccess, state)
}

func TestEventBusRetriesThenDead(t *testing.T) {
	bus := NewEventBus(2, time.Millisecond)
	var calls atomic.Int64
	bus.Register("payment.webhook", func(_ context.Context, _ Event) error {
		calls.Add(1)
		return errors.New("upstream 5xx")
	})

	err := bus.Publish(context.Background(), Event{ID: "evt-pay", Type: "payment.webhook"})
	require.Error(t, err) // 超过最大重试后返回最后一次错误
	require.Equal(t, int64(3), calls.Load(), "1 次初始 + 2 次重试")

	state, ok := bus.State("evt-pay")
	require.True(t, ok)
	assert.Equal(t, EventStateDead, state)
	require.GreaterOrEqual(t, bus.DeadCount, int64(1))
}

func TestEventBusRecoversAfterTransientFailure(t *testing.T) {
	bus := NewEventBus(3, time.Millisecond)
	var calls atomic.Int64
	bus.Register("task.settled", func(_ context.Context, _ Event) error {
		if calls.Add(1) == 1 {
			return errors.New("transient")
		}
		return nil
	})

	require.NoError(t, bus.Publish(context.Background(), Event{ID: "evt-task", Type: "task.settled"}))
	require.Equal(t, int64(2), calls.Load())
	state, ok := bus.State("evt-task")
	require.True(t, ok)
	assert.Equal(t, EventStateSuccess, state)
}

func TestEventBusNoHandlerAndPanicRecovered(t *testing.T) {
	bus := NewEventBus(0, time.Millisecond)

	err := bus.Publish(context.Background(), Event{ID: "evt-noh", Type: "unknown.type"})
	require.ErrorIs(t, err, ErrEventBusNoHandler)

	bus.Register("panic.handler", func(_ context.Context, _ Event) error {
		panic("boom")
	})
	err = bus.Publish(context.Background(), Event{ID: "evt-panic", Type: "panic.handler"})
	require.Error(t, err) // panic 被 safeHandle 转成 error，不崩溃
	state, ok := bus.State("evt-panic")
	require.True(t, ok)
	assert.Equal(t, EventStateDead, state)
}

func TestEventBusClosedRejectsNewEvents(t *testing.T) {
	bus := NewEventBus(0, time.Millisecond)
	bus.Register("x", func(_ context.Context, _ Event) error { return nil })
	bus.Close()

	err := bus.Publish(context.Background(), Event{ID: "evt-closed", Type: "x"})
	require.ErrorIs(t, err, ErrEventBusClosed)
}

func TestEventBusAsyncAndAutoID(t *testing.T) {
	bus := NewEventBus(0, time.Millisecond)
	var calls atomic.Int64
	bus.Register("async.type", func(_ context.Context, _ Event) error {
		calls.Add(1)
		return nil
	})

	// 无 ID → 自动生成成功投递
	ok := bus.PublishAsync(context.Background(), Event{Type: "async.type"})
	require.True(t, ok)
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, time.Millisecond)

	// 显式固定 ID：首次受理，再次投递幂等拒绝
	require.True(t, bus.PublishAsync(context.Background(), Event{ID: "async-same", Type: "async.type"}), "首次受理")
	require.False(t, bus.PublishAsync(context.Background(), Event{ID: "async-same", Type: "async.type"}), "重复事件异步也应拒绝")
	require.Eventually(t, func() bool { return calls.Load() == 2 }, time.Second, time.Millisecond)
}
