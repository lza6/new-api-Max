package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/response_cache_setting"
)

// Batch-9 / G3 响应缓存的契约测试。
//
// 最重要的两条不变量：
//  1. **跨用户隔离** —— 默认配置下 A 用户写入的响应绝不能被 B 用户读到（Red Team 的头号风险）；
//  2. **不缓存不该缓存的东西** —— 流式、非白名单模型、无 usage 的响应一律不进缓存。

// enableResponseCacheForTest 打开总开关并复位缓存与统计。
func enableResponseCacheForTest(t *testing.T) {
	t.Helper()
	enabled := "true"
	common.SetFeatureFlagOverride(common.FlagResponseCacheEnabled, &enabled)
	response_cache_setting.ResetStatsForTest()
	ResponseCacheClear()
	t.Cleanup(func() {
		common.ClearFeatureFlagOverrides()
		response_cache_setting.ResetStatsForTest()
		response_cache_setting.SetForTest(0, 0, []string{}, []string{}, nil)
		ResponseCacheClear()
	})
}

const (
	testCacheModel  = "gpt-cache-test"
	testCacheFormat = "openai"
)

func cacheableRequest(t *testing.T, temperature float64) []byte {
	t.Helper()
	body, err := common.Marshal(map[string]any{
		"model":       testCacheModel,
		"messages":    []map[string]string{{"role": "user", "content": "hello"}},
		"temperature": temperature,
	})
	require.NoError(t, err)
	return body
}

func TestResponseCacheIsOffByDefault(t *testing.T) {
	ResponseCacheClear()
	response_cache_setting.ResetStatsForTest()
	response_cache_setting.SetForTest(0, 0, []string{testCacheModel}, nil, nil)
	t.Cleanup(func() { response_cache_setting.SetForTest(0, 0, []string{}, nil, nil) })
	// 故意不打开开关

	require.False(t, response_cache_setting.Enabled(), "默认必须是关")

	body := cacheableRequest(t, 0.1)
	ResponseCacheStore(7, testCacheModel, testCacheFormat, body, []byte(`{"ok":1}`), 5)
	_, hit := ResponseCacheLookup(7, testCacheModel, testCacheFormat, body)
	assert.False(t, hit, "开关关闭时既不能写也不能命中")
	assert.False(t, ResponseCacheEligible(7, testCacheModel, testCacheFormat, false))
}

func TestResponseCacheRequiresModelAllowlist(t *testing.T) {
	enableResponseCacheForTest(t)

	// 白名单为空 = 全不缓存（保守起步）
	response_cache_setting.SetForTest(0, 0, []string{}, []string{}, nil)
	assert.False(t, ResponseCacheEligible(1, testCacheModel, testCacheFormat, false),
		"白名单为空时必须全不缓存")

	// 显式加白名单后才可以
	response_cache_setting.SetForTest(0, 0, []string{"gpt-cache-*"}, nil, nil)
	assert.True(t, ResponseCacheEligible(1, testCacheModel, testCacheFormat, false))

	// deny 优先于 allow
	response_cache_setting.SetForTest(0, 0, []string{"gpt-cache-*"}, []string{"gpt-cache-test"}, nil)
	assert.False(t, ResponseCacheEligible(1, testCacheModel, testCacheFormat, false),
		"deny 必须优先于 allow")
}

func TestResponseCacheEligibleRejectsStreamAndUnknownFormat(t *testing.T) {
	enableResponseCacheForTest(t)
	response_cache_setting.SetForTest(0, 0, []string{"*"}, nil, nil)

	assert.False(t, ResponseCacheEligible(1, testCacheModel, testCacheFormat, true), "流式不缓存")
	assert.False(t, ResponseCacheEligible(1, testCacheModel, "", false), "未知格式不缓存")
	assert.False(t, ResponseCacheEligible(0, testCacheModel, testCacheFormat, false), "无用户归属不缓存")
	assert.True(t, ResponseCacheEligible(1, testCacheModel, testCacheFormat, false))
}

func TestResponseCacheHitAndMiss(t *testing.T) {
	enableResponseCacheForTest(t)
	response_cache_setting.SetForTest(0, 0, []string{"*"}, nil, nil)

	req := cacheableRequest(t, 0.1)
	resp := []byte(`{"id":"a","usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`)

	_, hit := ResponseCacheLookup(1, testCacheModel, testCacheFormat, req)
	require.False(t, hit, "未写入时不应命中")

	ResponseCacheStore(1, testCacheModel, testCacheFormat, req, resp, 7)

	got, hit := ResponseCacheLookup(1, testCacheModel, testCacheFormat, req)
	require.True(t, hit, "写入后必须命中")
	assert.Equal(t, resp, got.Body, "命中必须原样返回上游字节（usage 因此天然如实回填）")
	assert.EqualValues(t, 7, got.TotalTokens)
}

// 本项最关键的不变量：默认配置下不可能跨用户命中。
func TestResponseCacheIsolatesUsersByDefault(t *testing.T) {
	enableResponseCacheForTest(t)
	response_cache_setting.SetForTest(0, 0, []string{"*"}, nil, nil) // AllowCrossUser 保持默认 false

	req := cacheableRequest(t, 0.1)
	ResponseCacheStore(1001, testCacheModel, testCacheFormat, req, []byte(`{"for":"user-1001"}`), 7)

	_, hit := ResponseCacheLookup(1002, testCacheModel, testCacheFormat, req)
	assert.False(t, hit, "默认配置下另一个用户绝不能命中（跨用户泄漏是这项最严重的风险）")

	_, own := ResponseCacheLookup(1001, testCacheModel, testCacheFormat, req)
	assert.True(t, own, "写入者自己必须能命中")
}

