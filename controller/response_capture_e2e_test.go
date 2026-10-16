package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 端到端验证：**真实 gin ResponseWriter** → tee → 提取 → 采集信号。
//
// 为什么不用 mock：这层代码的价值恰恰在于「它插在真实响应写出路径上」。
// mock 一个 writer 只能证明「我假设的调用顺序成立」，而真实 gin 的
// Write/WriteHeader/WriteString/Flush 行为（尤其流式下的分块写出）
// 才是需要被验证的对象 —— 这也正是本项目反复强调的
//「测了函数 ≠ 测了链路」。

// countingWriterE2E 复刻 relay.go 安装包装的那一段，
// 但用**真实 gin.Engine** 驱动，而不是手写假 writer。
func countingWriterE2E(t *testing.T, handler gin.HandlerFunc) (*responseCapture, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	var capture *responseCapture
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		// 与 controller/relay.go 完全相同的安装方式。
		capture = newResponseCapture()
		c.Writer = &countingResponseWriter{
			ResponseWriter: c.Writer,
			bytes:          new(int64),
			capture:        capture,
		}
		handler(c)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	engine.ServeHTTP(rec, req)
	return capture, rec
}

func TestE2ECaptureThroughRealGinWriter(t *testing.T) {
	t.Run("非流式：c.JSON 写出的响应能被提取", func(t *testing.T) {
		capture, rec := countingWriterE2E(t, func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"id": "chatcmpl-1",
				"choices": []gin.H{{
					"index":         0,
					"message":       gin.H{"role": "assistant", "content": "这是真实 gin 写出的回答"},
					"finish_reason": "stop",
				}},
			})
		})

		require.NotNil(t, capture)
		got := extractAssistantText(capture.bytes())
		assert.Equal(t, "这是真实 gin 写出的回答", got,
			"必须能从真实 gin.JSON 的字节流里提取出回复")
		// 同时确认 tee 没有破坏下游响应。
		assert.Contains(t, rec.Body.String(), "这是真实 gin 写出的回答")
	})

	t.Run("流式：多次 Write + Flush 的 SSE 能被拼接", func(t *testing.T) {
		// 模拟 relay 流式路径：多次写 data: 块并 Flush。
		chunks := []string{
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\"流式\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\"回答\"}}]}\n\n",
			"data: [DONE]\n\n",
		}
		capture, rec := countingWriterE2E(t, func(c *gin.Context) {
			c.Header("Content-Type", "text/event-stream")
			c.Status(http.StatusOK)
			c.Writer.Flush()
			for _, ch := range chunks {
				_, _ = c.Writer.WriteString(ch)
				c.Writer.Flush()
			}
		})

		require.NotNil(t, capture)
		got := extractAssistantText(capture.bytes())
		assert.Equal(t, "流式回答", got,
			"多次 Write 的分块必须被拼接，而不是只取一块")
		// 下游响应必须完整（tee 不得吞字节）。
		for _, ch := range chunks {
			assert.Contains(t, rec.Body.String(), ch)
		}
	})

	t.Run("写客户端失败时仍拿到已产出的文本", func(t *testing.T) {
		// 客户端断连是真实场景。tee 在写之前发生，
		// 所以即使写失败，模型已产出的内容仍应被采集到。
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		var capture *responseCapture
		engine.POST("/x", func(c *gin.Context) {
			capture = newResponseCapture()
			c.Writer = &countingResponseWriter{
				ResponseWriter: &failingWriter{ResponseWriter: c.Writer},
				bytes:          new(int64),
				capture:        capture,
			}
			_, _ = c.Writer.WriteString(
				"data: {\"choices\":[{\"delta\":{\"content\":\"断连前已产出\"}}]}\n\n")
		})
		engine.ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequest(http.MethodPost, "/x", nil))

		require.NotNil(t, capture)
		assert.Equal(t, "断连前已产出", extractAssistantText(capture.bytes()),
			"写客户端失败也必须保住已产出的文本（tee 必须先于写）")
	})
}

// failingWriter 模拟「连接已断开」的 writer。
type failingWriter struct {
	gin.ResponseWriter
}

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, http.ErrHandlerTimeout
}

func (w *failingWriter) WriteString(string) (int, error) {
	return 0, http.ErrHandlerTimeout
}

// 并发写入（多个 goroutine 同时给同一 capture 追加）不得丢内容或竞态。
func TestResponseCaptureConcurrentWrites(t *testing.T) {
	c := newResponseCapture()
	var wg sync.WaitGroup
	const writers, perWriter = 8, 200

	for i := range writers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			payload := []byte(`data: {"choices":[{"delta":{"content":"x"}}]}` + "\n")
			for range perWriter {
				c.append(payload)
			}
		}(i)
	}
	wg.Wait()

	// -race 下不得报竞态；且内容量应符合预期（未超上限时）。
	assert.NotEmpty(t, c.bytes())
	if !c.truncated {
		assert.Equal(t, writers*perWriter*len(`data: {"choices":[{"delta":{"content":"x"}}]}`+"\n"),
			len(c.bytes()))
	}
}

// 提取器对**真实字节形态**的兼容性：gin.JSON 会做 HTML 转义，
// 文本里含 <>& 时不得影响提取。
func TestExtractHandlesGinEscapedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.JSON(http.StatusOK, gin.H{
		"choices": []gin.H{{
			"message": gin.H{"content": "比较 3 < 5 && 4 > 2"},
		}},
	})
	// gin 默认开启 HTML 转义 → 字节流里是 < 等。
	var tmp bytes.Buffer
	tmp.Write(rec.Body.Bytes())
	require.True(t, json.Valid(tmp.Bytes()))

	got := extractAssistantText(tmp.Bytes())
	assert.Equal(t, "比较 3 < 5 && 4 > 2", got,
		"HTML 转义后的 JSON 仍须能被正确解析回原文")
}
