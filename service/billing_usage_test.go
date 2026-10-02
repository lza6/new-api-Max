package service

import (
	"testing"

	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppendBillingExplain B5-1：explain.facts 结构与敏感信息排除。
func TestAppendBillingExplain(t *testing.T) {
	other := model.NewLogOther()
	appendBillingExplain(nil, other, textQuotaSummary{
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
	appendBillingExplain(nil, other2, textQuotaSummary{PromptTokens: 1, ModelPrice: 0.02})
	snap2 := other2.Snapshot()
	explain2 := snap2["explain"].(map[string]any)
	facts2 := explain2["facts"].([]map[string]any)
	labels2 := map[string]any{}
	for _, f := range facts2 {
		labels2[f["label"].(string)] = f["value"]
	}
	assert.Equal(t, 0.02, labels2["model_price"], "model_price 场景走价格而非 ratio")

	// 零摘要：只输出基础 token 与 group_ratio（无空值噪音），inferences 为空。
	other3 := model.NewLogOther()
	appendBillingExplain(nil, other3, textQuotaSummary{})
	snap3 := other3.Snapshot()
	explain3 := snap3["explain"].(map[string]any)
	facts3 := explain3["facts"].([]map[string]any)
	assert.Len(t, facts3, 4)
	inferences3, ok := explain3["inferences"].([]map[string]any)
	require.True(t, ok, "inferences 必须始终输出数组（前端依赖）")
	assert.Empty(t, inferences3, "普通请求不应有推断")
}

// TestAppendBillingExplainTiered B5-1：tiered 结算路径的 facts 与 inferences。
func TestAppendBillingExplainTiered(t *testing.T) {
	other := model.NewLogOther()
	appendBillingExplain(nil, other, textQuotaSummary{
		PromptTokens:     10,
		CompletionTokens: 20,
		MatchedTier:      "0-4k",
		BillingUnit:      "token",
		GroupRatio:       1,
	})
	snap := other.Snapshot()
	explain := snap["explain"].(map[string]any)
	facts := explain["facts"].([]map[string]any)
	byLabel := map[string]any{}
	for _, f := range facts {
		byLabel[f["label"].(string)] = f["value"]
	}
	assert.Equal(t, "0-4k", byLabel["tier_matched"])
	assert.Equal(t, "token", byLabel["billing_unit"])

	infs := explain["inferences"].([]map[string]any)
	require.Len(t, infs, 1, "tiered 命中应有一条阶梯价推断")
	assert.Contains(t, infs[0]["text"], "0-4k")
	assert.Equal(t, "pricing", infs[0]["kind"])
}

// TestAppendBillingExplainFixedPrice B5-1：fixed-price（按请求计费）路径。
func TestAppendBillingExplainFixedPrice(t *testing.T) {
	other := model.NewLogOther()
	appendBillingExplain(nil, other, textQuotaSummary{
		PromptTokens:      1,
		CompletionTokens:  0,
		FixedPriceBilling: true,
		FixedPriceTier:    "pro-request",
		FixedPriceValue:   0.05,
		GroupRatio:        1,
	})
	snap := other.Snapshot()
	explain := snap["explain"].(map[string]any)
	facts := explain["facts"].([]map[string]any)
	byLabel := map[string]any{}
	for _, f := range facts {
		byLabel[f["label"].(string)] = f["value"]
	}
	assert.Equal(t, 0.05, byLabel["fixed_price"])
	assert.Equal(t, "pro-request", byLabel["tier_matched"])

	infs := explain["inferences"].([]map[string]any)
	require.Len(t, infs, 1, "fixed-price 应有一条按请求计费推断")
	assert.Equal(t, "pricing", infs[0]["kind"])
}

// 延迟拆解写入消费日志：有效计时写入三个字段，未采集（-1）时省略。
func TestAppendUpstreamTiming(t *testing.T) {
	// 有效计时：三段都写入。
	other := model.NewLogOther()
	info := &relaycommon.RelayInfo{
		UpstreamConnectMs: 6,
		UpstreamUploadMs:  692,
		UpstreamTtfbMs:    5874,
	}
	appendUpstreamTiming(info, other)
	snap := other.Snapshot()
	assert.EqualValues(t, 6, snap["upstream_connect_ms"])
	assert.EqualValues(t, 692, snap["upstream_upload_ms"])
	assert.EqualValues(t, 5874, snap["upstream_ttfb_ms"])

	// 未采集（-1）：不写任何字段（旧日志/非 relay 路径兼容）。
	other2 := model.NewLogOther()
	info2 := &relaycommon.RelayInfo{
		UpstreamConnectMs: -1,
		UpstreamUploadMs:  -1,
		UpstreamTtfbMs:    -1,
	}
	appendUpstreamTiming(info2, other2)
	snap2 := other2.Snapshot()
	_, hasUpload := snap2["upstream_upload_ms"]
	_, hasTtfb := snap2["upstream_ttfb_ms"]
	assert.False(t, hasUpload, "未采集时不应写入 upstream_upload_ms")
	assert.False(t, hasTtfb, "未采集时不应写入 upstream_ttfb_ms")

	// nil info 不 panic。
	other3 := model.NewLogOther()
	appendUpstreamTiming(nil, other3)
	assert.Empty(t, other3.Snapshot())
}
