package training_capture

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// 增量去重（纯函数，无 IO）。
//
// ## 要解决的真实问题（用户原话）
//
// 「重点是我们这个要怎么判断用户新会话等等这些算法要搞定清楚，比如说有些长期会话的
//   代码等等那些的话就会发现很多 token 都一样的，这样我们就能智能识别增量添加」
//
// 长期会话的**历史轮次内容不会变**。若按「会话整体」判重，用户每发一条新消息，
// 整个会话的哈希都变 → 整段历史重传。这既浪费上行带宽（本站仅 75Mbps），
// 也让云盘被重复数据填满。
//
// ## 双层设计
//
//   - **会话水位**（`Watermark`）：记录「这个会话已经上传了多少条消息」
//   - **消息指纹**（`Fingerprint`）：每条消息内容的哈希
//
// 上传时只取水位**之后**的消息。用户说的「很多 token 都一样」正是
// **消息级判重能命中**的场景：历史消息指纹不变 → 不重传。
//
// ## 为什么水位用「计数 + 末次指纹」而不是「已上传 id 集合」
//
// 集合会随对话量**无限增长**（每个会话都要存全部消息 id）。而
// 「计数 + 末次指纹」是 O(1)：计数给出位置，末次指纹用于**校验水位是否仍然
// 自洽**（例如消息被插入或删除导致的错位），不一致时退化为保守的「重传该会话」
// ——宁可多传一次，也不丢新内容。

// Message 参与采集的一条对话消息（已剥离敏感字段的最小结构）。
type Message struct {
	// Role 消息角色（user / assistant / system）。
	Role string
	// Content 消息正文（**已经过字段白名单过滤**，不含密钥/令牌等）。
	Content string
}

// Watermark 某个会话的采集水位。
type Watermark struct {
	// UploadedCount 已上传的消息条数。下次从该下标继续。
	UploadedCount int
	// LastFingerprint 已上传的最后一条消息的指纹（用于校验水位自洽）。
	LastFingerprint string
}

// Fingerprint 计算一条消息的内容指纹。
//
// 规范化（trim + 折叠连续空白）后再哈希：
//   - **折叠空白**：客户端因换行/缩进差异重发同一段内容时，
//     不应被当成「新内容」重复上传（这在代码类对话里很常见，
//     正是用户点名的场景）。
//   - **保留 role**：同样的文本出现在 user 与 assistant 两侧语义不同，
//     必须区分。
//
// 用 `\x00` 作 role 与 content 的分隔符：它不可能出现在正常文本里，
// 避免「role=a, content=bc」与「role=ab, content=c」撞成同一指纹。
func Fingerprint(message Message) string {
	normalized := normalizeContent(message.Content)
	h := sha256.New()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(message.Role))))
	h.Write([]byte{0})
	h.Write([]byte(normalized))
	return hex.EncodeToString(h.Sum(nil))
}

// normalizeContent 折叠连续空白为单个空格并 trim。
//
// 只折叠**空白**，不改变其它字符——不能在采集侧做「语义改写」，
// 否则存进云盘的内容与用户真实对话不一致（训练数据的价值就在于真实）。
func normalizeContent(content string) string {
	var b strings.Builder
	b.Grow(len(content))
	lastWasSpace := false
	for _, r := range content {
		// 覆盖常见空白：空格、tab、换行、回车、以及 Unicode 空格类。
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' || r == ' ' || r == '　' {
			if !lastWasSpace {
				b.WriteByte(' ')
				lastWasSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastWasSpace = false
	}
	return strings.TrimSpace(b.String())
}

// SelectIncrement 决定本次应上传哪些消息。
//
// 返回 `(increment, nextWatermark)`。`increment` 为空表示无新增（不产生上传）。
//
// 行为契约：
//  1. 水位自洽（`watermark.UploadedCount` 在下标范围内且该位置之前的内容未变）
//     → 只取水位之后的消息。
//  2. 水位**自洽校验**：若 `UploadedCount > 0`，检查下标 `UploadedCount-1`
//     的消息指纹是否等于 `LastFingerprint`。**不等**说明历史被改动
//     （消息被删除/插入/编辑）→ 保守地**全量重传**（宁可多传，不丢内容）。
//  3. 首次采集（`UploadedCount == 0`）→ 全量。
//  4. 消息数**少于**水位（用户删了消息）→ 保守全量重传。
//  5. 水位内出现**重复指纹**的新消息（同一内容重复发送）→ 仍然上传
//     （重复是真实的对话行为，去重是「不重传历史」，不是「去重内容」）。
//     这一点是刻意的：训练数据里「用户重复问同一件事」是有价值的信息。
func SelectIncrement(messages []Message, watermark Watermark) ([]Message, Watermark) {
	if len(messages) == 0 {
		return nil, watermark
	}

	// 水位异常（超出范围 / 为负）→ 保守全量。
	if watermark.UploadedCount < 0 || watermark.UploadedCount > len(messages) {
		return messages, watermarkAfter(messages)
	}

	if watermark.UploadedCount == 0 {
		return messages, watermarkAfter(messages)
	}

	// 水位自洽校验：末次指纹必须与下标处一致。
	// 不一致 = 历史被改动，保守全量重传。
	if watermark.LastFingerprint != "" {
		actual := Fingerprint(messages[watermark.UploadedCount-1])
		if actual != watermark.LastFingerprint {
			return messages, watermarkAfter(messages)
		}
	}

	increment := messages[watermark.UploadedCount:]
	if len(increment) == 0 {
		// 无新增：水位不变（不重建，避免无意义写）。
		return nil, watermark
	}
	return increment, watermarkAfter(messages)
}

// watermarkAfter 由完整消息列表构造新水位。
func watermarkAfter(messages []Message) Watermark {
	if len(messages) == 0 {
		return Watermark{}
	}
	return Watermark{
		UploadedCount:   len(messages),
		LastFingerprint: Fingerprint(messages[len(messages)-1]),
	}
}

// GlobalDedup 在**无会话标识**时的退化去重：按内容指纹过滤掉已上传过的消息。
//
// 用于拿不到会话 id 的格式（见 plan §10.3 的诚实边界）。
// 与 SelectIncrement 的差别：它不保证「只传增量」，只保证
// **同一内容不会被重复上传**，代价是需要维护一个已传指纹集合。
//
// `seen` 由调用方持有并**有界**（见 BoundedFingerprintSet），
// 本函数不负责容量管理。
func GlobalDedup(messages []Message, seen *BoundedFingerprintSet) []Message {
	if len(messages) == 0 || seen == nil {
		return nil
	}
	out := make([]Message, 0, len(messages))
	for _, m := range messages {
		fp := Fingerprint(m)
		if seen.Has(fp) {
			continue
		}
		seen.Add(fp)
		out = append(out, m)
	}
	return out
}
