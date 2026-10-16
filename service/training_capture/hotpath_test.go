package training_capture

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/lza6/new-api-Max/relaykit/types"
)

// 热路径的性能是**用户点名的验收标准**：
// 「采集数据的时候一定不能影响用户的请求等等延迟速度等等之类的」。
// 因此这里既要有正确性用例，也要有可复现的基准数字。

// ---------- 快速开关：未开启时必须零成本短路 ----------

func TestBuildSignalReturnsNilWhenHotPathDisabled(t *testing.T) {
	defer SetHotPathEnabledForTest(false)()

	signal := BuildSignal(CaptureInput{
		UserId: 1, Username: "a", Model: "m",
		Meta: &types.TokenCountMeta{CombineText: "有内容"},
	})
	if signal != nil {
		t.Fatal("热路径关闭时不得构造信号")
	}
}

func TestBuildSignalReturnsNilWithoutMetaOrUserId(t *testing.T) {
	defer SetHotPathEnabledForTest(true)()

	cases := []struct {
		name string
		in   CaptureInput
	}{
		{"无 meta", CaptureInput{UserId: 1}},
		{"用户 ID 为 0", CaptureInput{UserId: 0, Meta: &types.TokenCountMeta{CombineText: "x"}}},
		{"用户 ID 为负", CaptureInput{UserId: -1, Meta: &types.TokenCountMeta{CombineText: "x"}}},
		{"meta 无文本", CaptureInput{UserId: 1, Meta: &types.TokenCountMeta{}}},
		{"meta 只有空白", CaptureInput{UserId: 1, Meta: &types.TokenCountMeta{CombineText: "  \n\t "}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if signal := BuildSignal(tc.in); signal != nil {
				t.Fatalf("不该构造信号，实际 %+v", signal)
			}
		})
	}
}

func TestBuildSignalMapsFieldsToSignal(t *testing.T) {
	defer SetHotPathEnabledForTest(true)()

	signal := BuildSignal(CaptureInput{
		UserId: 42, Username: "alice", Model: "gpt-4o",
		ConversationID: "conv-9",
		Meta:           &types.TokenCountMeta{CombineText: "用户的提问"},
		ResponseText:   "助手的回答",
		CreatedAt:      1791937800,
	})
	if signal == nil {
		t.Fatal("应构造出信号")
	}
	if signal.UserID != 42 || signal.Username != "alice" || signal.Model != "gpt-4o" {
		t.Fatalf("身份字段映射错误：%+v", signal)
	}
	if signal.ConversationID != "conv-9" {
		t.Fatalf("会话 ID 未传递：%q", signal.ConversationID)
	}
	if signal.CreatedAt != 1791937800 {
		t.Fatalf("时间戳未被沿用（应由调用方决定归属日期）：%d", signal.CreatedAt)
	}
	// [约定变更] 助手回复**不并进 Messages**，而是单独放在 ResponseText。
	//
	// 为什么不并：并进去会让消息数变成 2 → `isSingleRequestBlob` 判假 →
	// 增量从「文本前缀」退化成「消息级」→ 每轮全量重传（去重静默失效）。
	// worker 在算出增量之后再把回复追加为一条独立记录。
	if len(signal.Messages) != 1 {
		t.Fatalf("Messages 应只有请求一条（回复走 ResponseText），实际 %d 条：%+v",
			len(signal.Messages), signal.Messages)
	}
	if signal.Messages[0].Role != "request" || signal.Messages[0].Content != "用户的提问" {
		t.Fatalf("请求文本映射错误：%+v", signal.Messages[0])
	}
	if signal.ResponseText != "助手的回答" {
		t.Fatalf("回复应放在 ResponseText，实际 %q", signal.ResponseText)
	}
}

func TestBuildSignalFillsCreatedAtWhenZero(t *testing.T) {
	defer SetHotPathEnabledForTest(true)()
	before := time.Now().Unix()

	signal := BuildSignal(CaptureInput{
		UserId: 1, Meta: &types.TokenCountMeta{CombineText: "x"},
	})
	if signal == nil {
		t.Fatal("应构造出信号")
	}
	if signal.CreatedAt < before {
		t.Fatalf("未提供时间时应填当前时间，实际 %d（早于 %d）", signal.CreatedAt, before)
	}
}

func TestBuildSignalOmitsEmptyResponse(t *testing.T) {
	defer SetHotPathEnabledForTest(true)()

	signal := BuildSignal(CaptureInput{
		UserId: 1, Meta: &types.TokenCountMeta{CombineText: "只有提问"}, ResponseText: "   ",
	})
	if signal == nil {
		t.Fatal("应构造出信号")
	}
	if len(signal.Messages) != 1 {
		t.Fatalf("空白回复不应产生消息，实际 %d 条", len(signal.Messages))
	}
}

// ---------- 大请求的硬闸门：不能把 10MB 上下文塞进队列 ----------

