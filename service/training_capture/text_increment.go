package training_capture

import (
	"unicode/utf8"
)

// 针对**单块文本**的增量（Spec-008 实际数据形态）。
//
// ## 为什么需要它（这是实测发现的设计缺口）
//
// 请求路径只能拿到 `TokenCountMeta.CombineText` —— 一个把整段对话
// 拼起来的字符串（见 hotpath.go）。因此**每次请求只产生 1 条消息**，
// 而不是逐轮的多条消息。
//
// 这与 `SelectIncrement` 的假设不符：它按**消息条数**做水位，
// 消息数恒为 1 时水位恒为 1，增量永远是「整段文本」——
// 去重完全失效，正是用户点名要避免的（「长期会话很多 token 都一样，
// 要智能识别增量添加」）。
//
// 所以这里按**文本前缀**做增量：新文本若以已上传前缀开头，
// 只上传**追加的那一段**。
//
// ## 安全性：为什么这不会丢内容
//
// 只在「新文本以旧文本为前缀」时才裁剪。这个条件下
// `new = old + suffix` 是**恒等式**，裁剪不丢任何字符。
// 任何不满足的情况（中途编辑、截断、完全换了内容）一律**全量重传**。
//
// 这是「宁可多传，绝不丢」原则在文本粒度上的落实。

// TextWatermark 单块文本的增量水位。
type TextWatermark struct {
	// UploadedBytes 已上传的**字节数**（下次从该偏移继续）。
	UploadedBytes int
	// PrefixHash 已上传前缀的指纹（用于校验前缀未被改动）。
	PrefixHash string
}

// SelectTextIncrement 决定本次应上传的文本增量。
//
// 返回 `(suffix, next)`；`suffix` 为空表示无新增（不产生上传）。
//
// 行为契约：
//  1. 首次（`UploadedBytes == 0`）→ 全量。
//  2. 前缀校验通过（旧前缀的哈希未变 + 新文本确实以旧前缀开头）
//     → 只返回 `new[UploadedBytes:]`。
//  3. 前缀校验**失败**（历史被编辑/截断）→ 全量重传。
//  4. 新文本比水位**短**（内容被删）→ 全量重传。
func SelectTextIncrement(text string, prev TextWatermark) (string, TextWatermark) {
	if text == "" {
		return "", prev
	}

	// 首次采集或水位异常 → 全量。
	if prev.UploadedBytes <= 0 {
		return text, textWatermarkAfter(text)
	}
	// 水位超出当前文本（内容变短/被清空）→ 保守全量。
	if prev.UploadedBytes > len(text) {
		return text, textWatermarkAfter(text)
	}

	// 前缀必须**逐字节一致**：既校验长度也校验内容。
	// 只比长度不够 —— 同样长度的不同内容会被误判为「未改动」，
	// 从而把被编辑掉的部分永久丢弃（B3 那类静默丢数据）。
	prefix := text[:prev.UploadedBytes]
	if prev.PrefixHash != "" && HashText(prefix) != prev.PrefixHash {
		// 历史被改动 → 保守全量（绝不裁剪，裁剪会丢被改动的内容）。
		return text, textWatermarkAfter(text)
	}

	suffix := text[prev.UploadedBytes:]
	if suffix == "" {
		// 无新增：水位不变（不重建，避免无意义写）。
		return "", prev
	}
	return suffix, textWatermarkAfter(text)
}

// textWatermarkAfter 由完整文本构造新水位。
func textWatermarkAfter(text string) TextWatermark {
	if text == "" {
		return TextWatermark{}
	}
	return TextWatermark{
		UploadedBytes: len(text),
		PrefixHash:    HashText(text),
	}
}

// HashText 计算文本的稳定指纹（与 Fingerprint 同一算法家族）。
//
// 单独暴露而不是让调用方自己拼 `Message`：水位校验只在
// 「同一个算法」下才有意义，散落各处容易漂移。
func HashText(text string) string {
	return Fingerprint(Message{Role: "text", Content: text})
}

// splitTextSuffix 把「追加的一段文本」按**行边界**切分成不超过 maxBytes 的块。
//
// 与 `splitBySize` 同样的理由：不能把一行从中间切断 ——
// 但对文本增量而言，更实际的理由是**保持可读性与可拼接性**。
// 单独成函数是因为输入是「纯文本后缀」而非 JSONL，
// 允许在换行处切分（JSONL 侧必须保证每条记录完整，要求更强）。
func splitTextSuffix(text string, maxBytes int) []string {
	if text == "" {
		return nil
	}
	if maxBytes <= 0 || len(text) <= maxBytes {
		return []string{text}
	}

	var chunks []string
	start := 0
	for start < len(text) {
		end := start + maxBytes
		if end >= len(text) {
			chunks = append(chunks, text[start:])
			break
		}
		// 从 end 往回找最近的换行，避免在行中间切断。
		cut := end
		for cut > start && text[cut-1] != '\n' {
			cut--
		}
		if cut == start {
			// 单行超长：只能在合法 UTF-8 边界硬切（不能死循环）。
			cut = utf8SafeCut(text, end)
		}
		chunks = append(chunks, text[start:cut])
		start = cut
	}
	return chunks
}

// utf8SafeCut 返回 <= limit 且不切断 UTF-8 字符的最大位置。
func utf8SafeCut(text string, limit int) int {
	if limit >= len(text) {
		return len(text)
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return cut
}
