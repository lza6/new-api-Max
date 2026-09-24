package relay

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/lza6/new-api-Max/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripClaudePassThroughReasoningEffort(t *testing.T) {
	t.Run("removes reasoning_effort top-level", func(t *testing.T) {
		body := `{"model":"claude-opus","reasoning_effort":"high","max_tokens":10}`
		out := stripClaudePassThroughReasoningEffort(nil, bytes.NewBufferString(body))
		raw, err := io.ReadAll(out)
		require.NoError(t, err)
		var obj map[string]any
		require.NoError(t, common.Unmarshal(raw, &obj))
		_, has := obj["reasoning_effort"]
		assert.False(t, has, "reasoning_effort 必须被剔除")
		assert.Equal(t, "claude-opus", obj["model"])
		assert.Equal(t, float64(10), obj["max_tokens"])
	})

	t.Run("removes camelCase ReasoningEffort", func(t *testing.T) {
		body := `{"ReasoningEffort":"medium","model":"x"}`
		out := stripClaudePassThroughReasoningEffort(nil, bytes.NewBufferString(body))
		raw, _ := io.ReadAll(out)
		var obj map[string]any
		require.NoError(t, common.Unmarshal(raw, &obj))
		_, has := obj["ReasoningEffort"]
		assert.False(t, has)
	})

	t.Run("non-json passes through untouched", func(t *testing.T) {
		body := `not json at all`
		out := stripClaudePassThroughReasoningEffort(nil, bytes.NewBufferString(body))
		raw, _ := io.ReadAll(out)
		assert.Equal(t, body, string(raw))
	})

	t.Run("no reasoning key stays byte-identical", func(t *testing.T) {
		body := `{"model":"claude-3-5-sonnet","thinking":{"type":"enabled","budget_tokens":2000}}`
		out := stripClaudePassThroughReasoningEffort(nil, bytes.NewBufferString(body))
		raw, _ := io.ReadAll(out)
		// JSON 重编码可能调整空格；语义等价即可。
		var a, b map[string]any
		require.NoError(t, common.Unmarshal([]byte(body), &a))
		require.NoError(t, common.Unmarshal(raw, &b))
		assert.Equal(t, a, b)
	})
}

// TestSanitizeOpenAIPassThroughReasoningEffort 生产回归：OpenAI 透传路径收到
// reasoning_effort=on（客户端通用"开启思考"表达）时必须归一/剔除，避免上游 400
// "field ReasoningEffort invalid"（channel 38 第三方中转实锤）。
func TestSanitizeOpenAIPassThroughReasoningEffort(t *testing.T) {
	cases := []struct {
		body     string
		wantJSON string
	}{
		{`{"model":"x","reasoning_effort":"on"}`, `{"model":"x"}`},
		{`{"model":"x","reasoning_effort":"true"}`, `{"model":"x"}`},
		{`{"model":"x","reasoning_effort":"off"}`, `{"model":"x","reasoning_effort":"none"}`},
		{`{"model":"x","reasoning_effort":"high"}`, `{"model":"x","reasoning_effort":"high"}`},
		{`{"model":"x","ReasoningEffort":"on"}`, `{"model":"x"}`},
		// v1.3.21 生产回归：非 string 类型（SDK 常用 bool/数字）此前被透传，
		// channel 38 上游 400 "field ReasoningEffort invalid" 实锤。
		{`{"model":"x","reasoning_effort":true}`, `{"model":"x"}`},
		{`{"model":"x","reasoning_effort":false}`, `{"model":"x","reasoning_effort":"none"}`},
		{`{"model":"x","reasoning_effort":1}`, `{"model":"x"}`},
		{`{"model":"x","reasoning_effort":0}`, `{"model":"x","reasoning_effort":"none"}`},
		// minimal/max 是通用枚举但 channel 38 只认 low/medium/high/xhigh/none；
		// 透传=镜像给单一上游，剔除避免 400，上游用默认。
		{`{"model":"x","reasoning_effort":"minimal"}`, `{"model":"x"}`},
		{`{"model":"x","reasoning_effort":"max"}`, `{"model":"x"}`},
		{`not-json`, `not-json`},
	}
	for _, tc := range cases {
		out := sanitizeOpenAIPassThroughReasoningEffort(strings.NewReader(tc.body), relayconstant.RelayModeChatCompletions)
		raw, _ := io.ReadAll(out)
		got := string(raw)
		var gotObj, wantObj map[string]any
		if common.Unmarshal([]byte(got), &gotObj) == nil && common.Unmarshal([]byte(tc.wantJSON), &wantObj) == nil {
			if !assert.Equal(t, wantObj, gotObj, "body %q", tc.body) {
				continue
			}
		} else if got != tc.wantJSON {
			t.Errorf("body %q: got %q, want %q", tc.body, got, tc.wantJSON)
		}
	}
}