func TestBuildSignalTruncatesHugeRequestText(t *testing.T) {
	defer SetHotPathEnabledForTest(true)()

	huge := strings.Repeat("a", maxCapturedTextBytes*3)
	signal := BuildSignal(CaptureInput{
		UserId: 1, Meta: &types.TokenCountMeta{CombineText: huge},
	})
	if signal == nil {
		t.Fatal("应构造出信号")
	}
	got := signal.Messages[0].Content
	if len(got) > maxCapturedTextBytes {
		t.Fatalf("请求文本必须被截断到 %d 字节，实际 %d 字节", maxCapturedTextBytes, len(got))
	}
	if len(got) == 0 {
		t.Fatal("截断后不该变成空")
	}

	// 队列容量 × 单条上限必须留在容器预算内（900MB）。
	const containerBudgetBytes = 900 << 20
	worstCase := int64(CaptureQueueDefaultCapacity) * maxCapturedTextBytes
	if worstCase > containerBudgetBytes/4 {
		t.Fatalf("队列最坏占用 %d 字节，超过容器预算的 1/4 —— 高并发下会 OOM", worstCase)
	}
}

func TestTruncateTextDoesNotSplitUTF8Rune(t *testing.T) {
	// 中文 3 字节/字：在 10 字节处硬切会切出半个汉字 → 非法 UTF-8，
	// 存进 JSONL 后解析报错。
	text := strings.Repeat("中", 10) // 30 字节
	got := truncateText(text, 10)
	if len(got) > 10 {
		t.Fatalf("不得超出上限：%d", len(got))
	}
	// 用标准库校验：非法 UTF-8 会让 utf8.ValidString 返回 false。
	if !utf8.ValidString(got) {
		t.Fatalf("截断切出了非法 UTF-8：%q", got)
	}
	if len(got) != 9 {
		t.Fatalf("应退到 9 字节（3 个汉字），实际 %d 字节", len(got))
	}

	// 边界：正好切在字符边界上时不回退。
	if got := truncateText("abc", 3); got != "abc" {
		t.Fatalf("未超限不应截断：%q", got)
	}
	if got := truncateText("中文", 6); got != "中文" {
		t.Fatalf("正好在边界上不应回退：%q", got)
	}
	// maxBytes <= 0 视为不限制（避免调用方误传 0 导致内容全空）。
	if got := truncateText("中文", 0); got != "中文" {
		t.Fatalf("maxBytes=0 不应截断，实际 %q", got)
	}
	// 逐字节验证：模拟「先硬切再回退」的等价路径，
	// 确认任何切点都产出合法 UTF-8。
	for cut := 1; cut <= len(text); cut++ {
		if out := truncateText(text, cut); !utf8.ValidString(out) {
			t.Fatalf("在 %d 字节处截断产生了非法 UTF-8：%q", cut, out)
		}
	}
}

// ---------- 快速开关与总开关的同步（防「开关开了但采不到」） ----------

func TestSyncHotPathFlagTracksGatewayAndSink(t *testing.T) {
	cases := []struct {
		name      string
		sink      TrainingDataSink
		enabled   bool
		wantHot   bool
		rationale string
	}{
		{"云盘已配 + 开关开", fakeSink{name: "s"}, true, true, "正常采集"},
		{"云盘已配 + 开关关", fakeSink{name: "s"}, false, false, "管理员关闭"},
		{"云盘未配 + 开关开", nil, true, false, "没地方存，采了只能丢"},
		{"云盘未配 + 开关关", nil, false, false, "双重关闭"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer SetSinkForTest(tc.sink)()
			defer SetGatewayEnabledForTest(tc.enabled)()

			if got := HotPathEnabled(); got != tc.wantHot {
				t.Fatalf("%s：热路径应为 %v，实际 %v", tc.rationale, tc.wantHot, got)
			}
		})
	}
}

func TestSetSinkForTestSyncsHotPathFlag(t *testing.T) {
	// 这是「死代码自查」的直接用例：注入后端后热路径必须立即放行，
	// 否则 controller 里的 BuildSignal 永远返回 nil，功能整个不生效。
	defer SetGatewayEnabledForTest(true)()

	restore := SetSinkForTest(fakeSink{name: "s"})
	if !HotPathEnabled() {
		t.Fatal("注入后端后热路径未放行 —— 采集会静默不工作")
	}
	restore()
	if HotPathEnabled() {
		t.Fatal("恢复为无后端后热路径应关闭")
	}
}

// ---------- 基准：证明热路径开销可忽略 ----------

// 未启用时的开销（最常见情况：绝大多数部署要么不用、要么云盘没配）。
// 目标：与一次普通函数调用同量级，**没有分配**。
func BenchmarkBuildSignalDisabled(b *testing.B) {
	// 基准函数里不能用 defer 恢复标志：`b.N` 次循环结束后 defer 才执行，
	// 但基准之间共享进程状态，前一个基准的 defer 会在**下一个基准开始后**
	// 才恢复 —— 这曾让 BenchmarkBuildSignalEnabled 因标志被改回 false 而失败。
	// 改为显式置位，不做恢复（基准不改变被测语义，进程随即退出）。
	SetHotPathEnabled(false)
	meta := &types.TokenCountMeta{CombineText: strings.Repeat("x", 4096)}
	in := CaptureInput{UserId: 1, Username: "a", Model: "m", Meta: meta}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if BuildSignal(in) != nil {
			b.Fatal("关闭时不该产生信号")
		}
	}
}

