package controller

import (
	"strings"
	"sync"

	"github.com/lza6/new-api-Max/common"
)

// 响应体捕获：为「训练数据采集」补上**助手回复**。
//
// ## 为什么需要它（这是本功能最大的功能缺口）
//
// 采集原先只拿到请求侧文本（`TokenCountMeta.CombineText`），
// 云盘记录里**只有用户侧、没有模型回复**。后果：
//   - 数据**不能直接做 SFT/微调**（没有训练目标）；
//   - 只能当继续预训练语料用。
//
// 修复的关键是找到**一个收口点**。`controller/relay.go` 已经用
// `countingResponseWriter` 包了 `c.Writer`（它原本只数字节数）——
// 那个包装点覆盖**所有** relay 格式。在这里顺手 tee 一份响应体，
// 就能一次性拿到全部格式的回复正文，而不需要分别改 12+ 个 adaptor。
//
// ## 为什么不在 adaptor 层做
//
// 在 adaptor 层做意味着每个格式（OpenAI / Claude / Gemini / Responses…）
// 都要各写一遍，且新增格式时必须记得实现 —— 漏了就静默少采。
// 包在 ResponseWriter 上则天然覆盖全部，包括未来新增的格式。
//
// ## 代价与边界（诚实说明）
//
//   - **有界**：最多捕获 `maxCapturedResponseBytes`。超出部分丢弃，
//     提取出的文本会偏短（`Truncated` 会体现在长度上）。
//     这是刻意的取舍：回复正文用于训练时本身就是有损样本，
//     而为了「不丢一个字」把无限长的 SSE 全缓存进内存会危及主业务。
//   - **只在采集开启时捕获**：未开启时 `capture` 为 nil，
//     `Write` 里只多一次 nil 判断，不产生任何复制开销。
//   - 捕获的是**已经写给客户端**的字节（tee），不改动响应内容与时机。

// maxCapturedResponseBytes 单个响应最多捕获的字节数。
//
// 取 256KB：一份完整回复通常远小于此（即使是长文分析）。
// 超出的部分是「超长回复的尾部」，丢弃它不影响「这段对话聊了什么」的判断。
const maxCapturedResponseBytes = 256 << 10

// responseCapture 有界响应捕获器。并发安全（Write 可能被多个 goroutine 调用）。
type responseCapture struct {
	mu        sync.Mutex
	buf       []byte
	truncated bool
}

func newResponseCapture() *responseCapture {
	return &responseCapture{buf: make([]byte, 0, 8<<10)}
}

// append 追加一段响应字节；超出上限后只记录「已截断」，不再增长。
func (c *responseCapture) append(data []byte) {
	if c == nil || len(data) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	remaining := maxCapturedResponseBytes - len(c.buf)
	if remaining <= 0 {
		c.truncated = true
		return
	}
	if len(data) > remaining {
		c.buf = append(c.buf, data[:remaining]...)
		c.truncated = true
		return
	}
	c.buf = append(c.buf, data...)
}

// bytes 返回已捕获的副本（调用方不应假定它可长期持有）。
func (c *responseCapture) bytes() []byte {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]byte, len(c.buf))
	copy(out, c.buf)
	return out
}

// extractAssistantText 从**已写给客户端**的响应字节里提取助手回复正文。
//
// 需要同时处理两类形态（它们在各家渠道上都会出现）：
//
//  1. **非流式**：一整块 JSON。
//     OpenAI `choices[0].message.content`、Claude `content[].text`、
//     Gemini `candidates[].content.parts[].text`。
//  2. **流式（SSE）**：多行 `data: {...}`，需要把各块的增量**拼接**起来。
//     OpenAI `choices[].delta.content`、Claude `delta.text`、
//     Gemini `candidates[].content.parts[].text`。
//
// 返回值可能为空（格式不认识、只有角色帧、纯工具调用等）。
// **不猜测**：不认识就不返回文本，而不是把原始字节当正文 ——
// 把 SSE 协议帧当训练文本存进数据集是污染，比少采更糟。
func extractAssistantText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}

	// 流式判定：SSE 以 `data:` 行组织。用「是否出现行首 data:」判断，
	// 而不是只看有没有换行（非流式 JSON 也可能被 pretty-print）。
	if looksLikeSSE(text) {
		return extractFromSSE(text)
	}
	return extractFromJSONObject([]byte(text))
}

