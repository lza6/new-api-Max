package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
)

// T5 工具抽屉 / 元函数三段式（kiwi-mem + aci 迁移）。
//
// 核心思路：请求里的 `tools` 定义会随对话轮次反复上传，占用大量 prompt token。
// 「工具抽屉」在**不改变语义**的前提下做两件可省 token 的事：
//  1. 去重：相同（去空白/键序归一后）的工具定义只保留一份（上游按 name 调用，
//     重复定义无意义）。等价变换，零语义风险。
//  2. 元函数三段式（meta-function）：把「未被本轮对话实际引用」的工具折叠为
//     一个轻量索引（name → 简短描述），而非全量 schema。仅当请求显式声明
//     期望按需注入时启用（默认关），避免破坏依赖完整 schema 的客户端。
//
// 本模块**只做分析与变换**，不解析/改写完整 JSON 结构；调用方在既有 DTO 上
// 应用结果。默认全关（TOOL_DRAWER_ENABLED=false），零行为变化。

// ToolDrawerEnabled 全局开关：TOOL_DRAWER_ENABLED=on|true。默认关。
// 值来源：管理端「实验功能」页持久化配置 > env TOOL_DRAWER_ENABLED（默认 false）。
var toolDrawerEnvDefault = common.GetEnvOrDefaultBool(common.FlagToolDrawerEnabled, false)

// SetToolDrawerEnabled 测试用覆盖；nil 恢复 env 默认。
func SetToolDrawerEnabled(v *bool) {
	common.SetFeatureFlagOverride(common.FlagToolDrawerEnabled, common.BoolFeatureFlagOverride(v))
}

// ToolDrawerEnabled 报告工具抽屉开关状态。
func ToolDrawerEnabled() bool {
	return common.FeatureFlagValue(common.FlagToolDrawerEnabled, toolDrawerEnvDefault)
}

// ToolDrawerHeader 是请求级开关的请求头名（优先级高于全局开关）。
const ToolDrawerHeader = "X-NewAPI-Tool-Drawer"

// toolDrawerDedupedRequests / toolDrawerSavedBytes 是工具抽屉的**收益度量**
// （供 /metrics 与「实验功能」页效果度量）。键空间是常量，无无界增长风险。
var (
	toolDrawerDedupedRequests atomic.Int64
	toolDrawerSavedBytes      atomic.Int64
)

// RecordToolDrawerSavings 记录一次工具抽屉去重的实际收益。
//
// savedBytes：被移除的重复工具定义字节量（粗略口径，用于相对比较与趋势观测）。
// 只在真的移除了内容时调用 —— 没省到东西不该计入"节省"。
func RecordToolDrawerSavings(savedBytes int) {
	if savedBytes <= 0 {
		return
	}
	toolDrawerDedupedRequests.Add(1)
	toolDrawerSavedBytes.Add(int64(savedBytes))
}

// ToolDrawerStats 返回工具抽屉的累计收益（进程内）。
type ToolDrawerStats struct {
	DedupedRequests int64 `json:"deduped_requests"`
	SavedBytes      int64 `json:"saved_bytes"`
}

// ToolDrawerSavings 返回收益快照。
func ToolDrawerSavings() ToolDrawerStats {
	return ToolDrawerStats{
		DedupedRequests: toolDrawerDedupedRequests.Load(),
		SavedBytes:      toolDrawerSavedBytes.Load(),
	}
}

// ResetToolDrawerSavingsForTest 清空收益计数（仅测试使用）。
func ResetToolDrawerSavingsForTest() {
	toolDrawerDedupedRequests.Store(0)
	toolDrawerSavedBytes.Store(0)
}

// 为什么需要请求级：全局开关一旦打开会影响**所有**客户端，而依赖完整 tool schema
// 的调用方可能被破坏（本模块原始注释已自认此风险）。把决定权下放到请求头后，
// 「保守的客户端不受影响」与「想省 token 的客户端自助开启」可以同时成立。
type ToolDrawerMode string

