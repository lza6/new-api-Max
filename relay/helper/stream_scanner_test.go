package helper

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/constant"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/lza6/new-api-Max/setting/relay_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
	if constant.StreamingTimeout == 0 {
		constant.StreamingTimeout = 30
	}
}

func setupStreamTest(t *testing.T, body io.Reader) (*gin.Context, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{
		Body: io.NopCloser(body),
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	return c, resp, info
}

func buildSSEBody(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "data: {\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}\n", i, i)
	}
	b.WriteString("data: [DONE]\n")
	return b.String()
}

// ---------- Basic correctness ----------

func TestStreamScannerHandler_NilInputs(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	StreamScannerHandler(c, nil, info, func(data string, sr *StreamResult) {})
	StreamScannerHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(""))}, info, nil)
}

func TestNewStreamScanner_AllowsLargeStreamLine(t *testing.T) {
	oldBufferMB := constant.StreamScannerMaxBufferMB
	constant.StreamScannerMaxBufferMB = 1
	t.Cleanup(func() {
		constant.StreamScannerMaxBufferMB = oldBufferMB
	})

	payload := strings.Repeat("x", 128<<10)
	scanner := NewStreamScanner(strings.NewReader("data: " + payload + "\n"))
	scanner.Split(bufio.ScanLines)

	require.True(t, scanner.Scan())
	assert.Equal(t, "data: "+payload, scanner.Text())
	require.NoError(t, scanner.Err())
}

func TestStreamScannerHandler_EmptyBody(t *testing.T) {
	t.Parallel()

	c, resp, info := setupStreamTest(t, strings.NewReader(""))

	var called atomic.Bool
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		called.Store(true)
	})

	assert.False(t, called.Load(), "handler should not be called for empty body")
}

func TestStreamScannerHandler_1000Chunks(t *testing.T) {
	t.Parallel()

	const numChunks = 1000
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(numChunks), count.Load())
	assert.Equal(t, numChunks, info.ReceivedResponseCount)
}

func TestStreamScannerHandler_OrderPreserved(t *testing.T) {
	t.Parallel()

	const numChunks = 500
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var mu sync.Mutex
	received := make([]string, 0, numChunks)

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		mu.Lock()
		received = append(received, data)
		mu.Unlock()
	})

	require.Equal(t, numChunks, len(received))
	for i := range numChunks {
		expected := fmt.Sprintf("{\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}", i, i)
		assert.Equal(t, expected, received[i], "chunk %d out of order", i)
	}
}

func TestStreamScannerHandler_DoneStopsScanner(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(50) + "data: should_not_appear\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(50), count.Load(), "data after [DONE] must not be processed")
}

func TestStreamScannerHandler_StopStopsStream(t *testing.T) {
	t.Parallel()

	const numChunks = 200
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	const stopAt int64 = 50
	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= stopAt {
			sr.Stop(fmt.Errorf("fatal at %d", n))
		}
	})

	assert.Equal(t, stopAt, count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
}

func TestStreamScannerHandler_SkipsNonDataLines(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString(": comment line\n")
	b.WriteString("event: message\n")
	b.WriteString("id: 12345\n")
	b.WriteString("retry: 5000\n")
	for i := range 100 {
		fmt.Fprintf(&b, "data: payload_%d\n", i)
		b.WriteString(": interleaved comment\n")
	}
	b.WriteString("data: [DONE]\n")

	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(100), count.Load())
}

func TestStreamScannerHandler_DataWithExtraSpaces(t *testing.T) {
	t.Parallel()

	body := "data:   {\"trimmed\":true}  \ndata: [DONE]\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var got string
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		got = data
	})

	assert.Equal(t, "{\"trimmed\":true}", got)
}

