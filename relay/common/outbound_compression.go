package common

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/logger"

	"github.com/gin-gonic/gin"
)

// MaybeCompressOutboundBody gzip-compresses the outbound request body when the
// channel opts in (ChannelSetting.RequestCompression) and the body is large
// enough to be worth it (>= RelayRequestCompressionThresholdKB).
//
// 动机：生产 5Mbps 上行是首字延迟的根因。网关把请求体完整上传上游时，出口带宽
// 被打满，frt（FirstResponseTime-StartTime）与 request_bytes 单调正相关
// （实测 >20MB → 200s+）。JSON 文本可压到 ~1%，显著缩短上传时间。
//
// 契约：
//   - 返回 (body, compressed, closer)。compressed=true 时调用方必须在请求
//     结束后 closer.Close()，并给上游请求设置 Content-Encoding: gzip。
//   - fail-open 且**永不返回 nil body**：任何前置条件不满足、压缩失败、或压缩后
//     不小于原文时，都回退为可用的明文 body（compressed=false，closer=nil）——
//     绝不因压缩失败而拒绝请求或发出空 body。
//   - 返回的 body 可被 net/http 安全重放（HTTP/2 透明重试）：压缩体是
//     ReplayableBody；明文回退是 *bytes.Reader（net/http 自动为其生成 GetBody）。
//
// 仅网关自行序列化的 JSON 请求体走此路径；multipart/表单/websocket 不涉及。
func MaybeCompressOutboundBody(c *gin.Context, info *RelayInfo, body io.Reader) (io.Reader, bool, io.Closer) {
	if body == nil || info == nil {
		return body, false, nil
	}
	if !common.RelayRequestCompressionEnabled {
		return body, false, nil
	}
	if info.ChannelMeta == nil || !info.ChannelSetting.RequestCompression {
		return body, false, nil
	}

	threshold := int64(common.RelayRequestCompressionThresholdKB) << 10

	// 可回放 body：已知大小，可从独立 reader 流式压缩，原 body 保持可用以便
	// 压缩无收益/失败时原样转发（不额外拷贝）。
	if replayable, ok := body.(common.ReplayableBody); ok {
		size := replayable.Size()
		if threshold > 0 && size < threshold {
			return body, false, nil
		}
		reader, err := replayable.NewReader()
		if err != nil {
			logCompressionWarn(c, fmt.Sprintf("request compression: cannot open replay reader, sending uncompressed: %v", err))
			return body, false, nil
		}
		compressed, err := gzipBytes(reader)
		_ = reader.Close()
		if err != nil {
			logCompressionWarn(c, fmt.Sprintf("request compression: gzip failed, sending uncompressed: %v", err))
			return body, false, nil
		}
		if int64(len(compressed)) >= size {
			// 压缩无收益（已压缩内容/极小 body）：保持明文。
			return body, false, nil
		}
		compressedBody, closer, ok := storeCompressedBody(c, compressed, size)
		if !ok {
			// 存储失败：原 body 仍完整可用，原样转发。
			return body, false, nil
		}
		return compressedBody, true, closer
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
		return body, false, nil
	}
	if threshold > 0 && int64(len(raw)) < threshold {
		return bytes.NewReader(raw), false, nil
	}
	compressed, err := gzipBytes(bytes.NewReader(raw))
	if err != nil || len(compressed) >= len(raw) {
		return bytes.NewReader(raw), false, nil
	}
	compressedBody, closer, ok := storeCompressedBody(c, compressed, int64(len(raw)))
	if !ok {
		return bytes.NewReader(raw), false, nil
	}
	return compressedBody, true, closer
}

// storeCompressedBody 把压缩后的字节封装成可回放 body。返回 ok=false 表示
// 存储不可用（调用方据此回退明文），此时 body 与 closer 均为 nil，不会泄漏。
func storeCompressedBody(c *gin.Context, compressed []byte, originalSize int64) (io.Reader, io.Closer, bool) {
	storage, err := common.CreateBodyStorage(compressed)
	if err != nil {
		logCompressionWarn(c, fmt.Sprintf("request compression: cannot store compressed body, sending uncompressed: %v", err))
		return nil, nil, false
	}
	if c != nil {
		logger.LogInfo(c, fmt.Sprintf(
			"request compression: gzip %d -> %d bytes (%.1f%%), outbound Content-Encoding: gzip",
			originalSize, storage.Size(), 100*float64(storage.Size())/float64(originalSize)))
	}
	return common.NewReplayableBodyReader(storage), storage, true
}

// logCompressionWarn 容忍 nil gin.Context：生产总是传入真实 context，但测试
// 或未来调用方可能传 nil，typed-nil 指针经 context.Context 接口后会让
// ctx.Value panic，故统一守卫。
func logCompressionWarn(c *gin.Context, msg string) {
	if c != nil {
		logger.LogWarn(c, msg)
	}
}

// gzipBytes 以 BestSpeed 级别压缩（首字延迟敏感，吞吐优先于压缩率）。
func gzipBytes(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
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
