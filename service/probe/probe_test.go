package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockUpstream 构造 OpenAI 兼容 mock 上游：按请求 body 返回可编程响应。
func newMockUpstream(t *testing.T, handler http.HandlerFunc) *ProbeTarget {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &ProbeTarget{
		ChannelID:   1,
		Name:        "mock",
		BaseURL:     server.URL,
		Key:         "sk-mock-key-1234567890",
		Model:       "mock-model",
		TimeoutSecs: 10,
	}
}

func chatBodyJSON(n int, content string, cachedTokens int) string {
	details := "{}"
	if cachedTokens > 0 {
		details = `{"cached_tokens": ` + itoaTest(cachedTokens) + `}`
	}
	return `{"choices":[{"message":{"role":"assistant","content":"` + content + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":` + details + `}}`
}

// chatHandler 按 n 参数返回对应数量的 choices（能力用例）；其余请求按脚本应答。
// 支持 stream=true（返回多条 SSE data）与 usage（自洽 10+5=15）。
func chatHandler(t *testing.T, singleAnswer string, answers []string, cachedTokens int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 读 body 提取 n 与 stream（简化：探测请求都带 JSON body）。
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		if countOccurrences(string(buf), `"n":2`) > 0 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a"}},{"message":{"content":"b"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			return
		}
		// 流式请求：返回多条 SSE data 块。
		if countOccurrences(string(buf), `"stream":true`) > 0 {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, piece := range []string{`{"choices":[{"delta":{"content":"我"}}]}`, `{"choices":[{"delta":{"content":"是"}}]}`, `{"choices":[{"delta":{"content":"助手"}}]}`} {
				_, _ = w.Write([]byte("data: " + piece + "\n\n"))
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		if len(answers) > 0 {
			ans := answers[0]
			answers = answers[1:]
			_, _ = w.Write([]byte(chatBodyJSON(1, ans, cachedTokens)))
			return
		}
		if singleAnswer != "" {
			_, _ = w.Write([]byte(chatBodyJSON(1, singleAnswer, cachedTokens)))
			return
		}
		_, _ = w.Write([]byte(chatBodyJSON(1, "ok", cachedTokens)))
	}
}

func countOccurrences(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}

func itoaTest(v int) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

func TestModelIDCasePass(t *testing.T) {
	// 王勃/H2O 两问均命中预期 → 满分。
	target := newMockUpstream(t, chatHandler(t, "", []string{"王勃", "H2O"}, 0))
	result := ModelIDCase{}.Run(context.Background(), target)
	require.NoError(t, nil)
	assert.True(t, result.Passed)
	assert.Equal(t, 25.0, result.Score)
	assert.NotContains(t, result.Evidence, "sk-mock", "证据不得包含渠道 key")
}

func TestModelIDCaseIdentityMismatch(t *testing.T) {
	// 套壳渠道：回答错误 → 不通过，证据含分歧。
	target := newMockUpstream(t, chatHandler(t, "", []string{"李白", "H2O"}, 0))
	result := ModelIDCase{}.Run(context.Background(), target)
	assert.False(t, result.Passed)
	assert.Zero(t, result.Score)
	assert.Contains(t, result.Error, "identity mismatch")
}

func TestCapabilityCaseNSupported(t *testing.T) {
	target := newMockUpstream(t, chatHandler(t, "", nil, 0))
	result := CapabilityCase{}.Run(context.Background(), target)
	assert.True(t, result.Passed)
	assert.Equal(t, 20.0, result.Score)
}

func TestCapabilityCaseNSilentlyDropped(t *testing.T) {
	// 静默丢弃 n=2：只回 1 个 choice → 0 分。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(server.Close)
	target := &ProbeTarget{BaseURL: server.URL, Key: "sk-x-12345678901234", Model: "m", TimeoutSecs: 5}
	result := CapabilityCase{}.Run(context.Background(), target)
	assert.False(t, result.Passed)
	assert.Zero(t, result.Score)
	assert.Contains(t, result.Error, "silently dropped")
}

func TestCacheCaseCachedTokensReported(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		cached := 0
		if calls >= 2 {
			cached = 8
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatBodyJSON(1, "好", cached)))
	}))
	t.Cleanup(server.Close)
	target := &ProbeTarget{BaseURL: server.URL, Key: "sk-x-12345678901234", Model: "m", TimeoutSecs: 5}
	result := CacheCase{}.Run(context.Background(), target)
	assert.True(t, result.Passed)
	assert.Equal(t, 20.0, result.Score)
}

func TestCacheCaseNoCacheDetailHalfScore(t *testing.T) {
	// usage 无 cached_tokens：无法证伪 → 半分。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatBodyJSON(1, "好", 0)))
	}))
	t.Cleanup(server.Close)
	target := &ProbeTarget{BaseURL: server.URL, Key: "sk-x-12345678901234", Model: "m", TimeoutSecs: 5}
	result := CacheCase{}.Run(context.Background(), target)
	assert.False(t, result.Passed)
	assert.Equal(t, 10.0, result.Score)
	assert.Contains(t, result.Error, "cached_tokens")
}

func TestConsistencyCase(t *testing.T) {
	// 两次一致 → 通过。
	target := newMockUpstream(t, chatHandler(t, "万有引力是物体间相互吸引的力", nil, 0))
	result := ConsistencyCase{}.Run(context.Background(), target)
	assert.True(t, result.Passed)
	assert.Equal(t, 15.0, result.Score)
}

