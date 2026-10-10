package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// G10 §12.2.1：令牌桶表此前**只写不删** —— token 被删除后条目永远残留。
// 现在按空闲时间淘汰；因为桶空闲足够久后一定已回填满，删掉与新建等价，
// 所以淘汰**不改变限流语义**。本用例同时守住这两点。
func TestTokenQBSBucketsEvictIdleEntries(t *testing.T) {
	tokenQBSBuckets.Lock()
	tokenQBSBuckets.tokens = make(map[int]struct {
		remaining float64
		updated   time.Time
	})
	tokenQBSBuckets.sweepCount = 0
	tokenQBSBuckets.Unlock()
	t.Cleanup(func() {
		tokenQBSBuckets.Lock()
		tokenQBSBuckets.tokens = make(map[int]struct {
			remaining float64
			updated   time.Time
		})
		tokenQBSBuckets.sweepCount = 0
		tokenQBSBuckets.Unlock()
	})

	for id := range 5 {
		require.True(t, allowTokenQBS(id, 10))
	}

	// 把这 5 个桶标记为「空闲过久」
	tokenQBSBuckets.Lock()
	for id, b := range tokenQBSBuckets.tokens {
		b.updated = time.Now().Add(-2 * tokenQBSIdleEvictAfter)
		tokenQBSBuckets.tokens[id] = b
	}
	require.Len(t, tokenQBSBuckets.tokens, 5)
	tokenQBSBuckets.Unlock()

	// 触发一次清扫（每 tokenQBSSweepEvery 次写入扫一次）
	for i := range tokenQBSSweepEvery {
		allowTokenQBS(10000+i, 10)
	}

	tokenQBSBuckets.Lock()
	_, staleStillThere := tokenQBSBuckets.tokens[0]
	tokenQBSBuckets.Unlock()

	assert.False(t, staleStillThere, "空闲过久的桶必须被淘汰（这张表此前只加不减）")
}

// 淘汰不得改变限流语义：桶空闲足够久后一定已回填满，删掉等价于新建。
func TestTokenQBSEvictionDoesNotChangeRateLimitSemantics(t *testing.T) {
	tokenQBSBuckets.Lock()
	tokenQBSBuckets.tokens = make(map[int]struct {
		remaining float64
		updated   time.Time
	})
	tokenQBSBuckets.Unlock()
	t.Cleanup(func() {
		tokenQBSBuckets.Lock()
		tokenQBSBuckets.tokens = make(map[int]struct {
			remaining float64
			updated   time.Time
		})
		tokenQBSBuckets.Unlock()
	})

	const qbs = 3
	// 打空令牌桶
	for range qbs {
		require.True(t, allowTokenQBS(42, qbs))
	}
	assert.False(t, allowTokenQBS(42, qbs), "令牌耗尽后应拒绝")

	// 模拟空闲超过淘汰窗口：淘汰后应重新拿到满桶（与未淘汰时的回填结果一致）
	tokenQBSBuckets.Lock()
	for id, b := range tokenQBSBuckets.tokens {
		b.updated = time.Now().Add(-2 * tokenQBSIdleEvictAfter)
		tokenQBSBuckets.tokens[id] = b
	}
	tokenQBSBuckets.Unlock()

	assert.True(t, allowTokenQBS(42, qbs), "空闲足够久后应允许（桶已回填满）")
}

func TestTokenQBSDisabledWhenQBSNotPositive(t *testing.T) {
	assert.True(t, allowTokenQBS(1, 0), "qbs<=0 表示未配置令牌桶限速 → 一律放行")
	assert.True(t, allowTokenQBS(1, -1))
}
