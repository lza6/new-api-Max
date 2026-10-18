package controller

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 管理端云盘查询的缓存与请求合并。
//
// 这组性质直接对应一个**生产风险**：管理端页面轮询会持续调用云盘 API，
// 而采集 worker 共用同一份速率配额 —— 管理端把配额打满会让采集上传 429，
// 表现为「采集莫名开始失败」，根因却在另一个页面。

func TestTimedCacheServesWithinTTL(t *testing.T) {
	c := newTimedCache[int](time.Minute)
	var calls atomic.Int32

	load := func() (int, error) {
		calls.Add(1)
		return 42, nil
	}

	for range 5 {
		v, err := c.get(load)
		require.NoError(t, err)
		assert.Equal(t, 42, v)
	}
	assert.Equal(t, int32(1), calls.Load(), "TTL 内只应真正加载一次")
}

func TestTimedCacheReloadsAfterTTL(t *testing.T) {
	c := newTimedCache[int](10 * time.Millisecond)
	var calls atomic.Int32
	load := func() (int, error) {
		n := calls.Add(1)
		return int(n), nil
	}

	first, err := c.get(load)
	require.NoError(t, err)
	assert.Equal(t, 1, first)

	time.Sleep(20 * time.Millisecond)

	second, err := c.get(load)
	require.NoError(t, err)
	assert.Equal(t, 2, second, "TTL 过期后必须重新加载")
	assert.Equal(t, int32(2), calls.Load())
}

// TestTimedCacheNegativeCachesErrorsBriefly 失败结果只**短暂**负缓存。
//
// 完全不缓存 → 云盘持续故障（尤其 429 限流）时，每个轮询都重新打一次，
// 等于持续加压，让限流更难解除；而采集 worker 共用同一份配额，会被一起拖住。
// 永久缓存 → 云盘恢复后管理员继续看到旧错误。
// 折中：负缓存 negativeTTL，到点自动重试。
//
// ## 为什么用「手动推 failedAt」而不是 sleep
//
// 最初的写法是「20ms 窗口 + sleep 30ms」。它**不稳定**：进入负缓存时会记一条
// `common.SysLog`，而本机首次 SysLog 经 cgo 写控制台约 60~120ms ——
// 直接吃掉了 20ms 窗口，第二次 get 时窗口已过期 → 断言失败。
// 也就是说测试在验证「时钟」而不是「逻辑」。
// 改为把时间戳直接推到过去，让用例**只依赖逻辑**、不依赖真实耗时。
func TestTimedCacheNegativeCachesErrorsBriefly(t *testing.T) {
	c := newTimedCache[int](time.Minute)
	c.negativeTTL = time.Hour // 足够大，确保不会被任何真实耗时穿透
	var calls atomic.Int32
	shouldFail := true
	load := func() (int, error) {
		calls.Add(1)
		if shouldFail {
			return 0, errors.New("cloud drive temporarily unavailable")
		}
		return 7, nil
	}

	_, err := c.get(load)
	require.Error(t, err, "第一次应把错误如实返回")
	require.Equal(t, int32(1), calls.Load())

	// 负缓存窗口内：直接返回同一个错误，**不再打云盘**。
	_, err = c.get(load)
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "负缓存窗口内不得重试（防重试风暴）")

	// 把失败时间推到窗口之外 —— 等价于「等待到期」，但不依赖真实时间。
	c.mu.Lock()
	c.failedAt = time.Now().Add(-2 * time.Hour)
	c.mu.Unlock()

	// 到期后自动重试；此时云盘已恢复 → 必须拿到数据。
	shouldFail = false
	v, err := c.get(load)
	require.NoError(t, err, "负缓存过期后必须重试并恢复")
	assert.Equal(t, 7, v)
	assert.Equal(t, int32(2), calls.Load())
}

// TestTimedCacheSuccessClearsNegativeCache 成功加载后必须清掉负缓存记录。
//
// 不清的后果：之后某次独立失败时，会命中**更早那次**的旧错误，
// 管理员看到的时间点与内容都对不上，排障被误导。
func TestTimedCacheSuccessClearsNegativeCache(t *testing.T) {
	// 成功值 ttl 很短，负缓存很长 —— 让我们能在负缓存仍有效时走到成功路径。
	c := newTimedCache[int](time.Millisecond)
	c.negativeTTL = time.Hour

	firstErr := errors.New("first failure")
	_, err0 := c.get(func() (int, error) { return 0, firstErr })
	require.Error(t, err0)

	// 负缓存很长，此时应直接返回 firstErr（且不再 load）。
	var calls atomic.Int32
	_, err := c.get(func() (int, error) { calls.Add(1); return 1, nil })
	require.Error(t, err)
	assert.Equal(t, int32(0), calls.Load(), "负缓存窗口内不得 load")

	// 把负缓存窗口缩短到已过期，再让加载成功 → 成功应清掉 lastErr。
	c.mu.Lock()
	c.failedAt = time.Now().Add(-2 * time.Hour)
	c.mu.Unlock()
	v, err := c.get(func() (int, error) { return 1, nil })
	require.NoError(t, err)
	assert.Equal(t, 1, v)

	// 成功之后 ttl 到期再失败：必须是**新的**错误，而不是更早的 firstErr。
	time.Sleep(5 * time.Millisecond)
	secondErr := errors.New("second failure")
	_, err = c.get(func() (int, error) { return 0, secondErr })
	require.Error(t, err)
	assert.EqualError(t, err, secondErr.Error(),
		"成功之后应记录新错误，不能沿用更早的负缓存")
}

// TestTimedCacheCollapsesConcurrentMisses 并发未命中必须合并成**一次**加载。
//
// 这是「多标签页 / 多管理员同时刷新」的真实场景：不合并会同时发起多份
// 全量云盘列举，正是打满配额的直接原因。
func TestTimedCacheCollapsesConcurrentMisses(t *testing.T) {
	c := newTimedCache[int](time.Minute)
	var calls atomic.Int32

	// 加载稍慢，制造「多个请求同时未命中」的窗口。
	load := func() (int, error) {
		calls.Add(1)
		time.Sleep(30 * time.Millisecond)
		return 99, nil
	}

	const concurrency = 16
	var wg sync.WaitGroup
	results := make([]int, concurrency)
	wg.Add(concurrency)
	for i := range concurrency {
		go func() {
			defer wg.Done()
			v, err := c.get(load)
			if err == nil {
				results[i] = v
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), calls.Load(),
		"并发未命中必须合并为一次云盘调用，实际 %d 次", calls.Load())
	for i, v := range results {
		assert.Equalf(t, 99, v, "第 %d 个并发调用拿到了错误结果", i)
	}
}

// TestTimedCacheKeepsStaleValueOutOfErrors 加载失败时**不清空**已有缓存值？
// —— 当前实现选择不返回旧值（避免把过期数据当新数据）。
// 这条用例锁定该语义：先成功填充，TTL 过期后加载失败 → 返回错误而不是旧值。
func TestTimedCacheReturnsErrorNotStaleValueAfterExpiry(t *testing.T) {
	c := newTimedCache[int](10 * time.Millisecond)
	fail := false

	load := func() (int, error) {
		if fail {
			return 0, errors.New("boom")
		}
		return 5, nil
	}

	v, err := c.get(load)
	require.NoError(t, err)
	require.Equal(t, 5, v)

	time.Sleep(20 * time.Millisecond)
	fail = true
	if _, err := c.get(load); err == nil {
		t.Fatal("过期后加载失败必须如实报错，不能把过期值当新值返回")
	}
}
