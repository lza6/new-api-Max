package relay

import (
	"io"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
)

// stripClaudePassThroughReasoningEffort 从 Claude 直通请求体中剔除顶层
// `reasoning_effort`（及畸形的 `ReasoningEffort`）键。直通路径把客户端原始
// body 字节级转发给 /v1/messages；某些 Claude 兼容上游（Anthropic 风格校验）
// 会因该 OpenAI 惯用键 400 "field ReasoningEffort invalid"。此处只在直通场景
// 最小改动：解析失败/非 JSON 时原样透传（保持 fail-open）。
func stripClaudePassThroughReasoningEffort(c *gin.Context, body io.Reader) io.Reader {
	raw, err := io.ReadAll(body)
	if err != nil {
		return body
	}
	var obj map[string]any
	if err := common.Unmarshal(raw, &obj); err != nil || obj == nil {
		// 非 JSON 或解析失败：原样放回。
		return newBytesReader(raw)
	}
	delete(obj, "reasoning_effort")
	delete(obj, "ReasoningEffort")
	cleaned, err := common.Marshal(obj)
	if err != nil {
		return newBytesReader(raw)
	}
	return newBytesReader(cleaned)
}

// newBytesReader 包 []byte 为可重复读取的 Reader（供请求体消费）。
func newBytesReader(b []byte) io.Reader {
	return &bytesReader{b: b}
}

type bytesReader struct {
	b []byte
	n int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.n >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.n:])
	r.n += n
	return n, nil
}