// TestStreamScannerHandler_ClientCancelAbortsUpstreamAndReturns pins the
// disconnect contract: when the client goes away, the handler must return
// promptly (all goroutines joined, so the gin.Context can never leak into a
// pooled reuse), the upstream body must be closed to stop token generation,
// and no data received after the disconnect may be processed or written.
func TestStreamScannerHandler_ClientCancelAbortsUpstreamAndReturns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	firstHandled := make(chan struct{})
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
			_ = StringData(c, data)
			if data == "first" {
				close(firstHandled)
			}
		})
		close(done)
	}()

	_, err := fmt.Fprint(pw, "data: first\n")
	require.NoError(t, err)

	select {
	case <-firstHandled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first chunk")
	}

	cancel()

	// The handler must return without any further upstream input: cleanup
	// closes resp.Body, which unblocks the scanner goroutine.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client disconnect")
	}

	// Upstream read side must be closed so the provider stops generating
	// (and billing) for a request nobody is listening to.
	_, err = fmt.Fprint(pw, "data: second\n")
	require.ErrorIs(t, err, io.ErrClosedPipe, "upstream body should be closed after client disconnect")

	assert.Equal(t, int64(1), count.Load(), "no chunk after disconnect should be processed")
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)

	body := recorder.Body.String()
	assert.Contains(t, body, "first")
	assert.NotContains(t, body, "second")
}

// ---------- Ping tests ----------

func TestStreamScannerHandler_PingSentDuringSlowUpstream(t *testing.T) {
	// T2-3：默认 fallover=on 会禁用 ping；本用例锁 off 路径。
	oldFallover := relay_setting.GetRelaySetting().StreamFallover
	relay_setting.GetRelaySetting().StreamFallover = false
	t.Cleanup(func() { relay_setting.GetRelaySetting().StreamFallover = oldFallover })

	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := range 4 {
			fmt.Fprintf(pw, "data: chunk_%d\n", i)
			time.Sleep(400 * time.Millisecond)
		}
		fmt.Fprint(pw, "data: [DONE]\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream to finish")
	}

	assert.Equal(t, int64(4), count.Load())

	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	// T2-3：stream_fallover 默认已翻转为 on（fallover 会禁 ping 保活）。
	// 本用例显式测 off 路径，确保「fallover off + ping 保活」行为不回退。
	assert.GreaterOrEqual(t, pingCount, 1,
		"expected at least 1 ping during slow stream with 1s interval; got %d", pingCount)
}

func TestStreamScannerHandler_PingDisabledByRelayInfo(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(buildSSEBody(5)))}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	assert.Equal(t, int64(5), count.Load())

	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	assert.Equal(t, 0, pingCount, "pings should be disabled when DisablePing=true")
}

// ---------- StreamStatus integration ----------

