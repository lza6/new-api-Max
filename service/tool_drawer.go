package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

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

// toolDrawerEnabled 全局开关：TOOL_DRAWER_ENABLED=on|true。默认关。
var toolDrawerEnabled = common.GetEnvOrDefaultBool("TOOL_DRAWER_ENABLED", false)

// SetToolDrawerEnabled 测试用覆盖；nil 恢复 env 默认。
func SetToolDrawerEnabled(v *bool) {
	if v == nil {
		toolDrawerEnabled = common.GetEnvOrDefaultBool("TOOL_DRAWER_ENABLED", false)
		return
	}
	toolDrawerEnabled = *v
}

// ToolDrawerEnabled 报告工具抽屉开关状态。
func ToolDrawerEnabled() bool { return toolDrawerEnabled }

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
