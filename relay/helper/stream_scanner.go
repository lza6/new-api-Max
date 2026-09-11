package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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

	// B3-2 流式首包缓冲 fallover（开关 relay.stream_fallover，默认 off）：
	// 安装缓冲 Writer 挡住客户端写；收到首个有效 data 块时 Commit 放行；
	// 首包超时则放弃本次 attempt（返回 fatalErr 交还渠道重试链）。
	// 缓冲期间 ping 保活必须关闭（PING 注释行会提前上线响应头）。
	fallover := relay_setting.GetRelaySetting().StreamFallover
	var bufferWriter *FirstPacketBufferWriter
	var firstTokenTimer *time.Timer
	firstTokenTimedOut := false
	if fallover {
		bufferWriter = NewFirstPacketBufferWriter(c.Writer)
		c.Writer = bufferWriter
		info.DisablePing = true
		firstTokenTimer = time.NewTimer(time.Duration(relay_setting.GetStreamFirstTokenTimeout()) * time.Second)
		defer firstTokenTimer.Stop()
	}
	commitBuffer := func() {
		if bufferWriter != nil {
			bufferWriter.Commit()
		}
		if firstTokenTimer != nil {
			firstTokenTimer.Stop()
		}
	}

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
				info.SetFirstResponseTime()
				info.ReceivedResponseCount++
				// B3-2：首个有效 data 块到达 → 一次性放行缓冲，进入直通。
				commitBuffer()

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

	// 主循环等待完成或超时
	select {
	case <-ticker.C:
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
	case <-stopChan:
		// EndReason already set by the goroutine that triggered stopChan
	case <-c.Request.Context().Done():
		// 客户端断开：立即 cleanup 关闭上游 resp.Body，解除 scanner 阻塞并让上游停止生成，
		// 避免为已放弃的请求继续消费上游 token。
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
	case <-firstTokenCh:
		// B3-2 首包超时：上游一个有效 data 块都没发出（响应头未上线），
		// 判定本次渠道失败，交还重试链换下一候选。
		firstTokenTimedOut = true
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
	}

	if firstTokenTimedOut {
		logger.LogWarn(c, fmt.Sprintf("stream first-token timeout (%ds), failover to next channel", relay_setting.GetStreamFirstTokenTimeout()))
		fatalErr = types.NewError(
			fmt.Errorf("upstream stream first-token timeout after %ds", relay_setting.GetStreamFirstTokenTimeout()),
			types.ErrorCodeDoRequestFailed,
			types.ErrOptionWithHideErrMsg("upstream stream timeout"),
		)
		return fatalErr
	}

	cleanup()
	if info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors() {
		logger.LogInfo(c, fmt.Sprintf("stream ended: %s", info.StreamStatus.Summary()))
	} else {
		logger.LogError(c, fmt.Sprintf("stream ended: %s, received=%d", info.StreamStatus.Summary(), info.ReceivedResponseCount))
	}
	return nil
}