const (
	// ToolDrawerModeOff 本次请求完全不做任何变换（逐字节透传）。
	ToolDrawerModeOff ToolDrawerMode = "off"
	// ToolDrawerModeDedupe 只做**等价去重**（移除重复的工具定义）。
	// 零语义风险：上游按 name 调用工具，重复定义没有意义。
	ToolDrawerModeDedupe ToolDrawerMode = "dedupe"
	// ToolDrawerModeMeta 元函数三段式。
	//
	// **本版未实现**：它需要宿主侧拦截「模型回调元函数要 schema」这一轮，
	// 那是独立的一整块能力。声明 meta 时按 dedupe 降级 —— 方向是安全的
	// （语义相同、只是省得更少），并记一次日志，不静默假装做了。
	ToolDrawerModeMeta ToolDrawerMode = "meta"
)

// toolDrawerMetaWarned / toolDrawerUnknownWarned 只告警一次，避免热路径刷日志。
var (
	toolDrawerMetaWarned    atomic.Bool
	toolDrawerUnknownWarned atomic.Bool
)

// ResolveToolDrawerMode 解析本次请求的工具抽屉模式。
//
// 优先级：**请求头 > 全局开关**。未声明请求头时沿用全局开关（缺省与历史行为一致）。
// 未知取值按全局开关处理（fail-safe，不因为一个拼错的头改变行为）。
func ResolveToolDrawerMode(c *gin.Context) ToolDrawerMode {
	globalDefault := ToolDrawerModeOff
	if ToolDrawerEnabled() {
		globalDefault = ToolDrawerModeDedupe
	}

	raw := ""
	if c != nil && c.Request != nil {
		raw = strings.ToLower(strings.TrimSpace(c.Request.Header.Get(ToolDrawerHeader)))
	}

	switch raw {
	case "":
		return globalDefault
	case string(ToolDrawerModeOff):
		return ToolDrawerModeOff
	case string(ToolDrawerModeDedupe):
		return ToolDrawerModeDedupe
	case string(ToolDrawerModeMeta):
		if toolDrawerMetaWarned.CompareAndSwap(false, true) {
			common.SysLog("tool drawer: X-NewAPI-Tool-Drawer=meta is not implemented in this version; " +
				"falling back to the equivalent 'dedupe' transform (same semantics, smaller saving)")
		}
		return ToolDrawerModeDedupe
	default:
		if toolDrawerUnknownWarned.CompareAndSwap(false, true) {
			common.SysLog("tool drawer: ignoring unsupported X-NewAPI-Tool-Drawer value " + raw +
				"; falling back to the global switch")
		}
		return globalDefault
	}
}

// ToolDef 工具定义的抽象视图（供分析与去重）。
type ToolDef struct {
	// Name 工具名（唯一标识，按 name 去重）。
	Name string
	// Fingerprint 归一化后的内容指纹（去空白 + 键序无关的稳定哈希），用于判同。
	Fingerprint string
	// ArgsBytes 参数 schema 的字节量（估算 token 成本用）。
	ArgsBytes int
}

// ToolDrawerAnalysis 工具抽屉分析结果（只读，供观测/报告）。
type ToolDrawerAnalysis struct {
	TotalTools      int `json:"total_tools"`
	UniqueTools     int `json:"unique_tools"`
	DuplicateCount  int `json:"duplicate_count"`  // 重复（同 name 或同指纹）
	SchemaBytes     int `json:"schema_bytes"`     // 全部工具 schema 字节
	DedupedBytes    int `json:"deduped_bytes"`    // 去重后字节
	SavedBytes      int `json:"saved_bytes"`      // 去重节省字节
	EstimatedTokens int `json:"estimated_tokens"` // 粗略估算（bytes/4）
}

