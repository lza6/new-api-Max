package oaichat

// 复现用例：缺陷 B —— Claude 流式 message_delta 的 cache creation 5m/1h 归一化。
//
// 背景（上游 issue #7777）：Claude 流式的 message_delta 事件只给出
// cache_creation_input_tokens 的总量（没有 5m/1h 拆分）。归一化时，
// 我们的实现把「总量 - 已知5m - 已知1h」的剩余全部塞进 ephemeral_5m。
//
// 当前实现（internal/shared/claude/cache.go:3-9）：
//   remainder := totalTokens - tokens5m - tokens1h
//   return tokens5m + remainder, tokens1h   // ← 全部剩余归 5m
//
// 期望行为（上游已修）：当 5m/1h 都没有拆分信息（都为 0）而只有总量时，
// 不应武断地把全部归入 5m（5m 与 1h 的单价不同，5m 便宜、1h 贵 2 倍，
// 全部归 5m 会**少收** 1h 档的费用）。上游的修法是保留「未拆分」语义，
// 让计费端对未拆分部分使用缓存写入基准价，而不是强行按 5m 计价。
//
// 本文件只断言当前行为，供主线程做 RED→GREEN，不修改产品代码。

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRepro_NormalizeCacheCreationSplit_TotalOnlyFallsInto5m 锁定「只给总量」时的
// 当前归一化结果：全部剩余归入 5m，1h=0。
func TestRepro_NormalizeCacheCreationSplit_TotalOnlyFallsInto5m(t *testing.T) {
	// 上游 message_delta 只报总量 2000，没有 5m/1h 拆分。
	cache5m, cache1h := NormalizeCacheCreationSplit(2000, 0, 0)

	// 期望行为（上游已修）：总量在无拆分信息时不应被伪装成「全部 5m」。
	// 当前行为（疑似缺陷）：3000 全部计入 5m，1h=0。
	assert.Equal(t, 2000, cache5m,
		"CURRENT BEHAVIOR: the whole un-split cache creation total is attributed to ephemeral_5m")
	assert.Equal(t, 0, cache1h,
		"CURRENT BEHAVIOR: ephemeral_1h stays 0 even though upstream never said so")
}

// TestRepro_NormalizeCacheCreationSplit_RemainderGoesTo5m 锁定「总量 > 5m+1h」时
// 剩余部分的归属。
func TestRepro_NormalizeCacheCreationSplit_RemainderGoesTo5m(t *testing.T) {
	// 4m 拆分给出 5m=100、1h=50，总量 1000；剩余 850。
	cache5m, cache1h := NormalizeCacheCreationSplit(1000, 100, 50)

	// 期望行为（上游已修）：剩余（未标明档位）应以「未拆分」语义计价。
	// 当前行为（疑似缺陷）：剩余 850 全部并入 5m → 950 / 50。
	assert.Equal(t, 950, cache5m,
		"CURRENT BEHAVIOR: remainder is folded into ephemeral_5m")
	assert.Equal(t, 50, cache1h,
		"CURRENT BEHAVIOR: ephemeral_1h keeps only the explicitly-reported part")
}

// TestRepro_NormalizeCacheCreationSplit_OverReportedSplit 锁定「5m+1h > 总量」时
// 剩余被 clamp 到 0 的行为。
func TestRepro_NormalizeCacheCreationSplit_OverReportedSplit(t *testing.T) {
	cache5m, cache1h := NormalizeCacheCreationSplit(3, 5, 1)

	// 期望行为（上游已修）：拆分已超总量时不应再放大。
	// 当前行为：remainder<0 → 0，返回 5 / 1。这一条与上游一致。
	assert.Equal(t, 5, cache5m)
	assert.Equal(t, 1, cache1h)
}
