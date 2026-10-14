package training_capture

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/relaykit/types"
)

// 请求热路径的采集入口。
//
// ## 这个文件存在的唯一理由：把「不该在热路径做的事」挡在外面
//
// 采集功能最大的风险不是采不到数据，而是**拖慢用户请求**（用户明确要求）。
// 因此热路径上只允许做三件事：
//
//  1. 读一个原子布尔量，短路掉绝大多数请求（未开启时开销 ≈ 两次原子读）；
//  2. 从**调用方已经算好的** `TokenCountMeta` 里取文本（不重新解析 JSON）；
//  3. 一次非阻塞入队（见 queue.go，实测 1.93µs）。
//
// 明确**不做**的事（都放在 worker 里）：
//   - 不查 DB（授权走缓存、水位由 worker 读）
//   - 不查用户/渠道/令牌
//   - 不解析请求体 JSON
//   - 不写日志
//
// ## 关于「拿不到结构化消息」这一事实（重要，影响数据形态）
//
// 网关的 `dto.Request` 接口只暴露 `GetTokenCountMeta()`，它把所有格式
// （OpenAI / Claude / Gemini / Responses）的对话压成**一个拼接字符串**
// `CombineText`，并且把 role、name、tool 定义、文本**混在一起**：
//
//	texts = append(texts, message.Role)
//	texts = append(texts, *message.Name)
//	texts = append(texts, m.Text)
//	texts = append(texts, tool.Function.Name, tool.Function.Description, ...)
//
// 这个拼接是**不可逆**的：无法从它还原出「哪段是 user、哪段是 assistant」。
// 要拿到真正的结构化消息，唯一办法是给 `dto.Request` 加一个返回
// `[]Message` 的方法，并在 **12+ 个请求类型**上各实现一遍
// （GeneralOpenAIRequest / ClaudeRequest / GeminiChatRequest / …）——
// 那是一次跨格式的大改动，且每个新格式都要记得实现（漏了就静默少采）。
//
// 所以这里**如实降级**，而不是假装采到了结构化对话：
//
//   - 采到的是一条 `role="request"` 的整体文本（`CombineText`），
//     加上一条 `role="assistant"` 的回复（若调用方能提供）。
//   - 增量的粒度因此是**「这次请求的拼接头 vs 上次」**，
//     对「长期会话只传增量」依然有效（拼接串前缀稳定），
//     但**不能**得到逐轮 role 标注的训练样本。
//
// 这个取舍写在 spec 的诚实边界里，不隐藏。若将来确实需要逐轮标注，
// 正确做法是扩展 `dto.Request`（见 ADR-008-9），而不是在这里猜结构。

// captureHotPathEnabled 进程级快速开关。
//
// 由 `MaybeCapture` 之外的地方维护（授权/总开关变化时更新），
// 目的：让「采集未开启」这个最常见情况下，热路径只付一次原子读。
//
// **它只是优化，不是正确性保证**：即使读到过期的 true，
// 后面 `IsGranted`（内部含总开关与云盘判定）仍会拦下来。
var captureHotPathEnabled atomic.Bool

// SetHotPathEnabled 由管理端/启动逻辑调用，与总开关状态保持同步。
func SetHotPathEnabled(enabled bool) {
	captureHotPathEnabled.Store(enabled)
}

// HotPathEnabled 报告快速开关状态（供测试与自检）。
func HotPathEnabled() bool {
	return captureHotPathEnabled.Load()
}

// SetHotPathEnabledForTest 测试用覆盖，返回恢复函数。
// 生产代码请用 `SyncHotPathFlag`（它会同时校验总开关与云盘）。
func SetHotPathEnabledForTest(enabled bool) func() {
	previous := captureHotPathEnabled.Load()
	captureHotPathEnabled.Store(enabled)
	return func() { captureHotPathEnabled.Store(previous) }
}

// maxCapturedTextBytes 单次请求最多捕获的文本字节数。
//
// **这是防止内存与云盘被单条巨型请求打爆的硬闸门**：
// 一个把 10MB 上下文塞进 prompt 的请求，若原样入队，
// 1024 条队列就是 10GB（容器只有 900MB），必然 OOM。
//
// 截断而非拒绝：截断仍保留开头的对话特征，对增量去重与
// 「有没有这类数据」的判断都够用；拒绝则等于大请求完全不被采集。
// 截断位置会被记录（见 `Truncated`），不假装是完整数据。
const maxCapturedTextBytes = 64 << 10 // 64KB

// CaptureInput 调用方（controller 层）能提供的**已有**数据。
//
// 全部是「不需要额外计算就能拿到」的字段，这是热路径零成本的前提。
type CaptureInput struct {
	UserId   int
	Username string
	Model    string

	// ConversationID 会话标识（用于精确增量）。
	//
	// 来源优先级：请求里的 `prompt_cache_key` > 空。
	// **没有会话 id 就退化为全局指纹去重**（见 worker 的退化路径），
	// 不影响正确性，只是去重粒度变粗。
	ConversationID string

	// Meta 调用方已算好的 token 计数元数据（含 CombineText）。
	// nil 时本次不采集（拿不到文本就没有采集的价值）。
	Meta *types.TokenCountMeta

	// ResponseText 助手回复文本。调用方能提供时才填
	// （流式场景下逐个 chunk 拼接的代价高，见下方 BuildSignal 的说明）。
	ResponseText string

	// CreatedAt 请求完成时间（unix 秒）。0 表示由本函数填 now。
	CreatedAt int64
}

// BuildSignal 把热路径输入转成采集信号。
//
// **纯计算，无 IO**：不查库、不联网、不加锁。
// 返回 nil 表示本次不该采集（未开启 / 输入不足 / 快速开关关）。
func BuildSignal(in CaptureInput) *CaptureSignal {
	if in.UserId <= 0 || in.Meta == nil {
		return nil
	}
	text := strings.TrimSpace(in.Meta.CombineText)
	if text == "" {
		// 拿不到任何文本 —— 常见于纯图片/纯 embedding/音频请求。
		// 这些请求采了也没有对话内容可训练，直接跳过更诚实
		// （而不是存一堆空记录进云盘）。
		return nil
	}
	if !captureHotPathEnabled.Load() {
		return nil
	}

	createdAt := in.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	messages := make([]Message, 0, 2)
	messages = append(messages, Message{
		Role:    "request",
		Content: truncateText(text, maxCapturedTextBytes),
	})
	if reply := strings.TrimSpace(in.ResponseText); reply != "" {
		messages = append(messages, Message{
			Role:    "assistant",
			Content: truncateText(reply, maxCapturedTextBytes),
		})
	}

	return &CaptureSignal{
		UserID:         in.UserId,
		Username:       in.Username,
		Model:          in.Model,
		ConversationID: in.ConversationID,
		Messages:       messages,
		ResponseText:   "", // 已并入 messages，避免 worker 侧重复追加
		CreatedAt:      createdAt,
	}
}

// truncateText 按**字节**上限截断，但不切断 UTF-8 字符。
//
// 直接 `text[:n]` 会切出半个汉字（非法 UTF-8），
// 存进 JSONL 后客户端解析会报错，且大模型训练数据里出现乱码。
func truncateText(text string, maxBytes int) string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text
	}
	// 从 maxBytes 往回退到合法 UTF-8 边界。
	cut := maxBytes
	for cut > 0 && (text[cut]&0xC0) == 0x80 {
		cut--
	}
	return text[:cut]
}
