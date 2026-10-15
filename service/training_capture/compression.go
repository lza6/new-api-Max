package training_capture

import (
	"bytes"
	"compress/gzip"
	"io"
)

// 分片压缩（增量需求 B2）。
//
// ## 为什么必须压缩
//
// 用户指出单条 64KB 截断不合理：「遇到二三十 MB 的可以压缩完再上传」。
// JSON 对话文本的 gzip 压缩率实测在 1/5 ~ 1/10（自然语言 + 重复的
// 结构性 token），压缩让：
//   - 单分片承载 20~40MB 原始对话（而不是靠截断丢弃）；
//   - 上行带宽占用降一个量级 —— 上行是全站共享的，
//     省带宽就是直接保护用户请求延迟（宪法 V）；
//   - 云盘文件数减少（每分片更大）。
//
// ## 格式与诚实边界
//
// 云盘上的分片是 **gzip 压缩的 JSONL**（扩展名 .jsonl.gz）。
// 任何消费方（训练管线、管理端分析）都必须先解压 ——
// 这是显式契约，不是巧合：文件名后缀就是声明。

// gzipCompress 把 payload 压成 gzip。
//
// BestSpeed：压缩发生在 worker goroutine（不占请求路径），但仍不宜用
// 最高档 —— 一个 30MB 文本用 BestCompression 要数百毫秒到秒级，
// 期间 worker 无法消费队列，积压会放大丢弃率。BestSpeed 对 JSON 文本
// 仍能达到 ~1/5 的典型压缩率，是吞吐与压缩率的平衡点。
func gzipCompress(payload []byte) ([]byte, error) {
	var buf bytes.Buffer
	// 预估容量：典型 JSON 压缩率 ~1/5，避免反复扩容。
	buf.Grow(len(payload)/5 + 512)
	w, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(payload); err != nil {
		return nil, err
	}
	// Close 必须调用：它会 flush 尾块；不 Close 的 gzip 流是损坏的。
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gzipDecompress 解压分片（管理端分析 / 读取侧用）。
//
// 用 io.LimitReader 防 zip-bomb：一个恶意构造的 gzip 文件解压后可以是
// 无界的（解压比可达 1000:1 以上）。读上限取「单分片原始上限的 4 倍」，
// 正常数据远达不到；超过即报错 —— 拒绝解压比 OOM 好。
func gzipDecompress(data []byte, maxPlain int64) ([]byte, error) {
	if maxPlain <= 0 {
		maxPlain = int64(MaxPartBytes) * 8
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	plain, err := io.ReadAll(io.LimitReader(zr, maxPlain+1))
	if err != nil {
		return nil, err
	}
	if int64(len(plain)) > maxPlain {
		return nil, ErrDecompressTooLarge
	}
	return plain, nil
}
