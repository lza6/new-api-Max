package service

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 语义缓存测试：默认关、白名单、阈值、鲁棒性与错命中防护。
// 全部使用进程内确定性 embedder（hashingEmbedder），不发起任何外部请求。

func enableSemanticCacheForTest(t *testing.T, models []string, similarity string) {
	t.Helper()
	savedOptions := common.OptionMap
	t.Cleanup(func() {
		common.OptionMap = savedOptions
		_ = config.GlobalConfig.LoadFromDB(savedOptions)
		common.SetFeatureFlagOverride(common.FlagResponseCacheEnabled, nil)
		common.SetFeatureFlagOverride(common.FlagSemanticResponseCache, nil)
		SemanticCacheClear()
		SemanticCacheResetMetricsForTest()
	})

	on, off := "true", "false"
	common.SetFeatureFlagOverride(common.FlagResponseCacheEnabled, &on)
	if models == nil {
		common.SetFeatureFlagOverride(common.FlagSemanticResponseCache, &off)
		return
	}
	common.SetFeatureFlagOverride(common.FlagSemanticResponseCache, &on)
	common.OptionMap = map[string]string{
		"response_cache.semantic_allow_models": toJSONArray(models),
		"response_cache.semantic_similarity":   similarity,
	}
	require.NoError(t, config.GlobalConfig.LoadFromDB(common.OptionMap))
}

func toJSONArray(items []string) string {
	out := "["
	for i, item := range items {
		if i > 0 {
			out += ","
		}
		out += `"` + item + `"`
	}
	return out + "]"
}

func chatBody(prompt string) []byte {
	return []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"` + prompt + `"}]}`)
}

// 关闭时必须零行为：既不命中也不建索引。
func TestSemanticCacheDisabledByDefault(t *testing.T) {
	enableSemanticCacheForTest(t, nil, "")
	t.Cleanup(func() { SemanticCacheClear(); SemanticCacheResetMetricsForTest() })

	SemanticCacheStore(1, "gpt-test", "openai", chatBody("hello world"), []byte(`{"ok":true}`), 3)
	assert.Equal(t, 0, SemanticCacheLiveEntries(), "disabled cache must not index anything")

	_, ok := SemanticCacheLookup(1, "gpt-test", "openai", chatBody("hello world"))
	assert.False(t, ok, "disabled cache must never hit")
}

// 白名单内 + 措辞微变 → 命中；完全不同的问题 → 不命中。
func TestSemanticCacheHitAndMiss(t *testing.T) {
	enableSemanticCacheForTest(t, []string{"gpt-test"}, "0.97")
	t.Cleanup(func() { SemanticCacheClear(); SemanticCacheResetMetricsForTest() })

	original := chatBody("Explain how HTTP range requests work for resumable downloads")
	SemanticCacheStore(1, "gpt-test", "openai", original, []byte(`{"answer":"range"}`), 42)

	// 大小写 + 空白 + 标点差异：应命中。
	near := chatBody("explain how http   range requests work, for resumable downloads!")
	hit, ok := SemanticCacheLookup(1, "gpt-test", "openai", near)
	require.True(t, ok, "near-identical prompt must hit")
	assert.Equal(t, `{"answer":"range"}`, string(hit.Body))
	assert.EqualValues(t, 42, hit.TotalTokens)

	// 无关问题：不应命中（防止答非所问）。
	_, ok = SemanticCacheLookup(1, "gpt-test", "openai", chatBody("Write a haiku about autumn leaves"))
	assert.False(t, ok, "unrelated prompt must not hit")

	hits, misses := SemanticCacheMetrics()
	assert.EqualValues(t, 1, hits)
	assert.EqualValues(t, 1, misses)
}

// 白名单外模型永不命中，即便索引里存在同用户其它模型的条目。
func TestSemanticCacheRespectsAllowlist(t *testing.T) {
	enableSemanticCacheForTest(t, []string{"gpt-test"}, "0.9")
	t.Cleanup(func() { SemanticCacheClear(); SemanticCacheResetMetricsForTest() })

	body := chatBody("repeatable prompt for allowlist test")
	SemanticCacheStore(1, "gpt-other", "openai", body, []byte(`{"answer":"x"}`), 1)

	_, ok := SemanticCacheLookup(1, "gpt-other", "openai", body)
	assert.False(t, ok, "model outside the allowlist must never hit")
	assert.Equal(t, 0, SemanticCacheLiveEntries())
}

// 用户隔离：不同用户即使 prompt 相同也不复用。
func TestSemanticCacheIsolatesUsers(t *testing.T) {
	enableSemanticCacheForTest(t, []string{"gpt-test"}, "0.97")
	t.Cleanup(func() { SemanticCacheClear(); SemanticCacheResetMetricsForTest() })

	body := chatBody("shared prompt string for isolation test")
	SemanticCacheStore(1, "gpt-test", "openai", body, []byte(`{"answer":"u1"}`), 1)

	_, ok := SemanticCacheLookup(2, "gpt-test", "openai", body)
	assert.False(t, ok, "another user must not read this entry")
}

// 中文（无空格）必须能用 bigram 建立可用相似度。
func TestSemanticCacheChineseBigram(t *testing.T) {
	enableSemanticCacheForTest(t, []string{"gpt-test"}, "0.9")
	t.Cleanup(func() { SemanticCacheClear(); SemanticCacheResetMetricsForTest() })

	SemanticCacheStore(1, "gpt-test", "openai", chatBody("请解释什么是断点续传"), []byte(`{"answer":"断点续传"}`), 1)
	_, ok := SemanticCacheLookup(1, "gpt-test", "openai", chatBody("请解释 什么是断点续传？"))
	assert.True(t, ok, "CJK prompts differing only in spacing must hit")
}

// prompt 文本抽取覆盖 chat / responses / completions 三种入参形态。
func TestExtractSemanticPromptText(t *testing.T) {
	assert.Contains(t, extractSemanticPromptText([]byte(`{"messages":[{"role":"user","content":"plain"}]}`)), "plain")
	assert.Contains(t, extractSemanticPromptText([]byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"segmented"}]}]}`)), "segmented")
	assert.Contains(t, extractSemanticPromptText([]byte(`{"prompt":"legacy completion"}`)), "legacy completion")
	assert.Equal(t, "", extractSemanticPromptText([]byte(`not json`)))
	assert.Equal(t, "", extractSemanticPromptText(nil))
}

// 语义索引容量上限：同一桶超过上限后淘汰最旧条目，永不无限增长。
func TestSemanticCacheBoundedMemory(t *testing.T) {
	enableSemanticCacheForTest(t, []string{"gpt-test"}, "0.99")
	t.Cleanup(func() { SemanticCacheClear(); SemanticCacheResetMetricsForTest() })

	for i := 0; i < semanticMaxEntriesPerKey+10; i++ {
		prompt := "unique prompt number " + string(rune('a'+i%26)) + string(rune('a'+i/26))
		SemanticCacheStore(1, "gpt-test", "openai", chatBody(prompt), []byte(`{"ok":1}`), 1)
	}
	assert.LessOrEqual(t, SemanticCacheLiveEntries(), semanticMaxEntriesPerKey,
		"per-bucket capacity must be enforced")
}
