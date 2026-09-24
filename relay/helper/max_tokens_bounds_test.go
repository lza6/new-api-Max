package helper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/stretchr/testify/require"
)

// TestMaxTokensBounds guards the billing invariant that user-supplied max
// token fields are bounded on every relay format. These values feed
// pre-consume quota math (preConsumedTokens * ratio); a huge or
// wrapped-negative value (e.g. 18446744073686646784 parsed into *uint) must
// be rejected at validation instead of corrupting the pre-charge.
func TestMaxTokensBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newJSONContext := func(t *testing.T, body string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/relay", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		return c
	}

	const hugeN = "18446744073686646784"

	t.Run("openai max_tokens overflow rejected", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_tokens":`+hugeN+`}`)
		_, err := GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("openai max_completion_tokens overflow rejected", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":`+hugeN+`}`)
		_, err := GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("claude max_tokens overflow rejected", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hi"}],"max_tokens":`+hugeN+`}`)
		_, err := GetAndValidateClaudeRequest(c)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("claude normal max_tokens accepted", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hi"}],"max_tokens":8192}`)
		req, err := GetAndValidateClaudeRequest(c)
		require.NoError(t, err)
		require.EqualValues(t, 8192, *req.MaxTokens)
	})

	t.Run("gemini maxOutputTokens overflow rejected", func(t *testing.T) {
		c := newJSONContext(t, `{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":`+hugeN+`}}`)
		_, err := GetAndValidateGeminiRequest(c)
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxOutputTokens is invalid")
	})

	t.Run("responses max_output_tokens overflow rejected", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"gpt-4o","input":"hi","max_output_tokens":`+hugeN+`}`)
		_, err := GetAndValidateResponsesRequest(c)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_output_tokens is invalid")
	})
}

// TestReasoningEffortNonStringNormalized 生产回归：reasoning_effort 为 bool/number
// 时，Go 强类型 string 字段会 unmarshal 失败 400（channel 38 实锤
// "json: cannot unmarshal bool into Go struct field ...reasoning_effort"）。
// 解析入口必须先归一非 string 形态，避免请求在反序列化阶段就失败。
func TestReasoningEffortNonStringNormalized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newJSONContext := func(t *testing.T, body string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/relay", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		return c
	}

	t.Run("bool true normalized away", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"reasoning_effort":true}`)
		req, err := GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.NoError(t, err)
		require.Equal(t, "", req.ReasoningEffort)
	})

	t.Run("bool false normalized to none", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"reasoning_effort":false}`)
		req, err := GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.NoError(t, err)
		require.Equal(t, "none", req.ReasoningEffort)
	})

	t.Run("number 1 normalized away", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"reasoning_effort":1}`)
		req, err := GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.NoError(t, err)
		require.Equal(t, "", req.ReasoningEffort)
	})

	t.Run("string on still passes sanitize later", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"on"}`)
		req, err := GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.NoError(t, err)
		require.Equal(t, "on", req.ReasoningEffort)
	})

	t.Run("camelCase bool normalized", func(t *testing.T) {
		c := newJSONContext(t, `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"ReasoningEffort":true}`)
		req, err := GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.NoError(t, err)
		require.Equal(t, "", req.ReasoningEffort)
	})
}