func looksLikeSSE(s string) bool {
	return strings.HasPrefix(s, "data:") ||
		strings.Contains(s, "\ndata:") ||
		strings.HasPrefix(s, "event:")
}

// extractFromSSE 拼接 SSE 各块的增量文本。
func extractFromSSE(s string) string {
	var sb strings.Builder
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		// 每块都尝试两类形态；命中即追加。
		if piece := extractFromJSONObject([]byte(payload)); piece != "" {
			sb.WriteString(piece)
		}
	}
	return sb.String()
}

// extractFromJSONObject 从一个 JSON 对象里尝试各家的回复字段。
//
// 用 map 而非逐家 struct：这里的输入是**别人给的字节**，
// 结构可能缺字段、类型可能不对（如 content 是数组而非字符串），
// 逐字段做类型断言比定义一堆可能不匹配的 struct 更稳。
func extractFromJSONObject(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var root map[string]any
	if err := common.Unmarshal(raw, &root); err != nil {
		return ""
	}
	if root == nil {
		return ""
	}

	// --- OpenAI 形态：choices[].message.content 或 choices[].delta.content ---
	if choices, ok := root["choices"].([]any); ok && len(choices) > 0 {
		var sb strings.Builder
		for _, ch := range choices {
			choice, ok := ch.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"delta", "message"} {
				container, ok := choice[key].(map[string]any)
				if !ok {
					continue
				}
				sb.WriteString(textFromContentField(container["content"]))
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}

	// --- Claude 形态：content 数组 [{type:text,text:...}]，流式在 delta.text ---
	if pieces := textFromContentField(root["content"]); pieces != "" {
		return pieces
	}
	if delta, ok := root["delta"].(map[string]any); ok {
		if s, ok := delta["text"].(string); ok && s != "" {
			return s
		}
	}
	// --- OpenAI **Responses** 流式形态：{"type":"response.output_text.delta","delta":"..."} ---
	//
	// 这里的 `delta` 是**字符串**，不是上面那种对象 —— 之前只处理了
	// `delta` 为对象的情形，导致 `/v1/responses` 的流式回复**静默不采集**
	// （注释却声称覆盖 OpenAI，那是不准确的）。
	if s, ok := root["delta"].(string); ok && s != "" {
		return s
	}

	// --- OpenAI **Responses** 非流式形态：output[].content[].{type:output_text,text} ---
	//
	// 与 chat/completions 的 `choices[].message.content` 完全不同的一套结构，
	// 必须单独处理，否则整个 `/v1/responses` 端点的回复都会被丢弃。
	if outputs, ok := root["output"].([]any); ok {
		var sb strings.Builder
		for _, o := range outputs {
			item, ok := o.(map[string]any)
			if !ok {
				continue
			}
			// output[] 里既有 message（含 content[]）也有 reasoning 等其它类型；
			// textFromContentField 只认带 text 的条目，不会误取结构。
			sb.WriteString(textFromContentField(item["content"]))
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}

	// --- Gemini 形态：candidates[].content.parts[].text ---
	if cands, ok := root["candidates"].([]any); ok {
		var sb strings.Builder
		for _, c := range cands {
			cand, ok := c.(map[string]any)
			if !ok {
				continue
			}
			content, ok := cand["content"].(map[string]any)
			if !ok {
				continue
			}
			sb.WriteString(textFromContentField(content["parts"]))
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}

	return ""
}

// textFromContentField 处理 `content` / `parts` 这类字段的多种形态：
// 字符串、`[{text:...}]`、`[{type:"text", text:...}]`。
func textFromContentField(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var sb strings.Builder
		for _, item := range t {
			switch m := item.(type) {
			case string:
				sb.WriteString(m)
			case map[string]any:
				if s, ok := m["text"].(string); ok {
					sb.WriteString(s)
				}
			}
		}
		return sb.String()
	case map[string]any:
		// 少数实现把 parts 包成对象
		if s, ok := t["text"].(string); ok {
			return s
		}
	}
	return ""
}
