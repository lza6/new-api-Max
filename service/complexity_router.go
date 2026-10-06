package service

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/lza6/new-api-Max/common"
)

// T4 复杂度路由（规则版 7 维打分）。
//
// 灵感来自 litellm 的 complexity router：不调用外部模型，仅用**本地规则**对请求
// 内容打 7 个维度的分，求和映射到 tier（simple/medium/complex）。目标是 <1ms、
// 可解释（每维得分与理由都可回显）、零外部依赖。
//
// 7 维度（各自归一化到 0..1，加权求和）：
//  1. length        —— 输入字符量（prompt 越长通常越难）
//  2. code          —— 代码/结构化信号（``` 围栏、缩进、常见关键字）
//  3. math          —— 数学信号（公式、运算符密度、数字密度）
//  4. reasoning     —— 推理信号（"why/explain/prove/step by step/证明/推理"）
//  5. tools         —— 工具调用信号（tools 数组非空、tool_choice）
//  6. multimodal    —— 多模态信号（含图片/音频 content part）
//  7. multi_turn    —— 多轮对话深度（消息条数）
//
// 打分只读请求的**文本片段**，不解析完整 JSON（调用方传入已抽取的摘要）。

// ComplexityTier 复杂度档位。
type ComplexityTier string

const (
	ComplexityTierSimple  ComplexityTier = "simple"
	ComplexityTierMedium  ComplexityTier = "medium"
	ComplexityTierComplex ComplexityTier = "complex"
)

// ComplexitySignals 调用方抽取的请求信号（只读文本，避免解析完整 JSON）。
type ComplexitySignals struct {
	PromptChars   int  // 输入总字符数（system+user+assistant 拼接）
	MessageCount  int  // 消息条数
	HasCodeFence  bool // 含 ``` 围栏
	HasCodeKw     bool // 含代码关键字
	HasMathExpr   bool // 含数学表达式信号
	HasReasoning  bool // 含推理关键词
	LooksCode     bool // 缩进/括号密度像代码
	HasTools      bool // 请求带 tools
	HasToolChoice bool // 请求带 tool_choice
	HasMultiModal bool // content 含非文本 part
}

// ComplexityScore 各维度得分（0..1）+ 加权总分 + 理由。
type ComplexityScore struct {
	Length     float64        `json:"length"`
	Code       float64        `json:"code"`
	Math       float64        `json:"math"`
	Reasoning  float64        `json:"reasoning"`
	Tools      float64        `json:"tools"`
	MultiModal float64        `json:"multimodal"`
	MultiTurn  float64        `json:"multi_turn"`
	Total      float64        `json:"total"` // 0..100
	Tier       ComplexityTier `json:"tier"`
	Reasons    []string       `json:"reasons,omitempty"`
}

// 维度权重（和=1.0）。长度与推理权重最高（最影响成本/质量）。
const (
	cxWeightLength     = 0.30
	cxWeightCode       = 0.15
	cxWeightMath       = 0.10
	cxWeightReasoning  = 0.20
	cxWeightTools      = 0.10
	cxWeightMultiModal = 0.08
	cxWeightMultiTurn  = 0.07
)

// 档位阈值（总分 0..100）。
const (
	cxMediumThreshold  = 35.0
	cxComplexThreshold = 65.0
)

// ComplexityRoutingEnabled 全局开关：COMPLEXITY_ROUTING=on|true 开启。默认关。
var complexityRoutingEnabled = common.GetEnvOrDefaultBool("COMPLEXITY_ROUTING", false)

// SetComplexityRoutingEnabled 测试用覆盖；nil 恢复 env 默认。
func SetComplexityRoutingEnabled(v *bool) {
	if v == nil {
		complexityRoutingEnabled = common.GetEnvOrDefaultBool("COMPLEXITY_ROUTING", false)
		return
	}
	complexityRoutingEnabled = *v
}

// ComplexityRoutingEnabled 报告复杂度路由开关状态。
func ComplexityRoutingEnabled() bool { return complexityRoutingEnabled }

