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

// TestTimedCacheDoesNotCacheErrors 错误**不得**被缓存：
// 云盘短暂故障恢复后，管理员应该立刻看到数据，而不是继续吃 5 分钟的旧错误。
func TestTimedCacheDoesNotCacheErrors(t *testing.T) {
	c := newTimedCache[int](time.Minute)
	var calls atomic.Int32

	// 第一次失败，第二次成功。
	load := func() (int, error) {
		if calls.Add(1) == 1 {
			return 0, errors.New("cloud drive temporarily unavailable")
		}
		return 7, nil
	}

	_, err := c.get(load)
	require.Error(t, err, "第一次应把错误如实返回")

	v, err := c.get(load)
	require.NoError(t, err, "错误不该被缓存，第二次应重新加载并成功")
	assert.Equal(t, 7, v)
	assert.Equal(t, int32(2), calls.Load())
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
