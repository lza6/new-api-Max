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
// maxCapturedTextBytes 回退值：预算系统未初始化时使用（见 resources.go）。
const maxCapturedTextBytes = 64 << 10 // 64KB

// currentMaxTextBytes 返回生效的单条文本上限（预算初始化后取预算值）。
func currentMaxTextBytes() int {
	if b := GetResourceBudget(); b.MaxTextBytes > 0 {
		return b.MaxTextBytes
	}
	return maxCapturedTextBytes
}

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
		//
		// **但要计数**：`CountToken` 关闭时 `CombineText` 恒为空，
		// 此时采集一条不出而管理端只看到「今天 0 条」，无法区分
		// 「没流量」与「配置导致的静默失效」。这个计数器就是那个信号。
		captureStats.TotalSkippedNoText.Add(1)
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
	stored, tail := truncateForCapture(text, currentMaxTextBytes())
	messages = append(messages, Message{
		Role:    "request",
		Content: stored,
	})

	// [修复防御] 助手回复**不并进 Messages**，而是单独放在 ResponseText。
	//
	// 并进去会让 len(Messages)==2 → `isSingleRequestBlob` 判假 →
	// 增量逻辑从「文本前缀」退化到「消息级」，而消息级对「每轮内容都在变的
	// 拼接文本」会前缀不匹配 → **每轮全量重传、去重静默失效**。
	// 这正是盲点扫描预警的潜伏缺陷（B6）：一旦有人接线 ResponseText 就会触发。
	//
	// 保持 Messages 只有请求一条 → 增量逻辑不变；
	// reply 由 worker 在算出增量后**追加**为一条独立记录。
	reply := strings.TrimSpace(in.ResponseText)

	return &CaptureSignal{
		UserID:         in.UserId,
		Username:       in.Username,
		Model:          in.Model,
		ConversationID: in.ConversationID,
		Messages:       messages,
		ResponseText:   truncateText(reply, currentMaxTextBytes()),
		// TailOnly 非空表示文本超限被截断：worker 用**尾部**做增量
		// （新内容在尾部；对整段做前缀比对会因省略标记长度变化而每次都全量重传）。
		TailOnly:  tail,
		CreatedAt: createdAt,
	}
}

// truncateForCapture 截断并**同时返回尾部片段**（供增量使用）。
//
// 返回 (存储用的完整内容, 尾部增量片段)。未截断时第二个返回值为空。
//
// **总长度严格 ≤ maxBytes**（含省略标记）：上限是内存闸门，
// 不能因为加了标记就超出 —— 否则队列最坏占用会按标记长度成倍放大。
func truncateForCapture(text string, maxBytes int) (string, string) {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text, ""
	}
	// 先按「上限」构造一次固定长度的标记，用它算出可用于正文的预算。
	// 标记长度只取决于 elided 的位数，用 0 先探一次即可（下界），
	// 实际 elided 最多 20 位，用 24 字节余量足够且不让总长超出。
	const markerReserve = 64
	bodyBudget := maxBytes - markerReserve
	if bodyBudget < 2 {
		// 上限小到放不下标记 → 退化为纯头部截断（无标记）。
		return strings.Clone(safeCutPrefix(text, maxBytes)), ""
	}

	half := bodyBudget / 2
	head := safeCutPrefix(text, half)
	tailStart := len(text) - half
	if tailStart <= len(head) {
		// 头尾重叠（文本只比上限略大）→ 不值得省略中间，直接取头部。
		return strings.Clone(safeCutPrefix(text, maxBytes)), ""
	}
	tail := text[skipToRuneStart(text, tailStart):]
	elided := len(text) - len(head) - len(tail)
	marker := elisionMarker(elided)
	// 标记实际长度可能与预留不同（elided 位数变化）——按实际裁剪尾部，
	// 保证 head+marker+tail 严格 ≤ maxBytes。
	if over := len(head) + len(marker) + len(tail) - maxBytes; over > 0 {
		if over >= len(tail) {
			tail = ""
		} else {
			tail = tail[over:]
			// 裁剪后可能落在多字节字符中间，向后对齐。
			tail = tail[skipToRuneStart(tail, 0):]
		}
	}
	stored := strings.Clone(head) + marker + strings.Clone(tail)
	// 尾部也要复制（切断对大原文的引用）；作为独立增量单独返回。
	return stored, strings.Clone(tail)
}

// truncateText 按**字节**上限截断（保留头+尾，见 truncateForCapture）。
//
// 只返回存储用的字符串；需要尾部增量时用 truncateForCapture。
// 合并成一个实现是刻意的：曾经这里与 truncateForCapture 是两份逻辑不同的
// UTF-8 边界截断（DRY 违规），两边行为不一致会让「显示的内容」与
// 「用于增量的内容」对不上，属于最难排查的一类不一致。
func truncateText(text string, maxBytes int) string {
	stored, _ := truncateForCapture(text, maxBytes)
	return stored
}

// safeCutPrefixIndex 返回 <= limit 且位于 UTF-8 边界的位置。
func safeCutPrefixIndex(text string, limit int) int {
	if limit >= len(text) {
		return len(text)
	}
	cut := limit
	for cut > 0 && (text[cut]&0xC0) == 0x80 {
		cut--
	}
	return cut
}

// safeCutPrefix 同 safeCutPrefixIndex，但直接返回子串。
func safeCutPrefix(text string, limit int) string {
	return text[:safeCutPrefixIndex(text, limit)]
}

// skipToRuneStart 从 pos 向后找到第一个合法 UTF-8 起始字节。
func skipToRuneStart(text string, pos int) int {
	i := pos
	for i < len(text) && (text[i]&0xC0) == 0x80 {
		i++
	}
	return i
}

// elisionMarker 中间省略标记。
//
// **固定前缀 + 省略字节数**。用一个显眼的、几乎不可能出现在真实对话里的
// 形式，便于下游识别「这不是用户原文」。
func elisionMarker(elidedBytes int) string {
	return "\n…[training-capture: " + itoaSmall(elidedBytes) + " bytes elided]…\n"
}

// itoaSmall 小整数转十进制字符串。
//
// 不用 strconv 是为了让 hotpath.go 只依赖 strings —— 该文件在请求路径上，
// 保持依赖面最小便于审查（且这里的数字最多 20 位，手写循环足够）。
func itoaSmall(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
