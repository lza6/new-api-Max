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
func chatHandler(t *testing.T, singleAnswer string, answers []string, cachedTokens int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 读 body 提取 n（简化：探测请求都带 JSON body）。
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		if countOccurrences(string(buf), `"n":2`) > 0 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a"}},{"message":{"content":"b"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
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