// ScoreComplexity 对请求信号打 7 维分并映射 tier（纯函数，无副作用，<1ms）。
func ScoreComplexity(s ComplexitySignals) ComplexityScore {
	sc := ComplexityScore{Reasons: []string{}}

	// 1. length：200 字符→~0.1，2000→~0.5，10000→~1.0（对数缩放，避免长 prompt 秒杀）。
	sc.Length = cxClamp01(logScale(float64(s.PromptChars), 200, 10000))
	if s.PromptChars > 2000 {
		sc.Reasons = append(sc.Reasons, "long_input")
	}

	// 2. code：围栏/关键字/缩进像代码。
	codeSignals := 0
	if s.HasCodeFence {
		codeSignals += 2
	}
	if s.HasCodeKw {
		codeSignals++
	}
	if s.LooksCode {
		codeSignals++
	}
	sc.Code = cxClamp01(float64(codeSignals) / 4.0)
	if s.HasCodeFence || s.HasCodeKw {
		sc.Reasons = append(sc.Reasons, "code")
	}

	// 3. math。
	if s.HasMathExpr {
		sc.Math = 1.0
		sc.Reasons = append(sc.Reasons, "math")
	}

	// 4. reasoning。
	if s.HasReasoning {
		sc.Reasoning = 1.0
		sc.Reasons = append(sc.Reasons, "reasoning")
	}

	// 5. tools。
	if s.HasToolChoice {
		sc.Tools = 1.0
		sc.Reasons = append(sc.Reasons, "tool_choice")
	} else if s.HasTools {
		sc.Tools = 0.6
		sc.Reasons = append(sc.Reasons, "tools")
	}

	// 6. multimodal。
	if s.HasMultiModal {
		sc.MultiModal = 1.0
		sc.Reasons = append(sc.Reasons, "multimodal")
	}

	// 7. multi_turn：1 条→0，10 条→1.0。
	sc.MultiTurn = cxClamp01(float64(max(s.MessageCount-1, 0)) / 9.0)
	if s.MessageCount >= 6 {
		sc.Reasons = append(sc.Reasons, "multi_turn")
	}

	sc.Total = (sc.Length*cxWeightLength +
		sc.Code*cxWeightCode +
		sc.Math*cxWeightMath +
		sc.Reasoning*cxWeightReasoning +
		sc.Tools*cxWeightTools +
		sc.MultiModal*cxWeightMultiModal +
		sc.MultiTurn*cxWeightMultiTurn) * 100.0

	switch {
	case sc.Total >= cxComplexThreshold:
		sc.Tier = ComplexityTierComplex
	case sc.Total >= cxMediumThreshold:
		sc.Tier = ComplexityTierMedium
	default:
		sc.Tier = ComplexityTierSimple
	}
	return sc
}

// cxClamp01 把 v 限制到 [0,1]。
func cxClamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// logScale 把 [lo,hi] 对数映射到 [0,1]；v<=lo→0，v>=hi→1。
func logScale(v, lo, hi float64) float64 {
	if v <= lo {
		return 0
	}
	if v >= hi {
		return 1
	}
	if lo <= 0 {
		return 0
	}
	// ln(v/lo) / ln(hi/lo)
	num := logRatio(v / lo)
	den := logRatio(hi / lo)
	if den == 0 {
		return 0
	}
	return num / den
}

// logRatio 自然对数（x<=0 返回 0）。
func logRatio(x float64) float64 {
	if x <= 0 {
		return 0
	}
	return math.Log(x)
}

// ExtractComplexitySignals 从文本片段集合抽取复杂度信号（不解析完整 JSON）。
// prompts 为各消息的文本内容（已由调用方抽取）；其余布尔信号由调用方从
// 请求结构判定后传入。此函数只做文本级启发式（长度/代码/数学/推理）。
func ExtractComplexitySignals(prompts []string, signals ComplexitySignals) ComplexitySignals {
	var sb strings.Builder
	for _, p := range prompts {
		sb.WriteString(p)
	}
	text := sb.String()
	signals.PromptChars = utf8.RuneCountInString(text)
	lower := strings.ToLower(text)

	if strings.Contains(text, "```") || strings.Contains(text, "~~~") {
		signals.HasCodeFence = true
	}
	if containsAny(lower, cxCodeKeywords) {
		signals.HasCodeKw = true
	}
	if containsAny(lower, cxMathKeywords) {
		signals.HasMathExpr = true
	}
	if containsAny(lower, cxReasoningKeywords) {
		signals.HasReasoning = true
	}
	if looksLikeCode(text) {
		signals.LooksCode = true
	}
	return signals
}

var cxCodeKeywords = []string{
	"function ", "func ", "def ", "class ", "import ", "package ",
	"select ", "from ", "where ", "=>", "public ", "private ",
	"return ", "const ", "var ", "async ", "await ",
}

var cxMathKeywords = []string{
	"计算", "求", "方程", "积分", "导数", "概率", "矩阵", "证明",
	"calculate", "equation", "integral", "derivative", "probability",
	"matrix", "solve for", "prove",
}

var cxReasoningKeywords = []string{
	"why", "explain", "reason", "step by step", "analyze", "analyse",
	"compare", "推导", "推理", "分析", "解释", "为什么", "对比", "论证",
	"step-by-step", "in detail",
}

func containsAny(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// looksLikeCode 用缩进/括号密度启发式判断文本像代码。
func looksLikeCode(text string) bool {
	if len(text) < 40 {
		return false
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return false
	}
	indented := 0
	braces := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
			indented++
		}
		braces += strings.Count(line, "{") + strings.Count(line, "}") + strings.Count(line, ";")
	}
	// 超过 1/4 行有缩进，或括号/分号密度高 → 像代码。
	return indented*4 >= len(lines) || braces >= len(lines)
}
