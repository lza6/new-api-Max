package service

import (
	"testing"

	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppendBillingExplain B5-1：explain.facts 结构与敏感信息排除。
func TestAppendBillingExplain(t *testing.T) {
	other := model.NewLogOther()
	appendBillingExplain(other, textQuotaSummary{
		PromptTokens:     1234,
		CompletionTokens: 567,
		CacheTokens:      800,
		CacheRatio:       0.5,
		ModelRatio:       2.5,
		CompletionRatio:  3,
		GroupRatio:       1.2,
	})

	snapshot := other.Snapshot()
	raw, ok := snapshot["explain"]
	require.True(t, ok, "explain 必须写入 other")
	explain, ok := raw.(map[string]any)
	require.True(t, ok)
	factsRaw, ok := explain["facts"].([]map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, factsRaw)

	byLabel := map[string]any{}
	for _, f := range factsRaw {
		byLabel[f["label"].(string)] = f["value"]
	}
	assert.Equal(t, 1234, byLabel["prompt_tokens"])
	assert.Equal(t, 800, byLabel["cached_tokens"])
	assert.Equal(t, 567, byLabel["completion_tokens"])
	assert.Equal(t, 2.5, byLabel["model_ratio"])
	assert.Equal(t, 0.5, byLabel["cache_ratio"])
	assert.Equal(t, 1.2, byLabel["group_ratio"])

	// ModelPrice 优先于 ratio（facts 顺序：prompt → completion → model_price → group_ratio）。
	other2 := model.NewLogOther()
	appendBillingExplain(other2, textQuotaSummary{PromptTokens: 1, ModelPrice: 0.02})
	snap2 := other2.Snapshot()
	explain2 := snap2["explain"].(map[string]any)
	facts2 := explain2["facts"].([]map[string]any)
	labels2 := map[string]any{}
	for _, f := range facts2 {
		labels2[f["label"].(string)] = f["value"]
	}
	assert.Equal(t, 0.02, labels2["model_price"], "model_price 场景走价格而非 ratio")

	// 零摘要：只输出基础 token 与 group_ratio（无空值噪音）。
	other3 := model.NewLogOther()
	appendBillingExplain(other3, textQuotaSummary{})
	snap3 := other3.Snapshot()
	facts3 := snap3["explain"].(map[string]any)["facts"].([]map[string]any)
	assert.Len(t, facts3, 4)
}
