package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	loggerINFO  = "INFO"
	loggerWarn  = "WARN"
	loggerError = "ERR"
	loggerDebug = "DEBUG"
)

const maxLogCount = 1000000

// logCount 与 setupLogWorking 会被所有并发日志调用读写。
// [fix-race] 原实现把 logCount 当「近似计数，不加锁」——但未同步的整数自增
// 在 Go 内存模型下就是数据竞争（并发压测下 race detector 会报错并导致测试
// panic）。用 atomic 既保留「近似计数、不阻塞日志」的原意，又消除竞争。
var logCount atomic.Int64

// logBytes 当前日志文件累计写入的字节数（T6.3 按大小分片的依据）。
//
// 用 atomic 的理由与 logCount 相同：多个 goroutine 并发写日志。
// 它在轮转时与 logCount 一起清零。
var logBytes atomic.Int64

// maxLogSizeBytes 单文件大小上限（启动时解析一次，避免每条日志读环境变量）。
var maxLogSizeBytes = resolveMaxLogSizeBytes()

var setupLogLock sync.Mutex
var setupLogWorking atomic.Bool
var currentLogPath string
var currentLogPathMu sync.RWMutex
var currentLogFile *os.File

func GetCurrentLogPath() string {
	currentLogPathMu.RLock()
	defer currentLogPathMu.RUnlock()
	return currentLogPath
}

func SetupLogger() {
	defer func() {
		setupLogWorking.Store(false)
	}()
	if *common.LogDir != "" {
		ok := setupLogLock.TryLock()
		if !ok {
			log.Println("setup log is already working")
			return
		}
		defer func() {
			setupLogLock.Unlock()
		}()
		// 文件名经唯一性校验：按大小轮转时两次轮转可能落在同一秒，
		// 只靠时间戳会让新文件与旧文件同名 → O_APPEND 继续写原文件，
		// 「轮转」静默失效（见 rotation.go 的说明）。
		logPath := nextLogFileName(*common.LogDir, time.Now(), nil)
		fd, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal("failed to open log file")
		}
		// 新文件从 0 字节开始计数（不是续上旧文件的累计值，
		// 否则新文件一创建就立刻超过阈值、陷入连续轮转）。
		logBytes.Store(0)
		currentLogPathMu.Lock()
		oldFile := currentLogFile
		currentLogPath = logPath
		currentLogFile = fd
		currentLogPathMu.Unlock()

		common.LogWriterMu.Lock()
		gin.DefaultWriter = io.MultiWriter(os.Stdout, fd)
		gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, fd)
		if oldFile != nil {
			_ = oldFile.Close()
		}
		common.LogWriterMu.Unlock()

		// 轮转后按保留策略清理旧文件。
		//
		// 放在这里而不是单开一个定时 loop：日志目录的增长**只发生在轮转时**
		// （每 100 万行新建一个文件），在轮转点清理与增长一一对应，
		// 既不需要额外 goroutine，也不会出现「长时间没轮转但定时器空跑」。
		// activePath 传入当前文件，保证它永不被删。
		cleanupOldLogFiles(*common.LogDir, logPath)
	}
}

func LogInfo(ctx context.Context, msg string) {
	logHelper(ctx, loggerINFO, msg)
}

func LogWarn(ctx context.Context, msg string, args ...any) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	logHelper(ctx, loggerWarn, msg)
}

func LogError(ctx context.Context, msg string) {
	logHelper(ctx, loggerError, msg)
}

func LogDebug(ctx context.Context, msg string, args ...any) {
	if common.DebugEnabled {
		if len(args) > 0 {
			msg = fmt.Sprintf(msg, args...)
		}
		logHelper(ctx, loggerDebug, msg)
	}
}

func logHelper(ctx context.Context, level string, msg string) {
	msg = common.MaskMessage(msg) // B1-1 密钥日志指纹化：出口单点脱敏
	var id any = "SYSTEM"
	if ctx != nil {
		if requestID := ctx.Value(common.RequestIdKey); requestID != nil {
			id = requestID
		}
	}
	now := time.Now()
	common.LogWriterMu.RLock()
	writer := gin.DefaultErrorWriter
	if level == loggerINFO {
		writer = gin.DefaultWriter
	}
	// Fprintf 的返回值就是**实际写入的字节数**，因此字节计数零额外开销
	// （不需要先格式化再量长度）。
	written, _ := fmt.Fprintf(writer, "[%s] %v | %s | %s \n", level, now.Format("2006/01/02 - 15:04:05"), id, msg)
	common.LogWriterMu.RUnlock()

	// 近似计数：用 atomic 避免数据竞争（无需精确，故不做 CAS 语义的严格递增）。
	if written > 0 {
		logBytes.Add(int64(written))
	}
	logCount.Add(1)

	// 轮转触发：**行数或字节数任一超限**（T6.3）。
	// 用 CAS 保证同一时刻只有一个轮转在进行（否则并发触发会开出多个新文件，
	// 后一个把前一个已写入的内容留在无人引用的文件里）。
	if shouldRotate(logCount.Load(), logBytes.Load(), maxLogCount, maxLogSizeBytes) &&
		setupLogWorking.CompareAndSwap(false, true) {
		logCount.Store(0)
		logBytes.Store(0)
		gopool.Go(func() {
			SetupLogger()
		})
	}
}

func LogQuota(quota int) string {
	// 新逻辑：根据额度展示类型输出
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f 额度", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f 额度", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d 点额度", quota)
	default: // USD
		return fmt.Sprintf("＄%.6f 额度", q/common.QuotaPerUnit)
	}
}

func FormatQuota(quota int) string {
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d", quota)
	default:
		return fmt.Sprintf("＄%.6f", q/common.QuotaPerUnit)
	}
}

// LogJson 仅供测试使用 only for test
func LogJson(ctx context.Context, msg string, obj any) {
	if !common.DebugEnabled {
		return
	}
	jsonStr, err := common.Marshal(obj)
	if err != nil {
		LogError(ctx, fmt.Sprintf("json marshal failed: %s", err.Error()))
		return
	}
	LogDebug(ctx, "%s | %s", msg, jsonStr)
}
