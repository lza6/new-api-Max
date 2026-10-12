package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/constant"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/lza6/new-api-Max/relay/helper"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/setting/relay_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 回归：上游「流式降级为非流式」的防御。
//
// 真实故障：渠道 #59（hebox.net，分组「超低价备用渠道」）对带
// `Content-Encoding: gzip` 的请求体不返回 SSE，而是返回 HTTP 200 +
// `application/json` 的非流式 chat.completion。网关的 SSE scanner 读不到
// `data:` 事件 → 判 emptyStream → 500 "upstream stream empty" 并 failover，
// 生产表现为「该分组老是 500，但上游侧没有任何失败记录」。
//
// 这些用例锁定：合成后的帧必须能被真实流式处理器消费，且非目标场景一律零行为变化。

func newDowngradeTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func newDowngradeTestInfo(isStream bool) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		IsStream:  isStream,
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:      59,
			ChannelSetting: dto.ChannelSettings{},
		},
	}
	info.UpstreamModelName = "deepseek-v4.1-flash"
	return info
}

const nonStreamCompletionBody = `{
  "id": "chatcmpl-abc",
  "object": "chat.completion",
  "created": 1791822747,
  "model": "deepseek-v4.1-flash",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "Hi there, friend."},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 595, "completion_tokens": 6, "total_tokens": 601}
}`

func TestStreamDowngradeSynthesizesSSEForNonStreamUpstreamBody(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       http.NoBody,
	}
	resp.Body = nopCloserString(nonStreamCompletionBody)

	require.True(t, downgradeUpstreamStreamToSSE(c, resp, info), "非流式 JSON 响应必须被合成为 SSE")

	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	body := readAllString(t, resp.Body)
	// 必须是合法 SSE：每个事件以 "data: " 开头，以空行分隔，且以 [DONE] 收尾。
	assert.True(t, strings.HasSuffix(body, "data: [DONE]\n\n"), "合成流必须以 [DONE] 收尾")
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		assert.True(t, strings.HasPrefix(line, "data: "), "每个 SSE 事件必须是 data: 行，实际=%q", line)
	}
	// 内容与 finish_reason / usage 必须忠实搬运。
	assert.Contains(t, body, `"Hi there, friend."`)
	assert.Contains(t, body, `"finish_reason":"stop"`)
	assert.Contains(t, body, `"total_tokens":601`)
	// 合成帧必须是 chunk 形状，否则流式处理器不会把它们当增量消费。
	assert.Contains(t, body, `"object":"chat.completion.chunk"`)
	// created 必须来自上游而不是被丢弃成 0。
	assert.Contains(t, body, `"created":1791822747`)
}

func TestStreamDowngradeLeavesStreamingUpstreamUntouched(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	original := "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\ndata: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       nopCloserString(original),
	}

	assert.False(t, downgradeUpstreamStreamToSSE(c, resp, info), "上游本就是 SSE 时不得干预")
	assert.Equal(t, original, readAllString(t, resp.Body), "body 必须逐字未变")
}

func TestStreamDowngradeSkipsNonStreamRequest(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(false)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       nopCloserString(nonStreamCompletionBody),
	}

	assert.False(t, downgradeUpstreamStreamToSSE(c, resp, info), "客户端本就要非流式时无需合成")
	assert.Equal(t, nonStreamCompletionBody, readAllString(t, resp.Body))
}

func TestStreamDowngradeSkipsForceFormatChannel(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	info.ChannelSetting.ForceFormat = true
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       nopCloserString(nonStreamCompletionBody),
	}

	assert.False(t, downgradeUpstreamStreamToSSE(c, resp, info), "ForceFormat 渠道走既有转换链，不合成")
}

// 回归：**透传渠道也必须合成**。
//
// 生产故障渠道 #59（hebox.net）同时开着 `pass_through_body_enabled: true` 与
// `request_compression: true`。透传只决定「发给上游的请求体是否改写」，与
// 「上游回来的响应体形状」无关——响应体形状由客户端请求的协议决定。
// 曾按 PassThroughBodyEnabled 排除，导致本修复对该生产渠道完全失效
// （端到端实测：压缩请求仍返回 500 upstream stream empty）。
func TestStreamDowngradeAppliesToPassThroughChannel(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	info.ChannelSetting.PassThroughBodyEnabled = true
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       nopCloserString(nonStreamCompletionBody),
	}

	require.True(t, downgradeUpstreamStreamToSSE(c, resp, info),
		"透传渠道的上游同样会返回非流式 JSON，必须合成，否则压缩请求持续 500")
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Contains(t, readAllString(t, resp.Body), "Hi there, friend.")
}