func TestStreamScannerHandler_StreamStatus_DoneReason(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Nil(t, info.StreamStatus.EndError)
	assert.True(t, info.StreamStatus.IsNormalEnd())
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_EOFWithoutDone(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := range 5 {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.IsNormalEnd())
}

func TestStreamScannerHandler_StreamStatus_HandlerStop(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(100)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 10 {
			sr.Stop(fmt.Errorf("stop at 10"))
		}
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_HandlerDone(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(20)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 5 {
			sr.Done()
		}
	})

	assert.Equal(t, int64(5), count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_Timeout(t *testing.T) {
	// Not parallel: modifies global constant.StreamingTimeout
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	pr, pw := io.Pipe()
	go func() {
		fmt.Fprint(pw, "data: {\"id\":1}\n")
		time.Sleep(2 * time.Second)
		pw.Close()
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream timeout")
	}

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason)
	assert.False(t, info.StreamStatus.IsNormalEnd())
}

func TestStreamScannerHandler_StreamStatus_SoftErrors(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("soft error for chunk"))
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_MultipleErrorsPerChunk(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("error A"))
		sr.Error(fmt.Errorf("error B"))
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_ErrorThenStop(t *testing.T) {
	t.Parallel()

	// Use a large body without [DONE] to avoid race between scanner's [DONE]
	// and handler's Stop on the sync.Once EndReason.
	var b strings.Builder
	for i := range 100 {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
		sr.Error(fmt.Errorf("soft error"))
		sr.Stop(fmt.Errorf("fatal"))
	})

	assert.Equal(t, int64(1), count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
	assert.Equal(t, 2, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_InitializedIfNil(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(1)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	assert.Nil(t, info.StreamStatus)

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.NotNil(t, info.StreamStatus)
}

func TestStreamScannerHandler_StreamStatus_ReplacesPreInitialized(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RecordError("pre-existing error")

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 0, info.StreamStatus.TotalErrorCount())
}

// TestStreamScannerHandler_FalloverOnCompletesImmediately B3-2 开关 on 缓冲期
// 断言：首个有效 data 块 commit 前 recorder.Body 为空（响应头未上线）。
func TestStreamScannerHandler_FalloverOnBuffersBodyBeforeFirstData(t *testing.T) {
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	rs.StreamFallover = true
	t.Cleanup(func() { rs.StreamFallover = oldFallover })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	// 首包到达前（极短同步执行窗口内 recorder 应为空——由 writer commit 时机保证）。
	bufferWriter := NewFirstPacketBufferWriter(c.Writer)
	c.Writer = bufferWriter
	// 模拟首包前：写入 data 行但未 commit。
	_, _ = bufferWriter.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n"))
	assert.Empty(t, recorder.Body.String(), "缓冲期响应体必须为空（响应头未上线）")
	assert.False(t, bufferWriter.Committed())
	assert.Positive(t, bufferWriter.BufferedBytes())

	// commit 后缓冲穿透到 recorder。
	bufferWriter.Commit()
	assert.Contains(t, recorder.Body.String(), "x")
}

// ---------- B1-1 stream_fallover 判空与空壳流 ----------

func TestIsUsefulStreamData(t *testing.T) {
	cases := []struct {
		name string
		data string
		want bool
	}{
		{"content string", `{"choices":[{"delta":{"content":"hi"}}]}`, true},
		{"content empty string", `{"choices":[{"delta":{"content":""}}]}`, false},
		{"content array non-empty", `{"choices":[{"delta":{"content":[{"text":"x"}]}}]}`, true},
		{"content array empty", `{"choices":[{"delta":{"content":[]}}]}`, false},
		{"tool calls", `{"choices":[{"delta":{"tool_calls":[{"id":"c1"}]}}]}`, true},
		{"reasoning only", `{"choices":[{"delta":{"reasoning":"think"}}]}`, false},
		{"reasoning_content only", `{"choices":[{"delta":{"reasoning_content":"think"}}]}`, false},
		{"non-openai format", `{"type":"content_block_delta","delta":{"text":"hi"}}`, true},
		{"invalid json", `not json`, true},
		{"empty choices", `{"choices":[]}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isUsefulStreamData(tc.data)
			if got != tc.want {
				t.Errorf("isUsefulStreamData(%q) = %v, want %v", tc.data, got, tc.want)
			}
		})
	}
}

// TestStreamScannerHandler_FalloverOnReasoningOnlyStream B1-1：fallover on，
// 上游只回 reasoning（无 content/tool_call）→ 空壳流 → StreamScannerHandler
// 返回 fatalErr（交还重试链换下一渠道），客户端不应收到任何内容。
func TestStreamScannerHandler_FalloverOnReasoningOnlyStream(t *testing.T) {
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	rs.StreamFallover = true
	t.Cleanup(func() { rs.StreamFallover = oldFallover })

	body := "data: {\"choices\":[{\"delta\":{\"reasoning\":\"think hard\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"more thinking\"}}]}\n" +
		"data: [DONE]\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	fatalErr := StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})
	require.NotNil(t, fatalErr, "reasoning-only stream 必须判定为失败（空壳流）")
	assert.Contains(t, fatalErr.Error(), "upstream stream empty")
	assert.Equal(t, types.ErrorCodeDoRequestFailed, fatalErr.GetErrorCode())
}

// TestStreamScannerHandler_FalloverOnEmptyBody B1-1：fallover on，空响应体
// （只有 [DONE]）→ 空壳流 → fatalErr。
func TestStreamScannerHandler_FalloverOnEmptyBody(t *testing.T) {
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	rs.StreamFallover = true
	t.Cleanup(func() { rs.StreamFallover = oldFallover })

	body := "data: [DONE]\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	fatalErr := StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})
	require.NotNil(t, fatalErr, "空响应体 + fallover on 必须判定为失败（空壳流）")
	assert.Contains(t, fatalErr.Error(), "upstream stream empty")
	assert.Equal(t, types.ErrorCodeDoRequestFailed, fatalErr.GetErrorCode())
}

// TestStreamScannerHandler_FalloverOffReasoningOnlyStream B1-1：fallover off，
// reasoning-only 流透传（零变化），不应返回 fatalErr。
func TestStreamScannerHandler_FalloverOffReasoningOnlyStream(t *testing.T) {
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	rs.StreamFallover = false
	t.Cleanup(func() { rs.StreamFallover = oldFallover })

	body := "data: {\"choices\":[{\"delta\":{\"reasoning\":\"think hard\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"more\"}}]}\n" +
		"data: [DONE]\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var got []string
	fatalErr := StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) { got = append(got, data) })
	assert.Nil(t, fatalErr, "fallover off 时 reasoning-only 流应照常透传")
	assert.Len(t, got, 2)
}

// TestStreamScannerHandler_FalloverOnReasoningThenContent B1-1：fallover on，
// 先 reasoning 后 content → 首个 content 触发 commit，正常结束无 fatalErr，
// 且 reasoning 与 content 都被写穿到客户端（缓冲一并放行）。
func TestStreamScannerHandler_FalloverOnReasoningThenContent(t *testing.T) {
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	rs.StreamFallover = true
	t.Cleanup(func() { rs.StreamFallover = oldFallover })

	body := "data: {\"choices\":[{\"delta\":{\"reasoning\":\"think\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n" +
		"data: [DONE]\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	fatalErr := StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		_, _ = c.Writer.WriteString(data + "\n")
	})
	assert.Nil(t, fatalErr, "先 reasoning 后 content 应正常结束")
	// 缓冲放行后客户端应收到 content（reasoning 也在缓冲中一并写穿）。
	assert.Contains(t, recorder.Body.String(), "hello")
}

// TestStreamScannerHandler_FalloverThreeChannelScenario B1-1 验收项 3：
// 模拟三渠道 fallover 场景（同一客户端连接，逐个渠道尝试，失败交还重试链）：
//
//	渠道1：只回 reasoning 的空壳流 → fatalErr(empty) → 换下一候选
//	渠道2：首包超时（reasoning 后挂起）→ fatalErr(timeout) → 换下一候选
//	渠道3：正常流（content）→ 成功，客户端收到内容
//
// 断言：最终客户端只收到渠道3 的内容；前两个渠道失败时响应体保持为空。
func TestStreamScannerHandler_FalloverThreeChannelScenario(t *testing.T) {
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	oldTimeout := rs.StreamFirstTokenTimeout
	rs.StreamFallover = true
	rs.StreamFirstTokenTimeout = 1 // 1s 首包超时，加速测试
	t.Cleanup(func() {
		rs.StreamFallover = oldFallover
		rs.StreamFirstTokenTimeout = oldTimeout
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	// 渠道2 慢流：写入一行 reasoning 后挂起（触发首包超时）。
	pipeR, pipeW := io.Pipe()
	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		_, _ = pipeW.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking...\"}}]}\n"))
		select {
		case <-time.After(1500 * time.Millisecond):
		case <-c.Request.Context().Done():
		}
		_ = pipeW.Close()
	}()

	channels := []struct {
		name    string
		body    io.Reader
		wantErr bool
	}{
		{name: "reasoning-only empty", body: strings.NewReader(
			"data: {\"choices\":[{\"delta\":{\"reasoning\":\"think\"}}]}\n" +
				"data: [DONE]\n"), wantErr: true},
		{name: "slow first token timeout", body: pipeR, wantErr: true},
		{name: "normal content", body: strings.NewReader(
			"data: {\"choices\":[{\"delta\":{\"content\":\"OK-FROM-THIRD\"}}]}\n" +
				"data: [DONE]\n"), wantErr: false},
	}

	var lastErr error
	for i, ch := range channels {
		resp := &http.Response{Body: io.NopCloser(ch.body)}
		// 每个渠道独立 RelayInfo（重试链语义：新渠道新状态），但共用 c/recorder。
		chInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
		fatalErr := StreamScannerHandler(c, resp, chInfo, func(data string, sr *StreamResult) {
			_, _ = c.Writer.WriteString(data + "\n")
		})
		if ch.wantErr {
			require.NotNilf(t, fatalErr, "渠道 %d (%s) 应失败", i, ch.name)
			lastErr = fatalErr
		} else {
			require.Nilf(t, fatalErr, "渠道 %d (%s) 应成功", i, ch.name)
			break
		}
	}
	require.NotNil(t, lastErr, "前两个渠道应至少有一个失败")
	// 客户端最终只收到渠道3 内容。
	bodyOut := recorder.Body.String()
	assert.Contains(t, bodyOut, "OK-FROM-THIRD", "客户端应收到第三渠道正常流")
	assert.NotContains(t, bodyOut, "thinking...", "慢渠道的缓冲不应写穿到客户端")
	<-slowDone
}

// BenchmarkStreamFalloverBufferedMemory B1-1 验收项 4：单请求缓冲内存峰值
// 与并发 100 无泄漏压测记录。缓冲上限 maxFirstPacketBufferBytes=1MB，超过即放行。
func BenchmarkStreamFalloverBufferedMemory(b *testing.B) {
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	rs.StreamFallover = true
	b.Cleanup(func() { rs.StreamFallover = oldFallover })

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			bw := NewFirstPacketBufferWriter(c.Writer)
			// 写入 ~100KB 未 commit（模拟大 reasoning 缓冲）。
			chunk := bytes.Repeat([]byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"x\"}}]}\n"), 2000)
			_, _ = bw.Write(chunk)
			if bw.BufferedBytes() > maxFirstPacketBufferBytes {
				b.Fatalf("buffer exceeded 1MB cap: %d", bw.BufferedBytes())
			}
			bw.Commit()
		}
	})
}

// TestFirstPacketBufferWriterOverflow B1-1：缓冲超过 1MB 直接放行（防异常占内存）。
func TestFirstPacketBufferWriterOverflow(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	bw := NewFirstPacketBufferWriter(c.Writer)
	big := bytes.Repeat([]byte("x"), maxFirstPacketBufferBytes+1024)
	_, _ = bw.Write(big)
	assert.True(t, bw.Committed(), "超过 1MB 缓冲上限必须直接放行（进入直通）")
	assert.Contains(t, recorder.Body.String(), "x")
}

// TestStreamScannerHandler_ReasoningRenewsFirstTokenTimeout 生产回归（用户日志：
// 上游长 reasoning 流被首包超时误杀为 500/504）。fallover on + 极短首包超时，
// 上游先持续发 reasoning 超过超时阈值后才发 content → 必须正常透传不误杀。
func TestStreamScannerHandler_ReasoningRenewsFirstTokenTimeout(t *testing.T) {
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	oldTimeout := rs.StreamFirstTokenTimeout
	rs.StreamFallover = true
	rs.StreamFirstTokenTimeout = 1 // 1s 首包超时，加速测试
	t.Cleanup(func() {
		rs.StreamFallover = oldFallover
		rs.StreamFirstTokenTimeout = oldTimeout
	})

	// 渠道：reasoning 块持续 2.5s（> 1s 首包超时阈值）后才发 content。
	pipeR, pipeW := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 3 {
			_, _ = pipeW.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking step\"}}]}\n"))
			time.Sleep(900 * time.Millisecond)
		}
		_, _ = pipeW.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"FINAL-ANSWER\"}}]}\n"))
		_, _ = pipeW.Write([]byte("data: [DONE]\n"))
		_ = pipeW.Close()
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{Body: io.NopCloser(pipeR)}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	var got []string
	fatalErr := StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		got = append(got, data)
		_, _ = c.Writer.WriteString(data + "\n")
	})
	<-done

	assert.Nil(t, fatalErr, "reasoning 持续流超过首包超时阈值后到达 content 必须正常结束，不得误杀为 upstream stream timeout")
	assert.Contains(t, recorder.Body.String(), "FINAL-ANSWER", "客户端应收到最终 content")
}
