package middleware

import (
	"testing"
	"time"

	"github.com/lza6/new-api-Max/setting/relay_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlobalConcurrencyGate 全局并发桶：并发上限 + FIFO 排队 + 超时拒绝。
func TestGlobalConcurrencyGate(t *testing.T) {
	prev := *relay_setting.GetRelaySetting()
	relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) {
		s.GlobalConcurrencyEnabled = true
		s.GlobalConcurrencyLimit = 2
		s.GlobalConcurrencyQueue = 2
		s.GlobalConcurrencyWaitTimeout = 1
	})
	t.Cleanup(func() {
		relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) { *s = prev })
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
	g.mu.Lock()
	waitingCount := g.waiting
	g.mu.Unlock()
	assert.Equal(t, 2, waitingCount, "两个请求应进入排队")

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
	prev := *relay_setting.GetRelaySetting()
	relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) {
		s.GlobalConcurrencyEnabled = true
		s.GlobalConcurrencyLimit = 1
		s.GlobalConcurrencyQueue = 2
		s.GlobalConcurrencyWaitTimeout = 1
	})
	t.Cleanup(func() {
		relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) { *s = prev })
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
	prev := *relay_setting.GetRelaySetting()
	relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) {
		s.GlobalConcurrencyEnabled = false
	})
	t.Cleanup(func() {
		relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) { *s = prev })
	})
	g := globalConcurrency
	g.syncLimit()
	a, _ := g.acquire(time.Second)
	assert.True(t, a, "未开启应直接放行")
	g.release()
}

// resetGateForTest 重置并发桶到干净状态（测试间互不污染）。
func resetGateForTest(t *testing.T) {
	t.Helper()
	prev := *relay_setting.GetRelaySetting()
	relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) {
		s.GlobalConcurrencyEnabled = true
		s.GlobalConcurrencyLimit = 1
		s.GlobalConcurrencyQueue = 4096
		s.GlobalConcurrencyWaitTimeout = 1
	})
	globalConcurrency.mu.Lock()
	globalConcurrency.active = 0
	globalConcurrency.waiting = 0
	globalConcurrency.fifo = nil
	globalConcurrency.mu.Unlock()
	globalConcurrency.syncLimit()
	t.Cleanup(func() {
		relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) { *s = prev })
		globalConcurrency.mu.Lock()
		globalConcurrency.active = 0
		globalConcurrency.waiting = 0
		globalConcurrency.fifo = nil
		globalConcurrency.mu.Unlock()
	})
}

// TestGlobalConcurrencyReleaseRacesTimeout 4.2.1 核心回归：release 与「队首等待者
// 超时离场」**同时发生**时，槽位不得泄漏。
//
// 精确竞态窗口（limit=1，A 占槽，W 入队后 timer 到点）：
//   - 正常：W 先在锁内把自己移出 fifo → release 看到空队首 → active=0。
//   - 正常：release 先出队 W 并 close(ch_W) → W 从 <-ch_W 醒来拿到槽位 → W 持有。
//   - 泄漏：release 先出队 W、active++、close(ch_W)，但 W 的 timer.C 已先到点、
//     正等锁；release 解锁后 W 在 fifo 找不到自己 → 返回 false 而**不再持有槽位**，
//     可 active 已被 release 加到 1 → 幽灵占用、永无人 release。
//
// 判定：等待者若真的拿到槽位（返回 true）则必须释放；一轮结束后 active 必须为 0。
func TestGlobalConcurrencyReleaseRacesTimeout(t *testing.T) {
	resetGateForTest(t)
	g := globalConcurrency

	const rounds = 4000
	leaked := 0
	for round := range rounds {
		held, _ := g.acquire(time.Second)
		require.True(t, held, "round %d: 应能占满槽位", round)

		waiterDone := make(chan struct{}, 1)
		go func() {
			got, _ := g.acquire(1 * time.Millisecond)
			if got {
				// 真的拿到槽位（release 正常交接）→ 用完释放，避免把「合法持有」误判成泄漏。
				g.release()
			}
			waiterDone <- struct{}{}
		}()

		// 在等待者超时临界点附近 release，逼出「release 先手、等待者 timer 先响」的交错。
		time.Sleep(time.Duration(round%4) * 150 * time.Microsecond)
		g.release()
		<-waiterDone

		g.mu.Lock()
		active := g.active
		waiting := g.waiting
		g.mu.Unlock()
		if active != 0 || waiting != 0 {
			leaked++
			// 隔离每轮：复位，避免前一泄漏持续放大后续判定。
			g.mu.Lock()
			g.active = 0
			g.waiting = 0
			g.fifo = nil
			g.mu.Unlock()
		}
	}

	assert.Zero(t, leaked, "release × 队首超时（真并发 %d 轮）不得出现任何槽位泄漏", rounds)

	ok := make(chan bool, 1)
	go func() {
		got, _ := g.acquire(time.Second)
		ok <- got
	}()
	select {
	case got := <-ok:
		assert.True(t, got, "泄漏后桶必须仍可接纳新请求")
	case <-time.After(2 * time.Second):
		t.Fatal("桶自我堵死：新请求被永久阻塞")
	}
	g.release()
	g.mu.Lock()
	assert.Zero(t, g.active)
	g.mu.Unlock()
}

// TestGlobalConcurrencyNoStarveAfterChurn 累积饥饿演示：反复 release × 超时后，
// 桶仍应能正常接纳新请求（不得永久堵死）。
func TestGlobalConcurrencyNoStarveAfterChurn(t *testing.T) {
	resetGateForTest(t)
	g := globalConcurrency
	// limit=1；先制造 200 轮交叉噪声。
	for range 200 {
		held, _ := g.acquire(time.Second)
		require.True(t, held)
		done := make(chan struct{})
		go func() {
			_, _ = g.acquire(5 * time.Millisecond)
			close(done)
		}()
		time.Sleep(time.Millisecond)
		<-done
		g.release()
	}

	// 噪声后必须仍能接纳新请求并正确释放。
	code := make(chan bool, 1)
	go func() {
		got, _ := g.acquire(time.Second)
		code <- got
	}()
	select {
	case got := <-code:
		assert.True(t, got, "交叉噪声后必须仍能接纳新请求（桶不得自我堵死）")
	case <-time.After(2 * time.Second):
		t.Fatal("交叉噪声后新请求被永久阻塞（桶已堵死）")
	}
	g.release()
	g.mu.Lock()
	assert.Zero(t, g.active)
	g.mu.Unlock()
}
