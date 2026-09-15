package model

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reportJSON(t *testing.T, id int) string {
	t.Helper()
	b, err := common.Marshal(map[string]any{"id": id, "grade": "A"})
	require.NoError(t, err)
	return string(b)
}

// TestMergeProbeHistoryKeepsMostRecent5 B4-1：新报告前插，历史截断到最近 5 次。
func TestMergeProbeHistoryKeepsMostRecent5(t *testing.T) {
	// 已有 5 条历史，插入新报告 → 保持 5 条且最新在前。
	existing := "[{\"id\":1},{\"id\":2},{\"id\":3},{\"id\":4},{\"id\":5}]"
	merged, err := mergeProbeHistory(existing, reportJSON(t, 6))
	require.NoError(t, err)
	var items []map[string]any
	require.NoError(t, common.Unmarshal([]byte(merged), &items))
	require.Len(t, items, 5)
	first := items[0]["id"].(float64)
	assert.Equal(t, float64(6), first, "最新报告必须在前")
	last := items[4]["id"].(float64)
	assert.Equal(t, float64(4), last, "最旧 1 号被截断（保留 id6+原1-4）")
}

// TestMergeProbeHistoryEmptyExisting B4-1：无历史时从单条开始。
func TestMergeProbeHistoryEmptyExisting(t *testing.T) {
	merged, err := mergeProbeHistory("[]", reportJSON(t, 1))
	require.NoError(t, err)
	var items []map[string]any
	require.NoError(t, common.Unmarshal([]byte(merged), &items))
	require.Len(t, items, 1)
	assert.Equal(t, float64(1), items[0]["id"].(float64))
}

// TestMergeProbeHistoryCorruptExistingFallsBackToSingle B4-1：坏历史回退为单条，
// 探测功能不被坏数据卡死。
func TestMergeProbeHistoryCorruptExistingFallsBackToSingle(t *testing.T) {
	_, err := mergeProbeHistory("not-json", reportJSON(t, 1))
	assert.Error(t, err, "坏历史应返回错误，由调用方回退")
}

// TestProbeHistoryMaxAgeIs24Hours B4-1：成本控制——每日 1 轮，24h 窗口。
func TestProbeHistoryMaxAgeIs24Hours(t *testing.T) {
	assert.Equal(t, 24*3600, int(probeHistoryMaxAge().Seconds()))
}
