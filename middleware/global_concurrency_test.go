package middleware

import (
	"testing"
	"time"

	"github.com/lza6/new-api-Max/setting/relay_setting"
	"github.com/stretchr/testify/assert"
)

// TestGlobalConcurrencyGate 全局并发桶：并发上限 + FIFO 排队 + 超时拒绝。
func TestGlobalConcurrencyGate(t *testing.T) {
	prev := relay_setting.GetRelaySetting()
	relay_setting.GetRelaySetting().GlobalConcurrencyEnabled = true
	relay_setting.GetRelaySetting().GlobalConcurrencyLimit = 2
	relay_setting.GetRelaySetting().GlobalConcurrencyQueue = 2
	relay_setting.GetRelaySetting().GlobalConcurrencyWaitTimeout = 1
	t.Cleanup(func() {
		*relay_setting.GetRelaySetting() = *prev
		globalConcurrency.mu.Lock()
		globalConcurrency.active = 0
		globalConcurrency.waiting = 0
		globalConcurrency.fifo = nil
		globalConcurrency.mu.Unlock()
	})

	g := globalConcurrency
	g.syncLimit()

	// 前 2 个立即获得并发。
	a1, _ := g.acquire(time.Second)
	a2, _ := g.acquire(time.Second)
	assert.True(t, a1 && a2)

	// 第 3、4 个进入排队（不立即放行，也未被拒绝）。
	acquiredCh := make(chan struct {
		acquired     bool
		queueLimited bool
	}, 4)
	go func() {
		x, y := g.acquire(2 * time.Second)
		acquiredCh <- struct {
			acquired     bool
			queueLimited bool
		}{x, y}
	}()
	go func() {
		x, y := g.acquire(2 * time.Second)
		acquiredCh <- struct {
			acquired     bool
			queueLimited bool
		}{x, y}
	}()
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 2, g.waiting, "两个请求应进入排队")

	// 第 5 个：队列满(2) → 立即拒绝。
	a5, ql := g.acquire(time.Second)
	assert.False(t, a5)
	assert.True(t, ql)

	// 释放 1 个 → 唤醒队首 1 个放行（FIFO）。
	g.release()
	first := <-acquiredCh
	assert.True(t, first.acquired)

	// 再释放 → 第二个排队者放行。
	g.release()
	second := <-acquiredCh
	assert.True(t, second.acquired)

	// 全部放行后 active 归零。
	g.release()
	g.release()
	g.mu.Lock()
	assert.Equal(t, 0, g.active)
	assert.Equal(t, 0, g.waiting)
	g.mu.Unlock()
}

// TestGlobalConcurrencyGateTimeout 排队超时：等不到并发则拒绝。
func TestGlobalConcurrencyGateTimeout(t *testing.T) {
	prev := relay_setting.GetRelaySetting()
	relay_setting.GetRelaySetting().GlobalConcurrencyEnabled = true
	relay_setting.GetRelaySetting().GlobalConcurrencyLimit = 1
	relay_setting.GetRelaySetting().GlobalConcurrencyQueue = 2
	relay_setting.GetRelaySetting().GlobalConcurrencyWaitTimeout = 1
	t.Cleanup(func() {
		*relay_setting.GetRelaySetting() = *prev
		globalConcurrency.mu.Lock()
		globalConcurrency.active = 0
		globalConcurrency.waiting = 0
		globalConcurrency.fifo = nil
		globalConcurrency.mu.Unlock()
	})

	g := globalConcurrency
	g.syncLimit()

	a1, _ := g.acquire(time.Second)
	assert.True(t, a1)

	// 排队者：上限 1 且无人释放 → 1 秒后超时拒绝。
	start := time.Now()
	a2, _ := g.acquire(300 * time.Millisecond)
	elapsed := time.Since(start)
	assert.False(t, a2)
	assert.True(t, elapsed >= 250*time.Millisecond, "应等待到超时")
	g.mu.Lock()
	assert.Equal(t, 0, g.waiting)
	g.mu.Unlock()

	g.release()
}

// TestGlobalConcurrencyDisabled 未开启时直接放行。
func TestGlobalConcurrencyDisabled(t *testing.T) {
	prev := relay_setting.GetRelaySetting()
	relay_setting.GetRelaySetting().GlobalConcurrencyEnabled = false
	t.Cleanup(func() {
		*relay_setting.GetRelaySetting() = *prev
	})
	g := globalConcurrency
	g.syncLimit()
	a, _ := g.acquire(time.Second)
	assert.True(t, a, "未开启应直接放行")
	g.release()
}
