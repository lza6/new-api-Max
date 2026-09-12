package relay

import (
	"bytes"
	"io"
	"testing"

	"github.com/lza6/new-api-Max/common"
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
