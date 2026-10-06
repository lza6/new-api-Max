package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSumUsedQuotaClampsOpenEndedWindow B2-4：SumUsedQuota 对「无上界/超长」时间窗口
// 收敛到 LOG_STAT_MAX_DAYS 上限，避免对全历史无界 SUM 聚合；窗口内的正常查询不受影响。
func TestSumUsedQuotaClampsOpenEndedWindow(t *testing.T) {
	db := setupLogPaginationDB(t)

	now := time.Now().Unix()
	// 一条“很久以前”（400 天前）的日志，一条最近的。
	logs := []*Log{
		{Type: LogTypeConsume, ModelName: "m", Username: "u", CreatedAt: now - 400*86400, Quota: 100},
		{Type: LogTypeConsume, ModelName: "m", Username: "u", CreatedAt: now - 86400, Quota: 7},
	}
	require.NoError(t, db.Create(&logs).Error)

	// 默认上限 366 天：startTimestamp=0（全量）应被收敛到近 366 天 → 只统计近 1 天那条。
	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "u", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, 7, stat.Quota, "open-ended window must be clamped, excluding >366d-old rows")

	// 显式给定窗口内 start：保持调用方原值，正常统计。
	stat2, err := SumUsedQuota(LogTypeConsume, now-2*86400, 0, "", "u", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, 7, stat2.Quota)

	// 上限可配为 0（关闭）→ 恢复全量统计。
	t.Setenv("LOG_STAT_MAX_DAYS", "0")
	stat3, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "u", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, 107, stat3.Quota, "cap=0 disables clamping and sums all history")
}