// 启用时的开销（真实采集请求）。
// 这里刻意用 4KB 文本 —— 真实对话的量级。
func BenchmarkBuildSignalEnabled(b *testing.B) {
	SetHotPathEnabled(true)
	meta := &types.TokenCountMeta{CombineText: strings.Repeat("x", 4096)}
	in := CaptureInput{UserId: 1, Username: "a", Model: "m", Meta: meta, ConversationID: "c"}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if BuildSignal(in) == nil {
			b.Fatal("启用时应产生信号")
		}
	}
}

// 端到端热路径：构造信号 + 非阻塞入队（不含 worker 消费）。
// 这是对「采集是否拖慢请求」最直接的量化。
func BenchmarkHotPathFullCapture(b *testing.B) {
	SetHotPathEnabled(true)
	restoreSink := SetSinkForTest(fakeSink{name: "s"})
	restoreGateway := SetGatewayEnabledForTest(true)

	// 用一个容量足够的独立队列，避免基准里计入「队列满」分支。
	originalQueue := captureQueue
	captureQueue = NewCaptureQueue(1 << 20)
	b.Cleanup(func() {
		captureQueue = originalQueue
		restoreGateway()
		restoreSink()
	})

	meta := &types.TokenCountMeta{CombineText: strings.Repeat("x", 4096)}
	in := CaptureInput{UserId: 1, Username: "a", Model: "m", Meta: meta}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		signal := BuildSignal(in)
		if signal != nil {
			captureQueue.TryEnqueue(*signal)
		}
	}
}

// 回归（独立审查 P1）：截断必须**复制**而不是返回子串。
//
// Go 的 `s[:n]` 共享底层数组。若 truncateText 返回子串，
// 一条 10MB 的 CombineText 即使「截断到 64KB」，其底层数组仍被
// 队列里的信号引用 —— 「内存硬闸门」实际不成立（1024 条队列
// 最坏钉住 10GB）。用 unsafe 指针比对验证截断产物**不再共享**原文。
func TestTruncateTextClonesUnderlyingArray(t *testing.T) {
	original := strings.Repeat("a", 1<<20) // 1MB
	truncated := truncateText(original, 64<<10)

	// 契约是「**不超过**上限」，不是「恰好等于」——
	// 现在保留头+尾并插入省略标记，长度会略小于上限。
	if len(truncated) > 64<<10 {
		t.Fatalf("截断必须不超过上限 64KB，实际 %d", len(truncated))
	}
	if len(truncated) == 0 {
		t.Fatal("截断后不该变成空")
	}
	if unsafeStringSharesBacking(original, truncated) {
		t.Fatal("截断产物与原文共享底层数组 —— 闸门失效，仍钉住 1MB 原文")
	}

	// 未截断路径允许共享（复制是浪费）：语义确认。
	if truncateText(original, 0) == "" {
		t.Fatal("maxBytes<=0 不应清空")
	}
}

// 超限截断必须保留**尾部**（新内容所在处），否则尾部永远采集不到。
func TestTruncateKeepsTailSoNewContentIsCapturable(t *testing.T) {
	head := strings.Repeat("H", 1000)
	tail := "TAIL-MARKER-NEW-CONTENT"
	text := head + strings.Repeat("M", 5000) + tail

	stored, tailOnly := truncateForCapture(text, 1024)

	if !strings.Contains(stored, tail) {
		t.Fatalf("截断后必须保留尾部内容（否则新内容永远采不到）：%q", stored[len(stored)-80:])
	}
	if !strings.Contains(tailOnly, tail) {
		t.Fatalf("TailOnly 必须含尾部内容，实际 %q", tailOnly)
	}
	if len(stored) > 1024 {
		t.Fatalf("存储内容不得超出上限：%d > 1024", len(stored))
	}
	// 未超限时不应产生 TailOnly（正常路径不该被这个机制影响）。
	if s, tl := truncateForCapture("short", 1024); tl != "" || s != "short" {
		t.Fatalf("未超限时不该截断或有 TailOnly：%q %q", s, tl)
	}
}

// unsafeStringSharesBacking 判断两个 string 是否共享底层数组。
// 仅测试用；生产代码不得出现 unsafe。
func unsafeStringSharesBacking(a, b string) bool {
	if len(a) == 0 || len(b) == 0 || len(b) > len(a) {
		return false
	}
	pa := unsafe.Pointer(unsafe.StringData(a))
	pb := unsafe.Pointer(unsafe.StringData(b))
	if pa == nil || pb == nil {
		return false
	}
	return uintptr(pa) <= uintptr(pb) && uintptr(pb) < uintptr(pa)+uintptr(len(a))
}
