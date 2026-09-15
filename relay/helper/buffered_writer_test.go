package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstPacketBufferWriter_BuffersUntilCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	w := NewFirstPacketBufferWriter(c.Writer)

	// 缓冲期内：写入只进 buf，不穿透 recorder。
	_, err := w.Write([]byte("data: {\"content\":\"a\"}\n"))
	require.NoError(t, err)
	assert.False(t, w.Committed())
	assert.Equal(t, len("data: {\"content\":\"a\"}\n"), w.BufferedBytes())
	assert.Empty(t, recorder.Body.String(), "commit 前响应体必须为空")

	// Commit：缓冲一次性穿透到 recorder。
	w.Commit()
	assert.True(t, w.Committed())
	assert.Equal(t, "data: {\"content\":\"a\"}\n", recorder.Body.String())
}

func TestFirstPacketBufferWriter_WriteAfterCommitGoesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	w := NewFirstPacketBufferWriter(c.Writer)
	_, _ = w.Write([]byte("first"))
	w.Commit()
	assert.Equal(t, "first", recorder.Body.String())

	// committed 后写入直接穿透。
	_, err := w.Write([]byte("|second"))
	require.NoError(t, err)
	assert.Equal(t, "first|second", recorder.Body.String())
	assert.Zero(t, w.BufferedBytes(), "committed 后 buf 已 reset，不累计")
}

func TestFirstPacketBufferWriter_OverflowPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	w := NewFirstPacketBufferWriter(c.Writer)

	// 接近上限：仍缓冲。
	chunk := make([]byte, maxFirstPacketBufferBytes-4)
	for i := range chunk {
		chunk[i] = 'x'
	}
	_, err := w.Write(chunk)
	require.NoError(t, err)
	assert.False(t, w.Committed(), "未超限不应提前 commit")

	// 超过上限：整体放行到 recorder（防御异常响应占内存）。
	overflow := []byte("OVERFLOW")
	_, err = w.Write(overflow)
	require.NoError(t, err)
	assert.True(t, w.Committed(), "超限应 commit 放行")
	assert.Contains(t, recorder.Body.String(), "OVERFLOW")
}

func TestFirstPacketBufferWriter_FlushAndWriteHeaderBeforeCommitAreNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	w := NewFirstPacketBufferWriter(c.Writer)
	// 缓冲期内 Flush / WriteHeaderNow 不提前上线响应头。
	w.Flush()
	w.WriteHeaderNow()
	assert.False(t, w.Committed())
	assert.Empty(t, recorder.Body.String())

	// committed 后 Flush 透传底层。
	w.Commit()
	w.Flush()
	assert.True(t, w.Committed())
}

func TestFirstPacketBufferWriter_SetWriteDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	w := NewFirstPacketBufferWriter(c.Writer)
	err := w.SetWriteDeadline(time.Now().Add(time.Second))
	// 底层无 deadline 支持时返回非 nil（httptest recorder）；不 panic。
	_ = err
}
