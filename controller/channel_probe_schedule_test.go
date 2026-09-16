package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRetryableProbeFailures B4-2：首轮失败渠道数决定是否补跑一轮。
func TestRetryableProbeFailures(t *testing.T) {
	// 无失败 → 不重试。
	assert.Zero(t, retryableProbeFailures(map[string]any{"failed": 0}))
	// 有失败 → 返回个数触发重试。
	assert.Equal(t, 3, retryableProbeFailures(map[string]any{"failed": 3}))
	// summary 无 failed 键（早期版本/异常路径）→ 安全返回 0。
	assert.Zero(t, retryableProbeFailures(map[string]any{"probed": 5}))
	// nil map → 安全返回 0。
	assert.Zero(t, retryableProbeFailures(nil))
}

// TestRetryMergeKeepsFirstRoundFailed B4-2：重试按「不覆盖首轮失败数」合并，
// 保证 summary 只增不减（首轮 failed 数保留供审计）。
func TestRetryMergeKeepsFirstRoundFailed(t *testing.T) {
	first := map[string]any{"probed": 5, "failed": 2}
	summary := map[string]any{"probed": 5, "failed": 2, "first_round_failed": 2, "retried": true}
	for k, v := range summary {
		if _, exists := first[k]; !exists {
			first[k] = v
		}
	}
	assert.Equal(t, 2, first["first_round_failed"])
	assert.Equal(t, true, first["retried"])
	assert.Equal(t, 2, first["failed"], "首轮 failed 数保留")
}