// 上游返回 OpenAI 错误体时绝不能合成「成功流」——否则会把上游错误包装成 200 成功。
func TestStreamDowngradeDoesNotSynthesizeUpstreamErrorBody(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	errBody := `{"error":{"message":"insufficient balance","type":"insufficient_quota","code":"insufficient_quota"}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       nopCloserString(errBody),
	}

	assert.False(t, downgradeUpstreamStreamToSSE(c, resp, info), "上游错误体必须交回通用错误处理")
	assert.Equal(t, errBody, readAllString(t, resp.Body), "body 必须完整保留给错误处理器")
}

// 空体必须交回原有空壳流检测，让错误信息保持准确（"upstream stream empty"），
// 不能被合成成一个「成功但无内容」的流。
func TestStreamDowngradeLeavesEmptyBodyToExistingDetection(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       nopCloserString(""),
	}

	assert.False(t, downgradeUpstreamStreamToSSE(c, resp, info))
	assert.Equal(t, "", readAllString(t, resp.Body))
}

// 非 JSON 体（如上游返回 HTML 错误页）必须原样交回，由其报告真实原因。
func TestStreamDowngradeRestoresBodyWhenNotJSON(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	html := "<html><body>502 Bad Gateway</body></html>"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       nopCloserString(html),
	}

	assert.False(t, downgradeUpstreamStreamToSSE(c, resp, info))
	assert.Equal(t, html, readAllString(t, resp.Body), "非 JSON 体必须原样可读")
}

// 多 choice 响应必须为每个 choice 各出一组帧，否则除第一个之外的内容会丢失。
func TestStreamDowngradeEmitsFramesForEveryChoice(t *testing.T) {
	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	body := `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[
	  {"index":0,"message":{"role":"assistant","content":"first"},"finish_reason":"stop"},
	  {"index":1,"message":{"role":"assistant","content":"second"},"finish_reason":"length"}],
	  "usage":{"total_tokens":9}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       nopCloserString(body),
	}

	require.True(t, downgradeUpstreamStreamToSSE(c, resp, info))
	out := readAllString(t, resp.Body)
	assert.Contains(t, out, `"first"`)
	assert.Contains(t, out, `"second"`)
	assert.Contains(t, out, `"finish_reason":"length"`)
	// 每个 choice 产出两帧（增量帧 + finish 帧）→ 共 4 帧，计数按逗号计数法：
	// 每帧 2 个 choices 字段（帧内 choices 是单元素数组），故 "index": 出现 4 次。
	assert.Equal(t, 4, strings.Count(out, `"index":`), "两个 choice 各出两帧 = 4 帧")
}

// 合成流必须能被真实的流式引擎读成有效增量——这是「修好了」的核心证据：
// 只断言「函数返回 true」是不够的，必须证明下游消费者不再判它为空壳流。
func TestStreamDowngradeOutputIsAcceptedByStreamScanner(t *testing.T) {
	// 真实生产条件：fallover 开启（空壳流在此开关下才判失败），且流式超时为正
	// （生产由 InitEnv 设置；测试里必须显式初始化，否则 NewTicker 会 panic）。
	rs := relay_setting.GetRelaySetting()
	oldFallover := rs.StreamFallover
	relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) { s.StreamFallover = true })
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		relay_setting.UpdateRelaySetting(func(s *relay_setting.RelaySetting) { s.StreamFallover = oldFallover })
		constant.StreamingTimeout = oldStreamingTimeout
	})

	c := newDowngradeTestContext()
	info := newDowngradeTestInfo(true)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       nopCloserString(nonStreamCompletionBody),
	}
	require.True(t, downgradeUpstreamStreamToSSE(c, resp, info))

	var got []string
	fatalErr := helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		got = append(got, data)
	})
	assert.Nil(t, fatalErr, "合成流不得被判为空壳流（这正是生产 500 的成因）")
	require.NotEmpty(t, got, "合成流必须产出 data 事件")
	joined := strings.Join(got, "\n")
	assert.Contains(t, joined, "Hi there, friend.")
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason,
		"合成流应以正常结束收尾")
}

func nopCloserString(s string) interface {
	Read([]byte) (int, error)
	Close() error
} {
	return nopCloser{strings.NewReader(s)}
}

type nopCloser struct{ *strings.Reader }

func (nopCloser) Close() error { return nil }

func readAllString(t *testing.T, r interface{ Read([]byte) (int, error) }) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}
