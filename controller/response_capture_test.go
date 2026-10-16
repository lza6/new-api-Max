package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 响应体 → 助手回复 的提取测试。
//
// ## 这是「数据能否用于 SFT」的关键一环
//
// 提取失败不会报错，只会让记录里少一条 assistant —— 从测试全绿的角度
// 完全看不出来，但数据就没法用于微调了。所以各家格式都要有用例钉住。

// ---------- 非流式 JSON ----------

func TestExtractOpenAINonStream(t *testing.T) {
	raw := `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"你好，这是回答"},"finish_reason":"stop"}],"usage":{}}`
	assert.Equal(t, "你好，这是回答", extractAssistantText([]byte(raw)))
}

func TestExtractOpenAIStreamConcatenatesDeltas(t *testing.T) {
	// 流式：多块 delta 必须**按序拼接**，而不是只取第一块或最后一块。
	raw := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"这是"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"一段"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"流式回复"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	assert.Equal(t, "这是一段流式回复", extractAssistantText([]byte(raw)))
}

func TestExtractClaudeStreamDeltaText(t *testing.T) {
	raw := strings.Join([]string{
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Claude "}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"流式片段"}}`,
		``,
	}, "\n")
	assert.Equal(t, "Claude 流式片段", extractAssistantText([]byte(raw)))
}

func TestExtractClaudeNonStreamContentArray(t *testing.T) {
	raw := `{"id":"msg_1","content":[{"type":"text","text":"第一段"},{"type":"text","text":"第二段"}],"role":"assistant"}`
	assert.Equal(t, "第一段第二段", extractAssistantText([]byte(raw)))
}

func TestExtractGeminiNonStream(t *testing.T) {
	raw := `{"candidates":[{"content":{"parts":[{"text":"Gemini 回答"}],"role":"model"}}]}`
	assert.Equal(t, "Gemini 回答", extractAssistantText([]byte(raw)))
}

func TestExtractGeminiStream(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"Gemini "}]}}]}`,
		``,
		`data: {"candidates":[{"content":{"parts":[{"text":"流式"}]}}]}`,
		``,
	}, "\n")
	assert.Equal(t, "Gemini 流式", extractAssistantText([]byte(raw)))
}

// ---------- 必须**不**产出文本的情形（宁可少采，不可污染） ----------

func TestExtractReturnsEmptyForNonConversationPayloads(t *testing.T) {
	// 把 SSE 协议帧当训练正文存进数据集是污染，比少采更糟。
	cases := []struct {
		name string
		raw  string
	}{
		{"空输入", ""},
		{"纯空白", "   \n  "},
		{"只有 DONE", "data: [DONE]"},
		{"只有角色帧没有内容", `data: {"choices":[{"delta":{"role":"assistant"}}]}`},
		{"非 JSON", "这是一段普通文本，不是 JSON"},
		{"JSON 但没有对话字段", `{"usage":{"prompt_tokens":10}}`},
		{"数组而非对象", `[1,2,3]`},
		{"错误响应", `{"error":{"message":"upstream failed"}}`},
		{"content 为空字符串", `{"choices":[{"message":{"content":""}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, extractAssistantText([]byte(tc.raw)),
				"不该从该载荷里提取出正文（否则会污染训练数据）")
		})
	}
}

func TestExtractHandlesMalformedInputWithoutPanicking(t *testing.T) {
	// 对抗视角：所有 API 响应都可能是畸形的。任何输入都不得 panic。
	inputs := []string{
		`{"choices":`,
		`{"choices":null}`,
		`{"choices":[]}`,
		`{"choices":[null]}`,
		`{"choices":[{"message":null}]}`,
		`{"choices":[{"message":{"content":123}}]}`,
		`{"choices":[{"message":{"content":[]}}]}`,
		`{"choices":[{"message":{"content":{"nested":"obj"}}}]}`,
		`data: {"choices":[{"delta":{"content":null}}]}`,
		`data: not-json-at-all`,
		`data:`,
		`{"content":null}`,
		`{"content":[{"text":null}]}`,
		`{"candidates":null}`,
		`{"candidates":[{"content":null}]}`,
		strings.Repeat("data: {", 100),
	}
	for i, in := range inputs {
		assert.NotPanics(t, func() {
			_ = extractAssistantText([]byte(in))
		}, "第 %d 个畸形输入触发了 panic：%q", i, in)
	}
}

func TestExtractTruncatedSSEStillYieldsPartialText(t *testing.T) {
	// 响应体超限被截断时，末尾可能留下半行 JSON。
	// 要求：**不 panic**，且尽量取出可解析的部分（而不是整段丢弃）。
	raw := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"前半段"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"后半`,
	}, "\n")
	got := extractAssistantText([]byte(raw))
	assert.Contains(t, got, "前半段", "可解析的部分应被取出")
	assert.NotPanics(t, func() { _ = extractAssistantText([]byte(raw)) })
}

// ---------- 有界捕获 ----------

func TestResponseCaptureIsBounded(t *testing.T) {
	// 无界捕获会让一个超长响应（或恶意上游持续吐字节）吃掉内存。
	c := newResponseCapture()

	// 灌入远超上限的数据。
	chunk := []byte(strings.Repeat("x", 64<<10))
	for range (maxCapturedResponseBytes / len(chunk)) + 5 {
		c.append(chunk)
	}

	got := c.bytes()
	assert.LessOrEqual(t, len(got), maxCapturedResponseBytes,
		"捕获必须有硬上限")
	assert.NotEmpty(t, got, "上限内应捕获到内容")
	assert.True(t, c.truncated, "超限后必须标记 truncated")
}

func TestResponseCaptureNilSafe(t *testing.T) {
	var c *responseCapture
	assert.NotPanics(t, func() { c.append([]byte("x")) })
	assert.Nil(t, c.bytes())
	assert.NotPanics(t, func() {
		w := &countingResponseWriter{capture: nil}
		_ = w // capture 为 nil 时不捕获（采集未开启的部署零代价）
	})
}

func TestResponseCaptureReturnsCopy(t *testing.T) {
	// 返回内部切片会让调用方在后续 append 时看到被改写的"旧"数据。
	c := newResponseCapture()
	c.append([]byte("第一次"))
	first := c.bytes()
	c.append([]byte("第二次"))

	assert.Equal(t, "第一次", string(first), "先前取出的内容不得被后续写入影响")
	assert.Equal(t, "第一次第二次", string(c.bytes()))
}