// AnalyzeTools 分析工具列表并给出可省 token 的估算（纯函数，无副作用）。
// defs 的 Fingerprint 由调用方用 FingerprintTool 生成。
func AnalyzeTools(defs []ToolDef) ToolDrawerAnalysis {
	a := ToolDrawerAnalysis{TotalTools: len(defs)}
	seenName := make(map[string]struct{}, len(defs))
	seenFP := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		a.SchemaBytes += d.ArgsBytes
		if _, ok := seenName[d.Name]; ok {
			a.DuplicateCount++
			continue
		}
		// 同指纹不同 name 也视为语义重复内容（保留首个）。
		if d.Fingerprint != "" {
			if _, ok := seenFP[d.Fingerprint]; ok {
				a.DuplicateCount++
				continue
			}
			seenFP[d.Fingerprint] = struct{}{}
		}
		seenName[d.Name] = struct{}{}
		a.UniqueTools++
		a.DedupedBytes += d.ArgsBytes
	}
	a.SavedBytes = a.SchemaBytes - a.DedupedBytes
	if a.SavedBytes < 0 {
		a.SavedBytes = 0
	}
	a.EstimatedTokens = a.SchemaBytes / 4
	return a
}

// FingerprintTool 生成工具定义的稳定指纹（键序无关 + 去空白）。
// 传入参数 schema 的归一化文本（调用方从 DTO 提取 name + 参数 JSON）。
// 空指纹（内容为空）返回空串，调用方据此跳过指纹去重。
func FingerprintTool(name, schemaText string) string {
	norm := normalizeToolSchema(schemaText)
	if norm == "" && strings.TrimSpace(name) == "" {
		return ""
	}
	h := sha256.Sum256([]byte(name + "\x00" + norm))
	return hex.EncodeToString(h[:8])
}

// normalizeToolSchema 归一化 schema 文本：**仅在字符串字面量之外**移除空白。
// JSON 语义中结构空白不显著（`{ "a" : 1 }` ≡ `{"a":1}`），但字符串值内的空白
// 是显著的（"a b" ≠ "ab"），故必须保留字符串内部的空白——否则两个语义不同的
// schema 会被误判为相同而错误去重。轻量状态机扫描，不依赖完整 JSON 解析。
func normalizeToolSchema(s string) string {
	if !strings.ContainsAny(s, " \t\n\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false
	for _, r := range s {
		if inString {
			b.WriteRune(r)
			switch {
			case escaped:
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
			b.WriteRune(r)
		case ' ', '\t', '\n', '\r':
			// 结构空白：舍弃。
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DedupToolDefs 返回去重后的工具定义索引（保留首次出现的顺序）。纯函数。
// 调用方据此决定实际保留哪些工具（默认只在开关开启时使用）。
func DedupToolDefs(defs []ToolDef) []int {
	var kept []int
	seenName := make(map[string]struct{}, len(defs))
	seenFP := make(map[string]struct{}, len(defs))
	for i, d := range defs {
		if _, ok := seenName[d.Name]; ok {
			continue
		}
		if d.Fingerprint != "" {
			if _, ok := seenFP[d.Fingerprint]; ok {
				continue
			}
			seenFP[d.Fingerprint] = struct{}{}
		}
		seenName[d.Name] = struct{}{}
		kept = append(kept, i)
	}
	return kept
}

// MetaFunctionIndex 把工具列表折叠为「name → 简短描述」索引（元函数三段式的
// 第一段）。描述取自 schemaText 的前 N 字节（截断到边界）。纯函数。
func MetaFunctionIndex(defs []ToolDef, schemaTexts []string, maxDescBytes int) []map[string]string {
	if maxDescBytes <= 0 {
		maxDescBytes = 120
	}
	out := make([]map[string]string, 0, len(defs))
	kept := DedupToolDefs(defs)
	for _, i := range kept {
		desc := ""
		if i < len(schemaTexts) {
			desc = truncateBytes(schemaTexts[i], maxDescBytes)
		}
		out = append(out, map[string]string{"name": defs[i].Name, "hint": desc})
	}
	return out
}

func truncateBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// 截断到 rune 边界。
	b := []byte(s)[:maxBytes]
	for len(b) > 0 && b[len(b)-1]&0xC0 == 0x80 {
		b = b[:len(b)-1]
	}
	return string(b)
}

// SortedToolNames 返回去重后的工具名有序列表（观测/报告用）。
func SortedToolNames(defs []ToolDef) []string {
	seen := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		if d.Name != "" {
			seen[d.Name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
