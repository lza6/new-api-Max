package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/lza6/new-api-Max/relaykit/dto"
)

// 上游「流式降级为非流式」的防御。
//
// ## 真实故障（2026-10-11/12 生产实测）
//
// 渠道 #59（`https://hebox.net`，分组「超低价备用渠道」）在客户端请求 `stream:true`
// 时返回 **非流式** `application/json`（`object: "chat.completion"`，没有 `data:`
// 事件）。原因是该网关不支持 `Content-Encoding: gzip` 的请求体：它把压缩体当普通
// JSON 解析失败后，回落到自己的非流式分支（HTTP 200，而不是 400/415/422）。
//
// 网关侧 SSE scanner 读不到任何 `data:` 事件 → 判 `emptyStream` → 返回 500
// `upstream stream empty` 并触发 failover。生产表现为「超低价分组老是 500，
// 但上游侧没有任何失败记录」——上游其实**成功**了，是网关把成功响应判成了空流。
//
// 更糟的是既有的压缩熔断（`TripChannelCompressionBreaker`）只认 400/415/422，
// 200 不触发，所以这条渠道永远不会被自动隔离，故障持续两天。
//
// ## 修法
//
// 上游已经产出了**完整且正确**的 chat.completion 响应体。把它合成为等价的 SSE
// 流（一个带完整 message 的 delta 帧 + 一个 finish 帧 + `[DONE]`）交给既有的
// `OaiStreamHandler`，客户端拿到的是它请求的流式格式，计费与日志走原有路径。
//
// ## 为什么必须做「可回放 body」
//
// `openai.OaiStreamHandler` 在 `fatalErr != nil`（如空壳流）时返回 fatalErr，渠道
// 重试链会**用同一个 resp 再试一次**？不会——重试链换渠道重建请求。但本函数在
// `DoResponse` 之前已经读掉了 `resp.Body`；若合成帧解析失败需要回退成真正的
// 「按非流式处理」，body 必须还在。因此这里先把上游体读进内存（有上限），再用
// 同一份字节同时支撑两种去向：合成 SSE，或还原 body 走非流式路径。

// maxStreamDowngradeBodyBytes 允许读取的上游响应体上限。正常的 chat.completion
// 响应远小于此值；上限用于防止异常上游用超大 body 撑爆内存。
const maxStreamDowngradeBodyBytes = 8 << 20 // 8MB

// adaptorWantsStreamingDowngrade 报告该 Adaptor 的 DoResponse 是否会走
// 「OpenAI 兼容 chat 流式处理器」。仅这类处理器才需要本合成（其它模式下
// `info.IsStream` 的语义不同，合成会引入错误行为）。
func adaptorWantsStreamingDowngrade(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeCompletions:
		return true
	default:
		return false
	}
}

// downgradeUpstreamStreamToSSE 尝试把非流式 JSON 响应合成为 SSE 流。
//
// 返回 true 表示已合成（httpResp.Body 已被替换为合成流，Content-Type 已改为
// text/event-stream）；返回 false 表示未改动（调用方按原样继续，行为与改动前
// 完全一致）。
//
// 契约：
//   - 仅在「客户端要流式」+「上游 200 且 Content-Type 不是 event-stream」时调用。
//   - 合成帧必须能被 OaiStreamHandler 正常消费：`choices[0].delta` 携带完整
//     message 内容，第二帧携带 finish_reason 与 usage，最后一帧 `[DONE]`。
//   - 上游返回的是 OpenAI 错误体（`error` 字段）时不合成，交回通用错误处理。
//   - 任何解析/编解码失败都 fail-open 回退非流式，绝不因合成失败而拒绝请求。
func downgradeUpstreamStreamToSSE(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) bool {
	if resp == nil || resp.Body == nil || info == nil {
		return false
	}
	if !info.IsStream {
		return false
	}
	// 上游确实在流式：无需合成。
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return false
	}
	// 渠道支持「强制格式转换」时，上游本可以返回任意格式并由既有转换链处理，
	// 保持既有语义不干预。
	if info.ChannelSetting.ForceFormat {
		return false
	}
	// 注意：**不要**在这里按 PassThroughBodyEnabled 排除。透传只影响「网关发给
	// 上游的请求体是否被改写」，与「上游回来的响应体是什么形状」无关：响应体始终
	// 由客户端请求的协议（此处是 OpenAI chat/completions）决定。生产故障渠道 #59
	// 恰恰同时开着透传与压缩——按透传排除会让本修复对该渠道完全失效（实测已复现）。

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxStreamDowngradeBodyBytes+1))
	if err != nil {
		// 读取失败：body 已被消费掉一部分，无法安全回退；如实报错而不是让
		// 上层把半截 body 当非流式解析（那会产生更迷惑的「bad response body」）。
		logger.LogError(c, "stream downgrade: failed to read non-stream upstream body: "+err.Error())
		return false
	}
	_ = resp.Body.Close()
	restore := func() {
		resp.Body = io.NopCloser(bytes.NewReader(raw))
	}
	if int64(len(raw)) > maxStreamDowngradeBodyBytes {
		logger.LogWarn(c, fmt.Sprintf("stream downgrade: upstream body exceeds %d bytes, keeping non-stream path", maxStreamDowngradeBodyBytes))
		restore()
		return false
	}
	if len(raw) == 0 {
		// 真空体：交回原有的空壳流检测（它会给出准确的 "upstream stream empty"）。
		restore()
		return false
	}

	var parsed dto.OpenAITextResponse
	if err := common.Unmarshal(raw, &parsed); err != nil {
		// 不是 JSON（可能是纯文本错误页等）：交回原有非流式路径，由其报告真实原因。
		restore()
		return false
	}
	if oaiErr := parsed.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
		// 上游返回的是 OpenAI 错误体：交回通用错误处理，不要合成成功流。
		restore()
		return false
	}
	if len(parsed.Choices) == 0 {
		restore()
		return false
	}

	frames, err := buildSSEFramesFromCompletion(&parsed, info)
	if err != nil {
		logger.LogError(c, "stream downgrade: failed to synthesize SSE frames: "+err.Error())
		restore()
		return false
	}

	// 先记下上游真实的 Content-Type 再改写，否则日志会把我们自己写回的值当成
	// 「上游返回的格式」，诊断信息失真。
	upstreamContentType := resp.Header.Get("Content-Type")
	resp.Body = io.NopCloser(bytes.NewReader(frames))
	resp.ContentLength = int64(len(frames))
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Header.Del("Content-Length")
	logger.LogWarn(c, fmt.Sprintf(
		"upstream returned a non-stream %s body for a streaming request; synthesized SSE so the response is not misjudged as an empty stream (channel #%d)",
		upstreamContentType, info.ChannelId))
	return true
}

