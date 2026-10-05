package common

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/setting/relay_setting"

	"github.com/gin-gonic/gin"
)

// 出站压缩累积统计（进程内原子计数，热路径零锁）。
//
// [功能] 管理员面板展示「累计压缩了多少字节 / 节省了多少带宽 / 花了多少时间」。仅累加
// **成功压缩**的请求；未压缩/压缩无收益的请求不计。多实例各自独立（与既有进程内指标
// 语义一致）。
var (
	compressionTotalOriginalBytes   atomic.Int64
	compressionTotalCompressedBytes atomic.Int64
	compressionTotalCount           atomic.Int64
	compressionTotalMs              atomic.Int64
)

// CompressionTotals 出站压缩累积统计快照。
type CompressionTotals struct {
	Count           int64 `json:"count"`            // 成功压缩的请求数
	OriginalBytes   int64 `json:"original_bytes"`   // 压缩前字节合计
	CompressedBytes int64 `json:"compressed_bytes"` // 压缩后字节合计
	SavedBytes      int64 `json:"saved_bytes"`      // 节省字节 = 原始 - 压缩后
	TotalMs         int64 `json:"total_ms"`         // 压缩累计耗时（毫秒）
}

// RecordCompression 累加一次成功压缩（供实时面板/统计）。
func RecordCompression(originalBytes, compressedBytes, compressMs int64) {
	if originalBytes <= 0 || compressedBytes < 0 {
		return
	}
	compressionTotalOriginalBytes.Add(originalBytes)
	compressionTotalCompressedBytes.Add(compressedBytes)
	compressionTotalCount.Add(1)
	if compressMs > 0 {
		compressionTotalMs.Add(compressMs)
	}
}

// GetCompressionTotals 返回出站压缩累积统计快照。
func GetCompressionTotals() CompressionTotals {
	orig := compressionTotalOriginalBytes.Load()
	comp := compressionTotalCompressedBytes.Load()
	saved := orig - comp
	if saved < 0 {
		saved = 0
	}
	return CompressionTotals{
		Count:           compressionTotalCount.Load(),
		OriginalBytes:   orig,
		CompressedBytes: comp,
		SavedBytes:      saved,
		TotalMs:         compressionTotalMs.Load(),
	}
}

// MaybeCompressOutboundBody gzip-compresses the outbound request body when the
// channel opts in (ChannelSetting.RequestCompression) and the body is large
// enough to be worth it (>= RelayRequestCompressionThresholdKB).
//
// 动机：生产 5Mbps 上行是首字延迟的根因。网关把请求体完整上传上游时，出口带宽
// 被打满，frt（FirstResponseTime-StartTime）与 request_bytes 单调正相关
// （实测 >20MB → 200s+）。JSON 文本可压到 ~1%，显著缩短上传时间。
//
// 契约：
//   - 返回 (body, compressed, closer, originalBytes, outBytes)。compressed=true 时
//     调用方必须在请求结束后 closer.Close()，并给上游请求设置 Content-Encoding: gzip。
//   - originalBytes/outBytes 是**本函数实测**的请求体字节（压缩前/实际发往上游），
//     供实时面板与日志展示；无法测得时为 -1（调用方据此跳过展示，避免显示 0/Infinity）。
//   - fail-open 且**永不返回 nil body**：任何前置条件不满足、压缩失败、或压缩后
//     不小于原文时，都回退为可用的明文 body（compressed=false，closer=nil）——
//     绝不因压缩失败而拒绝请求或发出空 body。
//   - 返回的 body 可被 net/http 安全重放（HTTP/2 透明重试）：压缩体是
//     ReplayableBody；明文回退是 *bytes.Reader（net/http 自动为其生成 GetBody）。
//
// 仅网关自行序列化的 JSON 请求体走此路径；multipart/表单/websocket 不涉及。
func MaybeCompressOutboundBody(c *gin.Context, info *RelayInfo, body io.Reader) (io.Reader, bool, io.Closer, int64, int64) {
	if body == nil || info == nil {
		return body, false, nil, -1, -1
	}
	// 开关与阈值来源：管理员热更新配置（relay_setting，走不可变快照，无锁读）
	// 优先；未配置时回退 env（common.RelayRequestCompression*）。env 仍作为
	// 部署级 kill-switch，任一为关即不压缩。
	if !common.RelayRequestCompressionEnabled || !relay_setting.GetRequestCompressionEnabled() {
		return body, false, nil, -1, -1
	}
	if info.ChannelMeta == nil || !channelCompressionEnabled(info.ChannelId, info.ChannelSetting.RequestCompression) {
		return body, false, nil, -1, -1
	}

	threshold := int64(relay_setting.GetRequestCompressionThresholdKB()) << 10
	level := relay_setting.GetRequestCompressionLevel()
	// 上限：超过则不压缩（护 CPU）。超大 body 压缩极耗 CPU，明文直发更快。
	maxBytes := int64(relay_setting.GetRequestCompressionMaxMB()) << 20

	// 可回放 body：已知大小，可从独立 reader 流式压缩，原 body 保持可用以便
	// 压缩无收益/失败时原样转发（不额外拷贝）。
	if replayable, ok := body.(common.ReplayableBody); ok {
		size := replayable.Size()
		if threshold > 0 && size < threshold {
			return body, false, nil, size, size
		}
		if maxBytes > 0 && size > maxBytes {
			// 超上限：不压缩（护 CPU），明文转发。
			logCompressionWarn(c, fmt.Sprintf("request compression: body %d bytes exceeds max %d, sending uncompressed", size, maxBytes))
			return body, false, nil, size, size
		}
		reader, err := replayable.NewReader()
		if err != nil {
			logCompressionWarn(c, fmt.Sprintf("request compression: cannot open replay reader, sending uncompressed: %v", err))
			return body, false, nil, size, size
		}
		compStart := time.Now()
		compressed, err := gzipBytes(reader, level)
		compressMs := time.Since(compStart).Milliseconds()
		_ = reader.Close()
		if err != nil {
			logCompressionWarn(c, fmt.Sprintf("request compression: gzip failed, sending uncompressed: %v", err))
			return body, false, nil, size, size
		}
		if int64(len(compressed)) >= size {
			// 压缩无收益（已压缩内容/极小 body）：保持明文。
			return body, false, nil, size, size
		}
		compressedBody, closer, ok := storeCompressedBody(c, info, compressed, size, compressMs)
		if !ok {
			// 存储失败：原 body 仍完整可用，原样转发。
			return body, false, nil, size, size
		}
		return compressedBody, true, closer, size, int64(len(compressed))
	}

	// 非可回放 body：先整体读入内存。此类 body 本就在内存中（如透传重序列化后的
	// *bytes.Reader），读取成本可忽略。加 MaxRequestBodyMB 上界作防御——即便未来
	// 传入无界 reader，也不会把整个流读进内存。
	// 未压缩时直接返回 *bytes.Reader —— net/http 会为其自动设置 ContentLength
	// 与 GetBody（无需自建 storage，避免内存/磁盘缓存计数与文件泄漏）。
	maxBodyMB := constant.MaxRequestBodyMB
	if maxBodyMB <= 0 {
		maxBodyMB = 128 // 与 InitEnv 默认一致；未跑 InitEnv 的测试/路径兜底
	}
	limit := (int64(maxBodyMB) << 20) + 1
	raw, err := io.ReadAll(io.LimitReader(body, limit))
	if err != nil {
		logCompressionWarn(c, fmt.Sprintf("request compression: cannot read body, sending uncompressed: %v", err))
		return body, false, nil, -1, -1
	}
	originalLen := int64(len(raw))
	if threshold > 0 && originalLen < threshold {
		return bytes.NewReader(raw), false, nil, originalLen, originalLen
	}
	compStart := time.Now()
	compressed, err := gzipBytes(bytes.NewReader(raw), level)
	compressMs := time.Since(compStart).Milliseconds()
	if err != nil || len(compressed) >= len(raw) {
		return bytes.NewReader(raw), false, nil, originalLen, originalLen
	}
	compressedBody, closer, ok := storeCompressedBody(c, info, compressed, originalLen, compressMs)
	if !ok {
		return bytes.NewReader(raw), false, nil, originalLen, originalLen
	}
	return compressedBody, true, closer, originalLen, int64(len(compressed))
}

