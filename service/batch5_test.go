package service

import (
	"strings"
	"testing"
	"time"

	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T4 复杂度路由：7 维打分 + tier 映射。
func TestScoreComplexityTiers(t *testing.T) {
	// 简单请求：短、无代码/数学/推理/工具。
	simple := ScoreComplexity(ComplexitySignals{PromptChars: 50, MessageCount: 1})
	assert.Equal(t, ComplexityTierSimple, simple.Tier)
	assert.Less(t, simple.Total, cxMediumThreshold)

	// 复杂请求：长 + 代码 + 推理 + 多轮 + 工具。
	complex := ScoreComplexity(ComplexitySignals{
		PromptChars: 20000, MessageCount: 12,
		HasCodeFence: true, HasCodeKw: true, LooksCode: true,
		HasReasoning: true, HasTools: true,
	})
	assert.Equal(t, ComplexityTierComplex, complex.Tier)
	assert.GreaterOrEqual(t, complex.Total, cxComplexThreshold)
}

func TestScoreComplexityDimensions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sig     ComplexitySignals
		dim     string
		nonzero bool
	}{
		{"math", ComplexitySignals{HasMathExpr: true}, "math", true},
		{"reasoning", ComplexitySignals{HasReasoning: true}, "reasoning", true},
		{"tools", ComplexitySignals{HasTools: true}, "tools", true},
		{"tool_choice", ComplexitySignals{HasToolChoice: true}, "tools", true},
		{"multimodal", ComplexitySignals{HasMultiModal: true}, "multimodal", true},
		{"multi_turn", ComplexitySignals{MessageCount: 10}, "multi_turn", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := ScoreComplexity(tc.sig)
			var v float64
			switch tc.dim {
			case "math":
				v = sc.Math
			case "reasoning":
				v = sc.Reasoning
			case "tools":
				v = sc.Tools
			case "multimodal":
				v = sc.MultiModal
			case "multi_turn":
				v = sc.MultiTurn
			}
			assert.Greater(t, v, 0.0, "%s dimension must be positive", tc.dim)
		})
	}
}

func TestExtractComplexitySignals(t *testing.T) {
	sig := ExtractComplexitySignals([]string{
		"Please explain why this code fails:\n```go\nfunc main() {}\n```",
	}, ComplexitySignals{MessageCount: 2})
	require.True(t, sig.HasCodeFence)
	require.True(t, sig.HasCodeKw)
	require.True(t, sig.HasReasoning) // "explain"/"why"
	require.Greater(t, sig.PromptChars, 0)
}

// T4 性能：<1ms（宽裕留到 5ms 防 CI 抖动，但应在微秒级）。
func TestScoreComplexityFast(t *testing.T) {
	sig := ComplexitySignals{PromptChars: 5000, MessageCount: 8, HasCodeFence: true}
	start := time.Now()
	for i := 0; i < 1000; i++ {
		_ = ScoreComplexity(sig)
	}
	elapsed := time.Since(start)
	per := elapsed / 1000
	assert.Less(t, per.Microseconds(), int64(500), "scoring must be sub-millisecond; got %v", per)
}

// T7 中继自检：错误泄漏检测。
func TestAuditErrorLeak(t *testing.T) {
	require.Empty(t, AuditErrorLeak("upstream returned 500"))
	require.NotEmpty(t, AuditErrorLeak("auth failed with key sk-abcdef0123456789abcdef"))
	require.NotEmpty(t, AuditErrorLeak("Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"))
	// 堆栈特征 → warn。
	found := AuditErrorLeak("panic at goroutine 12 runtime/panic.go:100")
	require.NotEmpty(t, found)
	assert.Equal(t, "warn", found[0].Severity)
}

// T7 中继自检：usage 单调。
func TestAuditUsageMonotonic(t *testing.T) {
	st := NewUsageMonotonicState()
	require.Empty(t, AuditUsageMonotonic(st, 100, 10, 110))
	require.Empty(t, AuditUsageMonotonic(st, 150, 20, 170))
	// 回退 → warn。
	found := AuditUsageMonotonic(st, 120, 20, 140)
	require.NotEmpty(t, found)
	assert.Equal(t, "usage_monotonic", found[0].Check)
}

// T7 中继自检：模型指纹。
func TestAuditModelFingerprint(t *testing.T) {
	require.Empty(t, AuditModelFingerprint("deepseek-v4.1-flash", "deepseek-v4-flash"))
	require.Empty(t, AuditModelFingerprint("gpt-4o", ""))
	found := AuditModelFingerprint("gpt-4o", "claude-3-opus")
	require.NotEmpty(t, found)
	assert.Equal(t, "model_fingerprint", found[0].Check)
}

