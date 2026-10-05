package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §压缩统计：热路径缓冲累加 + flush 落库 + 重启后从 DB 读回（不丢）。
func TestModelCompressionStatPersistAndReload(t *testing.T) {
	truncateTables(t)
	ResetModelCompressionStatBufferForTest()

	RecordModelCompression("gpt-x", 1_000_000, 20_000)
	RecordModelCompression("gpt-x", 500_000, 10_000)
	RecordModelCompression("glm-y", 300_000, 300_000)

	// flush 前：DB 为空，但合并读取应含进程内增量。
	rows, err := GetModelCompressionStats()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byName := map[string]ModelCompressionStat{}
	for _, r := range rows {
		byName[r.ModelName] = r
	}
	assert.EqualValues(t, 2, byName["gpt-x"].Count)
	assert.EqualValues(t, 1_500_000, byName["gpt-x"].OriginalBytes)
	assert.EqualValues(t, 30_000, byName["gpt-x"].CompressedBytes)
	assert.EqualValues(t, 1_470_000, byName["gpt-x"].SavedBytes)

	// flush 落库。
	FlushModelCompressionStats()

	// 模拟重启：清空进程内缓冲，应从 DB 读回同样的值。
	ResetModelCompressionStatBufferForTest()
	rows2, err := GetModelCompressionStats()
	require.NoError(t, err)
	byName2 := map[string]ModelCompressionStat{}
	for _, r := range rows2 {
		byName2[r.ModelName] = r
	}
	assert.EqualValues(t, 2, byName2["gpt-x"].Count, "count must survive restart")
	assert.EqualValues(t, 1_470_000, byName2["gpt-x"].SavedBytes, "saved bytes must survive restart")

	// 再累加 + flush 应是加法（不覆盖）。
	RecordModelCompression("gpt-x", 100_000, 5_000)
	FlushModelCompressionStats()
	rows3, _ := GetModelCompressionStats()
	for _, r := range rows3 {
		if r.ModelName == "gpt-x" {
			assert.EqualValues(t, 3, r.Count, "flush must accumulate, not overwrite")
		}
	}
}
