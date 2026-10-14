package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 日志按大小分片（T6.3）。
//
// ## 为什么需要（实测缺口）
//
// 原实现只在**行数**超过 100 万时轮转（`maxLogCount`），且进程启动时也会
// 开一个新文件。问题在于：行数阈值与**字节数**没有关系 ——
// 一行日志可以是 80 字节，也可以是一条 200KB 的上游错误体。
// 于是 100 万行可能只有 80MB，也可能有 200GB。
// 生产实测日志目录曾达 865MB 且全是日志，是磁盘占用的真凶。
//
// 因此补一个**字节数**阈值，与行数阈值**同时**生效（任一触发即轮转）。
//
// ## 命名：为什么不能只靠时间戳
//
// 原命名是 `oneapi-<YYYYMMDDHHmmss>.log`，**秒级精度**。
// 一旦按大小触发轮转，两次轮转完全可能落在同一秒内
// （例如阈值很小、或日志突然爆发）—— 那时文件名相同，
// `os.OpenFile(..., O_APPEND)` 会**继续写同一个文件**，
// 「轮转」就静默失效了（文件继续长大，而日志里看不出异常）。
//
// 所以这里做**唯一性校验**：目标名已存在就追加序号
// （`oneapi-20261015123456.1.log`），仍满足 `isManagedLogFile()`
// 的前缀/后缀约束，因此仍受 `retention.go` 的保留策略管辖。

const (
	// defaultLogMaxSizeMB 单个日志文件的默认大小上限（MB）。
	//
	// 取 256：与 `retention.go` 的总量上限 1GB 配合，
	// 单文件不超过总量的 1/4，不会出现「一个大文件挤掉其它所有历史」。
	defaultLogMaxSizeMB = 256

	// envLogMaxSizeMB 覆盖默认值的环境变量名。
	//
	// 命名与项目既有环境变量风格一致（全大写下划线）。
	envLogMaxSizeMB = "LOG_MAX_SIZE_MB"

	// maxLogSizeSuffixAttempts 唯一化时最多尝试的序号数。
	//
	// 上限存在的意义：目录里堆积了大量同秒文件时不要在命名上死循环。
	// 达到上限后进入纳秒兜底分支（见 nextLogFileName）。
	maxLogSizeSuffixAttempts = 256

	// maxLogSizeFallbackAttempts 纳秒兜底分支的尝试上限。
	//
	// 取得很大：正常情况下一次就中（序号 0 空闲）。这个上限只是
	// 「exists 恒为 true」这种病态输入的最后防线。
	maxLogSizeFallbackAttempts = 1 << 16
)

// resolveMaxLogSizeBytes 读环境变量得到单文件大小上限（字节）。
//
// 无法解析或非正数时回退默认值 —— 配置写错不该让日志系统失效，
// 更不该因为拿到 0 而变成「每条日志都轮转」。
func resolveMaxLogSizeBytes() int64 {
	raw := strings.TrimSpace(os.Getenv(envLogMaxSizeMB))
	mb := defaultLogMaxSizeMB
	if raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			mb = v
		}
	}
	return int64(mb) << 20
}

// shouldRotate 判断是否该轮转（**纯函数**，便于穷举边界）。
//
// 两个阈值任一触发即轮转：
//   - 行数超限：保持原行为，避免「日志行很短时单个文件堆积过多行」
//     （行数过多会让 `grep`/编辑器变得难用）；
//   - 字节数超限：新增，防止「单行很大时文件爆炸」。
//
// maxLines/maxBytes <= 0 视为该项**不限制**（允许运维单独关掉一个维度，
// 而不是因为传 0 就每行都轮转）。
func shouldRotate(lineCount, bytesWritten, maxLines, maxBytes int64) bool {
	if maxLines > 0 && lineCount > maxLines {
		return true
	}
	if maxBytes > 0 && bytesWritten > maxBytes {
		return true
	}
	return false
}

// nextLogFileName 生成一个**在 dir 内唯一**的日志文件名。
//
// 形如 `oneapi-20261015123456.log`；若已存在则依次尝试
// `oneapi-20261015123456.1.log`、`.2`…… 直到唯一。
//
// exists 由调用方注入（便于测试不碰文件系统）；传 nil 时用 os.Stat。
func nextLogFileName(dir string, now time.Time, exists func(path string) bool) string {
	if exists == nil {
		exists = func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		}
	}
	base := fmt.Sprintf("oneapi-%s", now.Format("20060102150405"))
	candidate := filepath.Join(dir, base+logFileSuffix)
	if !exists(candidate) {
		return candidate
	}
	for i := 1; i <= maxLogSizeSuffixAttempts; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s.%d%s", base, i, logFileSuffix))
		if !exists(candidate) {
			return candidate
		}
	}

	// 兜底：纳秒精度 + 递增序号。
	//
	// [修复防御] 原实现只换了纳秒时间戳就断言「必然唯一」——**那是错的**：
	// 只要调用方传入的 `now` 相同（例如同一时刻批量重建、或调用方
	// 复用了一个时间值），纳秒格式化结果就完全一样，仍然撞名。
	// 撞名的后果是 O_APPEND 继续写原文件，轮转静默失效。
	// 因此这里也**必须**逐一校验，而不是假设时间戳天然唯一。
	nanoBase := fmt.Sprintf("oneapi-%s", now.Format("20060102150405.000000000"))
	for i := 0; i <= maxLogSizeFallbackAttempts; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", nanoBase, i, logFileSuffix))
		if !exists(candidate) {
			return candidate
		}
	}

	// 走到这里说明 exists 对所有候选都返回 true（病态：目录里已有
	// 六万多个同纳秒文件）。此时返回任一个都会覆盖数据，故显式告警 ——
	// 不静默返回一个已知冲突的名字。
	log.Printf("logger: could not find a unique log file name in %s after %d attempts; "+
		"returning %s which may already exist", dir, maxLogSizeFallbackAttempts, candidate)
	return candidate
}