// T7 中继自检：SSE 白名单。
func TestAuditSSEWhitelist(t *testing.T) {
	require.Empty(t, AuditSSEWhitelist([]string{"id", "object", "choices", "usage"}))
	found := AuditSSEWhitelist([]string{"id", "evil_field"})
	require.NotEmpty(t, found)
	assert.Contains(t, found[0].Detail, "evil_field")
}

// T1 Savings Baseline：反事实基准计算。
func TestComputeSavingsBaseline(t *testing.T) {
	rows := []model.ModelCompressionStat{
		{ModelName: "m1", Count: 2, OriginalBytes: 1000, CompressedBytes: 400, SavedBytes: 600},
		{ModelName: "m2", Count: 1, OriginalBytes: 500, CompressedBytes: 200, SavedBytes: 300},
	}
	b := ComputeSavingsBaseline(rows)
	assert.Equal(t, int64(1500), b.TotalOriginalBytes)
	assert.Equal(t, int64(900), b.TotalSavedBytes)
	assert.Equal(t, int64(1500), b.CounterfactualBytes)
	assert.InDelta(t, 0.6, b.OverallSavedRatio, 0.001)
	assert.NotEmpty(t, b.SavedText)
	assert.Len(t, b.Models, 2)
}

// T5 工具抽屉：去重 + 省 token 估算。
func TestAnalyzeToolsAndDedup(t *testing.T) {
	defs := []ToolDef{
		{Name: "search", Fingerprint: FingerprintTool("search", `{"type":"object"}`), ArgsBytes: 200},
		{Name: "search", Fingerprint: FingerprintTool("search", `{"type":"object"}`), ArgsBytes: 200}, // dup name
		{Name: "calc", Fingerprint: FingerprintTool("calc", `{"type":"object"}`), ArgsBytes: 100},
	}
	a := AnalyzeTools(defs)
	assert.Equal(t, 3, a.TotalTools)
	assert.Equal(t, 2, a.UniqueTools)
	assert.Equal(t, 1, a.DuplicateCount)
	assert.Equal(t, 200, a.SavedBytes)

	kept := DedupToolDefs(defs)
	require.Equal(t, []int{0, 2}, kept)
}

func TestFingerprintToolStable(t *testing.T) {
	a := FingerprintTool("x", `{ "a" : 1 ,  "b" : 2 }`)
	b := FingerprintTool("x", "{\n  \"a\": 1,\n  \"b\": 2\n}")
	assert.Equal(t, a, b, "fingerprint must ignore whitespace")
	c := FingerprintTool("x", `{"a":1}`)
	assert.NotEqual(t, a, c)
}

func TestMetaFunctionIndex(t *testing.T) {
	defs := []ToolDef{
		{Name: "search", ArgsBytes: 200},
		{Name: "calc", ArgsBytes: 100},
	}
	idx := MetaFunctionIndex(defs, []string{"a very long schema description for search tool", "calc"}, 10)
	require.Len(t, idx, 2)
	assert.Equal(t, "search", idx[0]["name"])
	assert.LessOrEqual(t, len(idx[0]["hint"]), 12) // 含 rune 边界余量
	assert.True(t, strings.HasPrefix(idx[0]["hint"], "a very"))
}

// 空输出 → 错误日志判定：仅上游故障类（eof/timeout/空/非流式）记错误日志；
// 客户端主动断开（client_gone）不记。
func TestEmptyUpstreamResponseIsError(t *testing.T) {
	// 非流式（无 StreamStatus）→ 记。
	require.True(t, emptyUpstreamResponseIsError(&relaycommon.RelayInfo{}))

	// 流式 done（上游确实回空）→ 记。
	require.True(t, emptyUpstreamResponseIsError(&relaycommon.RelayInfo{
		StreamStatus: &relaycommon.StreamStatus{EndReason: relaycommon.StreamEndReasonDone},
	}))
	// eof（上游提前结束）→ 记。
	require.True(t, emptyUpstreamResponseIsError(&relaycommon.RelayInfo{
		StreamStatus: &relaycommon.StreamStatus{EndReason: relaycommon.StreamEndReasonEOF},
	}))
	// client_gone（用户取消）→ 不记。
	require.False(t, emptyUpstreamResponseIsError(&relaycommon.RelayInfo{
		StreamStatus: &relaycommon.StreamStatus{EndReason: relaycommon.StreamEndReasonClientGone},
	}))
}
