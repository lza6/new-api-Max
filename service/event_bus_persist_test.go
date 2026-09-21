package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDeliveryStore 内存版投递记录存储：验证总线「持久化幂等 + 状态同步」。
type fakeDeliveryStore struct {
	mu      sync.Mutex
	records map[string]EventDeliveryRecord
}

func newFakeDeliveryStore() *fakeDeliveryStore {
	return &fakeDeliveryStore{records: make(map[string]EventDeliveryRecord)}
}

func (f *fakeDeliveryStore) CreateDelivery(_ context.Context, rec EventDeliveryRecord) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := rec.EventID + "|" + rec.Handler
	if _, ok := f.records[key]; ok {
		return false, nil
	}
	f.records[key] = rec
	return true, nil
}

func (f *fakeDeliveryStore) UpdateDeliveryState(_ context.Context, eventID, handler string, state EventState, attempts int, lastError string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := eventID + "|" + handler
	rec, ok := f.records[key]
	if !ok {
		return errors.New("delivery record not found")
	}
	rec.State = state
	rec.Attempts = attempts
	rec.LastError = lastError
	f.records[key] = rec
	return nil
}

func (f *fakeDeliveryStore) state(eventID, handler string) (EventDeliveryRecord, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[eventID+"|"+handler]
	return rec, ok
}

func TestEventBusPersistDedupAcrossPublish(t *testing.T) {
	bus := NewEventBus(0, time.Millisecond)
	store := newFakeDeliveryStore()
	bus.SetDeliveryStore(store)
	var calls atomic.Int64
	bus.Register("pay.webhook", func(_ context.Context, _ Event) error {
		calls.Add(1)
		return nil
	})

	require.NoError(t, bus.Publish(context.Background(), Event{ID: "evt-persist-1", Type: "pay.webhook"}))
	require.Equal(t, int64(1), calls.Load())

	rec, ok := store.state("evt-persist-1", "pay.webhook")
	require.True(t, ok)
	assert.Equal(t, EventStateSuccess, rec.State)

	// 第二次 Publish：存储层幂等拒绝，不重复执行处理器。
	err := bus.Publish(context.Background(), Event{ID: "evt-persist-1", Type: "pay.webhook"})
	require.ErrorIs(t, err, ErrEventBusDup)
	require.Equal(t, int64(1), calls.Load())
}

func TestEventBusPersistFailedAndDeadState(t *testing.T) {
	bus := NewEventBus(1, time.Millisecond)
	store := newFakeDeliveryStore()
	bus.SetDeliveryStore(store)
	var calls atomic.Int64
	bus.Register("pay.webhook", func(_ context.Context, _ Event) error {
		calls.Add(1)
		return errors.New("upstream 5xx")
	})

	err := bus.Publish(context.Background(), Event{ID: "evt-persist-fail", Type: "pay.webhook"})
	require.Error(t, err)

	rec, ok := store.state("evt-persist-fail", "pay.webhook")
	require.True(t, ok)
	assert.Equal(t, EventStateDead, rec.State)
	require.GreaterOrEqual(t, rec.Attempts, 1)

	metrics := bus.Metrics()
	assert.GreaterOrEqual(t, metrics.FailedCount, int64(1))
	assert.GreaterOrEqual(t, metrics.DeadCount, int64(1))
	assert.GreaterOrEqual(t, metrics.RetryCount, int64(1))
	assert.GreaterOrEqual(t, metrics.LastDispatchMs, int64(0))
}

func TestEventBusPersistMetricsWithoutStore(t *testing.T) {
	bus := NewEventBus(0, time.Millisecond)
	var calls atomic.Int64
	bus.Register("plain.event", func(_ context.Context, _ Event) error {
		calls.Add(1)
		return nil
	})
	require.NoError(t, bus.Publish(context.Background(), Event{ID: "evt-no-store", Type: "plain.event"}))
	require.Equal(t, int64(1), calls.Load())
	metrics := bus.Metrics()
	assert.Equal(t, int64(1), metrics.DeliveredCount)
	assert.GreaterOrEqual(t, metrics.LastDispatchMs, int64(0))
}
