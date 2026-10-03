package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/logger"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/lza6/new-api-Max/setting/relay_setting"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 64 << 10  // 64KB (64*1024)
	DefaultMaxScannerBufferSize = 128 << 20 // 64MB (64*1024*1024) default SSE buffer size
	DefaultPingInterval         = 10 * time.Second
	// streamWriteTimeout bounds a single blocked write to a slow client so the
	// unconditional wg.Wait() in cleanup can always finish. Without it, a slow
	// but connected client (full TCP buffer, no server WriteTimeout) could hang
	// the handler forever.
	streamWriteTimeout = 30 * time.Second
)

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

func NewStreamScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	return scanner
}

// isUsefulStreamData 判断一个 SSE data 块是否包含“有效”增量（content 或
// tool_call），语义对齐 free-router usefulDelta：仅 reasoning/reasoning_content
// 视为无效（空壳流）。非 OpenAI 兼容结构（Claude/Gemini/Dify 等）保守返回
// true，避免误伤其它格式渠道。fallover 关闭时本函数不参与决策。
func isUsefulStreamData(data string) bool {
	var payload struct {
		Choices []struct {
			Delta *struct {
				Content          any   `json:"content"`
				ReasoningContent any   `json:"reasoning_content"`
				Reasoning        any   `json:"reasoning"`
				ToolCalls        []any `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := common.Unmarshal([]byte(data), &payload); err != nil {
		// 无法解析 → 保守放行（保持旧透传行为）。
		return true
	}
	if len(payload.Choices) == 0 || payload.Choices[0].Delta == nil {
		// 非 OpenAI 兼容结构 → 保守放行。
		return true
	}
	delta := payload.Choices[0].Delta
	if len(delta.ToolCalls) > 0 {
		return true
	}
	// [修复防御] reasoning/reasoning_content 是上游返回的真实内容（deepseek 系
	// 思考流）：透传模式必须透传，不能因「无 content」判空壳 500。
	// 真空响应（零 data 事件）仍由 emptyStream 检测兜底。
	if isNonEmptyDelta(delta.ReasoningContent) || isNonEmptyDelta(delta.Reasoning) {
		return true
	}
	switch v := delta.Content.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	}
	return false
}

func copyCodexSSEHeaders(c *gin.Context, resp *http.Response) {
	if c == nil || c.Writer == nil || resp == nil {
		return
	}
	// codex
	for _, name := range []string{"X-Reasoning-Included", "X-Codex-Turn-State"} {
		values := resp.Header.Values(name)
		if !service.ShouldCopyUpstreamHeader(c, name, values) {
			continue
		}
		for _, value := range values {
			if value != "" {
				c.Writer.Header().Add(name, value)
			}
		}
	}
}

// ExtendWriteDeadline pushes the connection write deadline forward before each
// stream write. Best-effort: writers that don't support deadlines (e.g.
// httptest recorders) are silently ignored.
func ExtendWriteDeadline(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(streamWriteTimeout))
}

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string, sr *StreamResult)) (fatalErr *types.NewAPIError) {

	if resp == nil || dataHandler == nil {
		return nil
	}

	// 无条件新建 StreamStatus
	info.StreamStatus = relaycommon.NewStreamStatus()

	// B3-2 流式首包缓冲 fallover（开关 relay.stream_fallover，默认 on）：
	// 安装缓冲 Writer 挡住客户端写；收到首个有效 data 块时 Commit 放行；
	// 首包超时则放弃本次 attempt（返回 fatalErr 交还渠道重试链）。
	// 缓冲期间 ping 保活必须关闭（PING 注释行会提前上线响应头）。
	fallover := relay_setting.GetRelaySetting().StreamFallover
	var bufferWriter *FirstPacketBufferWriter
	var firstTokenTimer *time.Timer
	firstTokenTimedOut := false
	// [修复防御] 渠道级「流式直通（透传）」：稳定/中转渠道开启
	// DisableStreamFirstTokenTimeout 后，首包缓冲与空壳兜底保留，但不再按
	// stream_first_token_timeout（默认 15s）判首包超时——上游慢但正常（排队、
	// 长 reasoning 后才有首 token）的流不再被网关策略误杀成 500/504；
	// 仍由持续流超时（STREAMING_TIMEOUT）与空壳流检测兜底。
	firstTokenTimeout := relay_setting.GetStreamFirstTokenTimeout()
	if info != nil && info.ChannelMeta != nil && info.ChannelSetting.DisableStreamFirstTokenTimeout {
		firstTokenTimeout = 0
		logger.LogInfo(c, "channel disables stream first-token timeout (passthrough)")
	}
	// B1-1 修复：保存原始 writer，函数返回前恢复。否则渠道重试链上
	// 第二次调用会嵌套包装上一次未 commit 的 bufferWriter，导致提交数据
	// 写进旧缓冲而无法透传到客户端（真实 bug：三渠道 fallover 场景验证暴露）。
	originalWriter := c.Writer
	if fallover {
		bufferWriter = NewFirstPacketBufferWriter(c.Writer)
		c.Writer = bufferWriter
		info.DisablePing = true
		// 渠道透传开关关闭首包超时时不建定时器（firstTokenCh 为 nil，
		// 主 select 永不命中该分支），由缓冲+空壳/持续流超时兜底。
		if firstTokenTimeout > 0 {
			firstTokenTimer = time.NewTimer(time.Duration(firstTokenTimeout) * time.Second)
			defer firstTokenTimer.Stop()
		}
		// 成功路径：buffer 已 commit 写穿到 originalWriter，恢复后 dataHandler
		// 直接写原始 writer（等价直通）；失败路径：缓冲丢弃，客户端零字节，
		// 重试链干净地换下一候选。
		defer func() {
			c.Writer = originalWriter
		}()
	}
	// [修复防御] 仅停止首包超时计时（上游已开始传输任何数据块），不 commit 缓冲：
	// reasoning 流不断续命不被误杀，但纯 reasoning 无 content 流结束仍判空壳。
	stopFirstTokenTimer := func() {
		if firstTokenTimer != nil {
			firstTokenTimer.Stop()
		}
	}
	commitBuffer := func() {
		if bufferWriter != nil {
			bufferWriter.Commit()
		}
		stopFirstTokenTimer()
	}
	// B1-1 空壳流标记：fallover 开启时，若流正常结束但从未遇到有效
	// content/tool_call，且缓冲里也没有任何上游字节，才判定本次渠道失败交还
	// 重试链。由 scanner goroutine / main 双方涉及，用 atomic 消除数据竞争。
	var emptyStream atomic.Bool

	ctx, cancel := context.WithCancel(context.Background())

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second

	var (
		stopChan    = make(chan bool, 3) // 增加缓冲区避免阻塞
		scanner     = NewStreamScanner(resp.Body)
		ticker      = time.NewTicker(streamingTimeout)
		pingTicker  *time.Ticker
		writeMutex  sync.Mutex     // Mutex to protect concurrent writes
		wg          sync.WaitGroup // 用于等待所有 goroutine 退出
		cleanupOnce sync.Once
		stopOnce    sync.Once
	)

	stop := func() {
		stopOnce.Do(func() {
			close(stopChan)
		})
	}

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	logger.LogDebug(c, "relay timeout seconds: %d", common.RelayTimeout)
	logger.LogDebug(c, "relay max idle conns: %d", common.RelayMaxIdleConns)
	logger.LogDebug(c, "relay max idle conns per host: %d", common.RelayMaxIdleConnsPerHost)
	logger.LogDebug(c, "streaming timeout seconds: %d", int64(streamingTimeout.Seconds()))
	logger.LogDebug(c, "ping interval seconds: %d", int64(pingInterval.Seconds()))

	cleanup := func() {
		cleanupOnce.Do(func() {
			cancel()
			stop()
			if resp.Body != nil {
				_ = resp.Body.Close()
			}

			ticker.Stop()
			if pingTicker != nil {
				pingTicker.Stop()
			}

			wg.Wait()
		})
	}
	// Ensure gin.Context is not returned to Gin's pool while any stream goroutine can still use it.
	defer cleanup()

	scanner.Split(bufio.ScanLines)
	copyCodexSSEHeaders(c, resp)
	SetEventStreamHeaders(c)

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	// Handle ping data sending with improved error handling
	if pingEnabled && pingTicker != nil {
		wg.Add(1)
		gopool.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogError(c, fmt.Sprintf("ping goroutine panic: %v", r))
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("ping panic: %v", r))
					stop()
				}
				logger.LogDebug(c, "ping goroutine exited")
				wg.Done()
			}()

			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute // 最大 ping 持续时间
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()

			for {
				select {
				case <-pingTicker.C:
					var err error
					func() {
						writeMutex.Lock()
						defer writeMutex.Unlock()
						ExtendWriteDeadline(c)
						err = PingData(c)
					}()
					if err != nil {
						logger.LogError(c, "ping data error: "+err.Error())
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
						return
					}
					logger.LogDebug(c, "ping data sent")
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					// 监听客户端断开连接
					return
				case <-pingTimeout.C:
					logger.LogError(c, "ping goroutine max duration reached")
					return
				}
			}
		})
	}

	dataChan := make(chan string, 10)

	wg.Add(1)
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("data handler goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("handler panic: %v", r))
			}
			stop()
			wg.Done()
		}()
		sr := newStreamResult(info.StreamStatus)
		for data := range dataChan {
			sr.reset()
			func() {
				writeMutex.Lock()
				defer writeMutex.Unlock()
				ExtendWriteDeadline(c)
				dataHandler(data, sr)
			}()
			if sr.IsStopped() {
				return
			}
		}
	})

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			close(dataChan)
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("scanner panic: %v", r))
			}
			stop()
			logger.LogDebug(c, "scanner goroutine exited")
			wg.Done()
		}()

		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			default:
			}

			ticker.Reset(streamingTimeout)
			data := scanner.Text()
			logger.LogDebug(c, "stream scanner data: %s", data)

			if len(data) < 6 {
				continue
			}
			if data[:5] != "data:" && data[:6] != "[DONE]" {
				continue
			}
			data = data[5:]
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}
			if !strings.HasPrefix(data, "[DONE]") {
				if !fallover || isUsefulStreamData(data) {
					// B3-2/B1-1：首个“有效”data 块（含 content/tool_call）到达
					// → 一次性放行缓冲，进入直通。仅 reasoning 的空壳流不提交。
					info.SetFirstResponseTime()
					info.ReceivedResponseCount++
					commitBuffer()
				} else {
					// [修复防御] 上游已产出数据块但暂无法识别为 content（role-only /
					// 空 delta / 非标准格式的 reasoning 字段）。此处**不再停掉首包计时器**：
					// 计时器保留作为「缓冲持有上限」——到期时若缓冲已有字节则放行透传
					// （见主 select），只有缓冲零字节（上游完全无声）才判首包失败。
					// 否则缓冲会被无界持有，用户被扣住响应头直到首个 content 出现，
					// 首字延迟被缓冲放大到数分钟（生产现象）。
					logger.LogDebug(c, "stream first packet lacks recognizable content, keep buffering until hold deadline")
				}

				select {
				case dataChan <- data:
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				}
			} else {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				logger.LogDebug(c, "received [DONE], stopping scanner")
				return
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, "scanner error: "+err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
			}
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	})

	// B3-2 首包超时 channel：fallover 关闭/禁用超时时为 nil（select 永不命中）。
	var firstTokenCh <-chan time.Time
	if firstTokenTimer != nil {
		firstTokenCh = firstTokenTimer.C
	}

	// 主循环：等待流结束/超时/客户端断开。[修复防御] 首包计时器到期时若缓冲非空，
	// **放行缓冲后必须继续等待流结束**（而非直接结束流），否则会把一个正在输出的
	// 上游中途掐断——这正是本次修复要消除的「首字几分钟」链条上的新坑。
	for {
		select {
		case <-ticker.C:
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
			goto streamFinished
		case <-stopChan:
			// EndReason already set by the goroutine that triggered stopChan
			goto streamFinished
		case <-c.Request.Context().Done():
			// 客户端断开：立即 cleanup 关闭上游 resp.Body，解除 scanner 阻塞并让上游停止生成，
			// 避免为已放弃的请求继续消费上游 token。
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
			goto streamFinished
		case <-firstTokenCh:
			// 首包计时器到期 = 缓冲持有上限到达。关键区分：
			//   - 缓冲已有字节（上游活着，只是迟迟没产出可识别 content）→ 放行缓冲继续透传，
			//     并继续等待流结束（firstTokenCh 置 nil 后本分支不再触发）。
			//   - 缓冲为空（上游一个字节都没发）→ 判首包失败，交还重试链换下一候选。
			if bufferWriter == nil || bufferWriter.Committed() {
				// 计时器与 scanner 竞态：scanner 已先提交/无缓冲，忽略即可。
			} else if bufferWriter.BufferedBytes() > 0 {
				logger.LogWarn(c, fmt.Sprintf(
					"stream first-token deadline (%ds) reached with %d bytes buffered but no recognizable content; releasing buffer and continuing passthrough",
					relay_setting.GetStreamFirstTokenTimeout(), bufferWriter.BufferedBytes()))
				commitBuffer()
			} else {
				firstTokenTimedOut = true
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
				goto streamFinished
			}
			// 放行后计时器已停止，避免重复命中；继续等待流自然结束。
			firstTokenCh = nil
		}
	}

streamFinished:

	// [修复防御] 客户端断开不是上游故障，优先级高于其余结束原因：上游可能仍在
	// prefill，客户端先取消（用户取消 / 切页 / 客户端超时 / 网络抖动）。此时主循环
	// 走 ClientGone 分支，resp.Body 由 cleanup() 关闭，scanner 收到 **本地** 错误串
	// "http2: response body closed"（Go 在 resp.Body.Close() 时生成，上游从不发送），
	// 但缓冲尚未 commit 且零字节 —— 与「上游真的什么都没回」形态完全相同。
	// 若不在此处拦截，会误判成 500「upstream stream empty」并对同渠道无意义重试
	//（生产实测 49→49，上游侧无任何失败记录），用户误以为是上游错误。
	// 以请求上下文的 cancel 为权威信号（EndReason 有 first-wins 竞态：scanner
	// goroutine 可能先落 ScannerErr 覆盖 ClientGone）。客户端已无接收方，静默结束。
	if c.Request != nil && c.Request.Context().Err() != nil {
		logger.LogInfo(c, "stream ended: client disconnected, skip empty-stream failover")
		return nil
	}

	if firstTokenTimedOut {
		// [修复防御] 首包超时是「上游慢/无响应」而非「网关故障」：诚实映射 504
		// Gateway Timeout，不再返回误导性的 500（生产「莫名 500」的另一真因）。
		// 保留可观测日志，不静默吞错。
		timeoutSecs := relay_setting.GetStreamFirstTokenTimeout()
		logger.LogWarn(c, fmt.Sprintf("stream first-token timeout (%ds), upstream sent no data; failover to next channel", timeoutSecs))
		fatalErr = types.NewError(
			fmt.Errorf("upstream stream first-token timeout after %ds (no data received)", timeoutSecs),
			types.ErrorCodeDoRequestFailed,
			types.ErrOptionWithStatusCode(http.StatusGatewayTimeout),
			types.ErrOptionWithHideErrMsg("upstream stream timeout"),
		)
		return fatalErr
	}

	// B1-1 空壳流：流已正常结束但从未遇到有效 content/tool_call。free-router 语义：
	// reasoning only or empty stream → 换下一候选。
	// [修复防御] 必须先 cleanup()（wg.Wait 等所有缓冲写入落定）再判定，否则会与
	// dataHandler goroutine 竞争读到「尚未写入」的缓冲字节，把有数据的流误判空壳。
	cleanup()
	if fallover && bufferWriter != nil && !bufferWriter.Committed() {
		if buffered := bufferWriter.BufferedBytes(); buffered > 0 {
			// 缓冲确有上游数据（role-only / 非识别 content 的心跳）：放行给客户端，
			// 绝不判失败——否则对一个正在工作的上游反复重试并最终 500。
			logger.LogWarn(c, fmt.Sprintf(
				"stream ended without recognizable content but %d bytes were buffered; releasing to client instead of failing over", buffered))
			bufferWriter.Commit()
		} else {
			emptyStream.Store(true)
		}
	}
	if emptyStream.Load() {
		logger.LogWarn(c, "stream empty (no data events at all), failover to next channel")
		fatalErr = types.NewError(
			fmt.Errorf("upstream stream empty (no data received)"),
			types.ErrorCodeDoRequestFailed,
			types.ErrOptionWithHideErrMsg("upstream stream empty"),
		)
		return fatalErr
	}

	if info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors() {
		logger.LogInfo(c, fmt.Sprintf("stream ended: %s", info.StreamStatus.Summary()))
	} else {
		logger.LogError(c, fmt.Sprintf("stream ended: %s, received=%d", info.StreamStatus.Summary(), info.ReceivedResponseCount))
	}
	return nil
}

func isNonEmptyDelta(v any) bool {
	switch val := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(val) != ""
	case []any:
		return len(val) > 0
	default:
		return true
	}
}