// storeCompressedBody 把压缩后的字节封装成可回放 body。返回 ok=false 表示
// 存储不可用（调用方据此回退明文），此时 body 与 closer 均为 nil，不会泄漏。
// compressMs 为本次压缩耗时（毫秒），写入 RelayInfo 供面板/日志/AB 观测。
func storeCompressedBody(c *gin.Context, info *RelayInfo, compressed []byte, originalSize, compressMs int64) (io.Reader, io.Closer, bool) {
	storage, err := common.CreateBodyStorage(compressed)
	if err != nil {
		logCompressionWarn(c, fmt.Sprintf("request compression: cannot store compressed body, sending uncompressed: %v", err))
		return nil, nil, false
	}
	if info != nil {
		info.RequestCompressionMs = compressMs
		info.RequestOriginalBytes = originalSize
		info.RequestCompressedBytes = storage.Size()
	}
	if c != nil {
		logger.LogInfo(c, fmt.Sprintf(
			"request compression: gzip %d -> %d bytes (%.1f%%) in %dms, outbound Content-Encoding: gzip",
			originalSize, storage.Size(), 100*float64(storage.Size())/float64(originalSize), compressMs))
	}
	// 累积统计：仅计成功压缩的请求（面板展示累计压缩字节与节省带宽）。
	RecordCompression(originalSize, storage.Size(), compressMs)
	// §按模型压缩统计（持久化）：经回调上报，避免 relay/common → model 依赖。
	if ModelCompressionRecorder != nil && info != nil {
		ModelCompressionRecorder(info.OriginModelName, originalSize, storage.Size())
	}
	return common.NewReplayableBodyReader(storage), storage, true
}

// ModelCompressionRecorder 由 service 层注册，按模型累积压缩统计（持久化）。
// nil = 不启用（默认零行为）。
var ModelCompressionRecorder func(modelName string, originalBytes, compressedBytes int64)

// logCompressionWarn 容忍 nil gin.Context：生产总是传入真实 context，但测试
// 或未来调用方可能传 nil，typed-nil 指针经 context.Context 接口后会让
// ctx.Value panic，故统一守卫。
func logCompressionWarn(c *gin.Context, msg string) {
	if c != nil {
		logger.LogWarn(c, msg)
	}
}

// gzipBytes 以给定级别压缩。level 由 relay_setting.GetRequestCompressionLevel()
// 提供（默认 DefaultCompression=6）。BestSpeed(1) 吞吐优先但压缩率低，Default(6)
// 对可压内容显著更优（实测代码类 0.6%→0.3%），CPU 代价在 2C2G 上可接受（大 body
// 压缩仅百 ms 级）。level 越界时回退 DefaultCompression。
func gzipBytes(r io.Reader, level int) ([]byte, error) {
	if level < gzip.HuffmanOnly || level > gzip.BestCompression {
		level = gzip.DefaultCompression
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(zw, r); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