func TestResponseCacheSharesAcrossUsersOnlyWhenExplicitlyAllowed(t *testing.T) {
	enableResponseCacheForTest(t)
	crossUser := true
	response_cache_setting.SetForTest(0, 0, []string{"*"}, nil, &crossUser)

	req := cacheableRequest(t, 0.1)
	ResponseCacheStore(2001, testCacheModel, testCacheFormat, req, []byte(`{"shared":true}`), 7)

	_, hit := ResponseCacheLookup(2002, testCacheModel, testCacheFormat, req)
	assert.True(t, hit, "显式允许跨用户共享时才共享")
}

func TestResponseCacheKeyIgnoresTransportOnlyFields(t *testing.T) {
	enableResponseCacheForTest(t)
	response_cache_setting.SetForTest(0, 0, []string{"*"}, nil, nil)

	base := `{"model":"gpt-cache-test","messages":[{"role":"user","content":"hi"}]}`
	withTransport := `{"model":"gpt-cache-test","messages":[{"role":"user","content":"hi"}],"stream":false,"stream_options":{"include_usage":true},"user":"client-abc"}`

	ResponseCacheStore(1, testCacheModel, testCacheFormat, []byte(base), []byte(`{"x":1}`), 1)

	_, hit := ResponseCacheLookup(1, testCacheModel, testCacheFormat, []byte(withTransport))
	assert.True(t, hit, "stream / stream_options / user 不影响输出，必须被剔除出键")

	// 顶层键序也要无关（map 序列化按 key 排序）
	reordered := `{"messages":[{"role":"user","content":"hi"}],"model":"gpt-cache-test"}`
	_, hit2 := ResponseCacheLookup(1, testCacheModel, testCacheFormat, []byte(reordered))
	assert.True(t, hit2, "顶层键序不同不应导致漏命中")
}

func TestResponseCacheKeyChangesWithSamplingParams(t *testing.T) {
	enableResponseCacheForTest(t)
	response_cache_setting.SetForTest(0, 0, []string{"*"}, nil, nil)

	ResponseCacheStore(1, testCacheModel, testCacheFormat, cacheableRequest(t, 0.1), []byte(`{"x":1}`), 1)

	_, hit := ResponseCacheLookup(1, testCacheModel, testCacheFormat, cacheableRequest(t, 0.9))
	assert.False(t, hit, "采样参数变化必须导致不命中（宁可不命中，不可错命中）")

	_, hitDiffModel := ResponseCacheLookup(1, "gpt-cache-test-2", testCacheFormat, cacheableRequest(t, 0.1))
	assert.False(t, hitDiffModel, "模型不同必须不命中")

	_, hitDiffFormat := ResponseCacheLookup(1, testCacheModel, "claude", cacheableRequest(t, 0.1))
	assert.False(t, hitDiffFormat, "客户端格式不同必须不命中（响应体格式不同）")
}

func TestResponseCacheExpiresAfterTTL(t *testing.T) {
	enableResponseCacheForTest(t)
	response_cache_setting.SetForTest(1, 0, []string{"*"}, nil, nil) // TTL = 1s

	req := cacheableRequest(t, 0.1)
	ResponseCacheStore(1, testCacheModel, testCacheFormat, req, []byte(`{"x":1}`), 1)
	_, hit := ResponseCacheLookup(1, testCacheModel, testCacheFormat, req)
	require.True(t, hit)

	time.Sleep(1200 * time.Millisecond)

	_, expired := ResponseCacheLookup(1, testCacheModel, testCacheFormat, req)
	assert.False(t, expired, "超过 TTL 必须失效")
	assert.Zero(t, ResponseCacheLiveEntries(), "过期条目应被移除而不是留在内存里")
}

func TestResponseCacheEvictsWhenCapacityExceeded(t *testing.T) {
	enableResponseCacheForTest(t)
	response_cache_setting.SetForTest(0, 2, []string{"*"}, nil, nil) // 容量上限 2

	body := func(n int) []byte {
		b, err := common.Marshal(map[string]any{"model": testCacheModel, "n": n})
		require.NoError(t, err)
		return b
	}
	for i := range 5 {
		ResponseCacheStore(1, testCacheModel, testCacheFormat, body(i), []byte(`{"x":1}`), 1)
	}

	assert.LessOrEqual(t, ResponseCacheLiveEntries(), 2,
		"进程内条目数必须有硬上界（无界增长是用户明确关切的问题）")

	// 最新写入的仍在
	_, hit := ResponseCacheLookup(1, testCacheModel, testCacheFormat, body(4))
	assert.True(t, hit, "最新条目应保留")
	// 最早的已被淘汰
	_, evicted := ResponseCacheLookup(1, testCacheModel, testCacheFormat, body(0))
	assert.False(t, evicted, "最旧条目应被淘汰")
}

func TestExtractUsageFromResponseBody(t *testing.T) {
	openai := []byte(`{"id":"x","usage":{"prompt_tokens":11,"completion_tokens":22,"total_tokens":33}}`)
	usage := ExtractUsageFromResponseBody(openai)
	require.NotNil(t, usage)
	assert.Equal(t, 11, usage.PromptTokens)
	assert.Equal(t, 22, usage.CompletionTokens)
	assert.Equal(t, 33, usage.TotalTokens)

	assert.Nil(t, ExtractUsageFromResponseBody([]byte(`{"id":"x"}`)), "无 usage 时返回 nil")
	assert.Nil(t, ExtractUsageFromResponseBody(nil))
	assert.Nil(t, ExtractUsageFromResponseBody([]byte(`not json`)))
}