func TestRunProbeFullReport(t *testing.T) {
	// 全良渠道：4 用例满分 → 等级 A。
	target := newMockUpstream(t, chatHandler(t, "", []string{"王勃", "H2O", "x"}, 8))
	// 一致性用例两次请求复用 singleAnswer。
	report := RunProbe(context.Background(), target)
	require.NotEmpty(t, report.Results)
	for _, r := range report.Results {
		t.Logf("case=%s passed=%v score=%.1f/%.1f err=%q", r.Name, r.Passed, r.Score, r.Weight, r.Error)
	}
	assert.Equal(t, GradeA, report.Grade)
	assert.Positive(t, report.Score)
	assert.Positive(t, report.ProbedAt)
}

func TestScoreGrade(t *testing.T) {
	assert.Equal(t, GradeA, ScoreGrade(80, 80))
	assert.Equal(t, GradeB, ScoreGrade(65, 80))
	assert.Equal(t, GradeC, ScoreGrade(50, 80))
	assert.Equal(t, GradeD, ScoreGrade(30, 80))
	assert.Equal(t, GradeF, ScoreGrade(20, 80))
	assert.Equal(t, GradeF, ScoreGrade(10, 0), "零权重必须 F")
}

func TestTruncateEvidence(t *testing.T) {
	long := make([]byte, 3000)
	for i := range long {
		long[i] = 'a'
	}
	got := truncateEvidence(string(long))
	assert.LessOrEqual(t, len(got), MaxEvidenceBytes+16)
	assert.Contains(t, got, "truncated")
}

// TestStreamIntegrityCasePass B4-1 用例5：mock 返回多条 SSE → 通过。
func TestStreamIntegrityCasePass(t *testing.T) {
	target := newMockUpstream(t, chatHandler(t, "", nil, 0))
	result := StreamIntegrityCase{}.Run(context.Background(), target)
	assert.True(t, result.Passed, "流式渠道应返回多 chunks: %s", result.Error)
	assert.Equal(t, 10.0, result.Score)
	assert.Contains(t, result.Evidence, "chunks=3")
}

// TestStreamIntegrityCaseSingleChunkFails B4-1 用例5：只回 1 chunk 视为非真流式。
func TestStreamIntegrityCaseSingleChunkFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"only\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)
	target := &ProbeTarget{BaseURL: server.URL, Key: "sk-x-12345678901234", Model: "m", TimeoutSecs: 5}
	result := StreamIntegrityCase{}.Run(context.Background(), target)
	assert.False(t, result.Passed, "单 chunk 不算真流式")
	assert.Zero(t, result.Score)
	assert.Contains(t, result.Error, ">=2")
}

// TestBillingConsistencyCasePass B4-1 用例6：usage 自洽（10+5=15）→ 通过。
func TestBillingConsistencyCasePass(t *testing.T) {
	target := newMockUpstream(t, chatHandler(t, "", nil, 0))
	result := BillingConsistencyCase{}.Run(context.Background(), target)
	assert.True(t, result.Passed, "usage 自洽应通过: %s", result.Error)
	assert.Equal(t, 10.0, result.Score)
	assert.Contains(t, result.Evidence, "prompt=10 completion=5 total=15")
}

// TestBillingConsistencyCaseInconsistent B4-1 用例6：usage 打架 → 不通过。
func TestBillingConsistencyCaseInconsistent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":999}}`))
	}))
	t.Cleanup(server.Close)
	target := &ProbeTarget{BaseURL: server.URL, Key: "sk-x-12345678901234", Model: "m", TimeoutSecs: 5}
	result := BillingConsistencyCase{}.Run(context.Background(), target)
	assert.False(t, result.Passed, "usage 不一致应失败")
	assert.Zero(t, result.Score)
	assert.Contains(t, result.Error, "inconsistent")
}

// TestRunProbeGoodChannelGradeA B4-1 验收：良渠道（全用例通过）→ 等级 A。
func TestRunProbeGoodChannelGradeA(t *testing.T) {
	target := newMockUpstream(t, chatHandler(t, "", []string{"王勃", "H2O", "x"}, 8))
	report := RunProbe(context.Background(), target)
	assert.Equal(t, GradeA, report.Grade)
	assert.InDelta(t, 100.0, report.TotalWeight, 0.001)
	assert.Len(t, report.Results, 6, "六维题库")
	// 劣渠道须在 D/F 档，此处证明良渠道满分可达 A。
}

// TestRunProbeBadChannelGradeF B4-1 验收：套壳渠道（模型身份错 + 非流式 +
// usage 打架）→ 等级 F。用恶意 mock 模拟"挂羊头卖狗肉"。
func TestRunProbeBadChannelGradeF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		if countOccurrences(string(buf), `"stream":true`) > 0 {
			// 非真流式：只回单块。
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		if countOccurrences(string(buf), `"temperature":0`) > 0 {
			// 一致性：两次返回不同内容（套壳随机）。
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"第一次回答A"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			return
		}
		// 模型身份：答非所问；usage：打架。
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"我是另一个模型，答非所问"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":999}}`))
	}))
	t.Cleanup(server.Close)
	target := &ProbeTarget{BaseURL: server.URL, Key: "sk-x-12345678901234", Model: "m", TimeoutSecs: 10}
	report := RunProbe(context.Background(), target)
	assert.Equal(t, GradeF, report.Grade, "套壳渠道应得 F，实际 grade=%s", report.Grade)
	assert.Less(t, report.Score, 35.0, "F 档要求得分 <35（<35%%），实际 %.1f", report.Score)
}
