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
// 无界的（解压比可达 1000:1 以上）。超过上限即报错 ——
// 拒绝解压比 OOM 好。
//
// maxPlain <= 0 时走 `EffectiveDecompressMaxPlainBytes()`（按**当前预算**
// 推导，见该函数说明）；**不再退回编译期常量**。
func gzipDecompress(data []byte, maxPlain int64) ([]byte, error) {
	if maxPlain <= 0 {
		maxPlain = EffectiveDecompressMaxPlainBytes()
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

// decompressPlainMultiplier 解压上限相对**单分片原始上限**的倍数。
//
// 8 倍：正常分片解压后绝不会超过自己的原始上限（分片就是按这个上限切的），
// 留 8 倍余量是为了容忍「分片是旧上限切的、期间上限被调小」这类跨版本场景。
// 仍然有界，zip-bomb 照挡。
const decompressPlainMultiplier = 8

// EffectiveDecompressMaxPlainBytes 解压后明文的上限（按当前预算推导）。
//
// ## 为什么不能用编译期常量
//
// 旧实现写死 `MaxPartBytes(2MB) * 8 = 16MB`。但**分片上限是自适应的**：
// `CAPTURE_MAX_PART_MB` 可调，且 `deriveLimits` 还受内存预算与
// 「分片上限 ≥ 文本上限 × 8」这条不变量影响（内存大的机器上可到 128MB+）。
//
// 后果是双向的、且都在生产才暴露：
//   - 上限被调大 → 分片原始大小超过 16MB → **管理端预览与分析永远解压失败**，
//     报 ErrDecompressTooLarge，而数据本身完全正常（看起来像数据损坏）；
//   - 上限被调小 → 白白放宽了 zip-bomb 的窗口，多算出来的余量没有任何用处。
//
// 现在与 `EffectiveMaxPartRawBytes` 同源，读写两侧的上限始终一致。
func EffectiveDecompressMaxPlainBytes() int64 {
	base := EffectiveMaxPartRawBytes()
	if base <= 0 {
		// 理论上不可达（EffectiveMaxPartRawBytes 自身有兜底推导），
		// 但仍给一个正数，避免 maxPlain<=0 把防 bomb 的闸门整个关掉。
		base = MaxPartBytes
	}
	return int64(base) * decompressPlainMultiplier
}

// DecompressPart 解压一个分片（导出给接口层用）。
//
// 上限取 `EffectiveDecompressMaxPlainBytes()`（随预算自适应），
// 超限即 ErrDecompressTooLarge（防 zip-bomb）。
func DecompressPart(data []byte) ([]byte, error) {
	return gzipDecompress(data, EffectiveDecompressMaxPlainBytes())
}

// CompressPartForTest 暴露压缩给跨包测试（controller 层要造真实的
// gzip 分片 fixture）。生产代码路径请用 gzipCompress。
func CompressPartForTest(payload []byte) ([]byte, error) {
	return gzipCompress(payload)
}