// buildSSEFramesFromCompletion 把完整 chat.completion 响应合成为等价的 SSE 帧序列。
//
// 帧结构（对齐 OpenAI 流式语义，确保 OaiStreamHandler 的
// `processTokenData` / `handleLastResponse` 都能正确消费 usage 与 finish_reason）：
//
//	帧1：delta{role,content,tool_calls,reasoning_content}, finish_reason=null
//	帧2：delta{}, finish_reason=<上游的 finish_reason>, usage=<上游 usage>
//	帧3：data: [DONE]
func buildSSEFramesFromCompletion(resp *dto.OpenAITextResponse, info *relaycommon.RelayInfo) ([]byte, error) {
	model := resp.Model
	if model == "" && info != nil {
		model = info.UpstreamModelName
	}
	var buf bytes.Buffer
	for i := range resp.Choices {
		choice := &resp.Choices[i]
		first, err := marshalSSEFrame(streamChunk{
			ID:      resp.Id,
			Object:  "chat.completion.chunk",
			Created: createdUnix(resp.Created),
			Model:   model,
			Choices: []streamChunkChoice{{
				Index: choice.Index,
				Delta: streamChunkDelta{
					Role:             choice.Message.Role,
					Content:          choice.Message.StringContent(),
					ReasoningContent: choice.Message.GetReasoningContent(),
					ToolCalls:        choice.Message.ToolCalls,
				},
				FinishReason: nil,
			}},
		})
		if err != nil {
			return nil, err
		}
		buf.Write(first)

		finish := choice.FinishReason
		last, err := marshalSSEFrame(streamChunk{
			ID:      resp.Id,
			Object:  "chat.completion.chunk",
			Created: createdUnix(resp.Created),
			Model:   model,
			Choices: []streamChunkChoice{{
				Index:        choice.Index,
				Delta:        streamChunkDelta{},
				FinishReason: &finish,
			}},
			// usage 只挂在最后一帧（OpenAI 语义：stream_options.include_usage 时
			// 末端出现带 usage 的空 choices 帧；这里直接附带在 finish 帧上，既有
			// handleLastResponse 同样按「最后一帧取 usage」消费）。
			Usage: &resp.Usage,
		})
		if err != nil {
			return nil, err
		}
		buf.Write(last)
	}
	buf.WriteString("data: [DONE]\n\n")
	return buf.Bytes(), nil
}

// streamChunk / streamChunkChoice / streamChunkDelta 是合成帧与上游真实 SSE 帧
// 共用的 JSON 形状（字段名与 OpenAI chat.completion.chunk 一致）。
type streamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []streamChunkChoice `json:"choices"`
	Usage   *dto.Usage          `json:"usage,omitempty"`
}

type streamChunkChoice struct {
	Index        int              `json:"index"`
	Delta        streamChunkDelta `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
}

type streamChunkDelta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
}

func marshalSSEFrame(chunk streamChunk) ([]byte, error) {
	payload, err := common.Marshal(chunk)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("data: ")
	buf.Write(payload)
	buf.WriteString("\n\n")
	return buf.Bytes(), nil
}

// createdUnix 把上游 created 字段（可能是 float64 / json.Number / int64，取决于
// 上游与解码路径）归一为秒级 Unix 时间戳。无法识别时返回 0，与上游缺失该字段
// 时的行为一致（不猜测当前时间：合成帧应忠实反映上游响应）。
func createdUnix(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
	case string:
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n
		}
	}
	return 0
}
