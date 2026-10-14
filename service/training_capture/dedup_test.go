package training_capture

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 增量去重是用户**点名**的算法需求：
// 「长期会话的代码等等那些的话就会发现很多 token 都一样的，这样我们就能智能识别增量添加」。
// 因此「历史不变时只传增量」必须有直接、可读的用例。

func msg(role, content string) Message {
	return Message{Role: role, Content: content}
}

// ---------- Fingerprint ----------

func TestFingerprintIsStableAndRoleSensitive(t *testing.T) {
	a := Fingerprint(msg("user", "hello"))
	assert.Equal(t, a, Fingerprint(msg("user", "hello")), "同内容同角色必须同指纹")
	assert.NotEqual(t, a, Fingerprint(msg("assistant", "hello")),
		"同样文本在 user/assistant 两侧语义不同，指纹必须区分")
	assert.NotEqual(t, a, Fingerprint(msg("user", "hello!")))
}

func TestFingerprintNormalizesWhitespace(t *testing.T) {
	// 客户端因换行/缩进差异重发同一段内容时不应被当成新内容。
	// 这在**代码类对话**里极常见——正是用户点名的场景。
	base := Fingerprint(msg("user", "def f():\n    return 1"))
	assert.Equal(t, base, Fingerprint(msg("user", "def f():\r\n    return 1")), "CRLF vs LF")
	assert.Equal(t, base, Fingerprint(msg("user", "def f():   return 1")), "多余空格")
	assert.Equal(t, base, Fingerprint(msg("user", "  def f():\n    return 1  ")), "首尾空白")
	assert.Equal(t, base, Fingerprint(msg("user", "def f():\treturn 1")), "tab vs 空格")
}

func TestFingerprintDoesNotAlterNonWhitespaceContent(t *testing.T) {
	// 采集侧不得做语义改写，否则存进云盘的内容与真实对话不一致。
	assert.NotEqual(t,
		Fingerprint(msg("user", "a b")),
		Fingerprint(msg("user", "ab")),
		"空白折叠不应把有意义的空格差异吃掉")
}

func TestFingerprintSeparatesRoleAndContentBoundary(t *testing.T) {
	// 用 \x00 分隔避免「role=a,content=bc」与「role=ab,content=c」撞指纹。
	assert.NotEqual(t,
		Fingerprint(msg("a", "bc")),
		Fingerprint(msg("ab", "c")))
}

func TestFingerprintHandlesEmptyAndUnicode(t *testing.T) {
	assert.NotEmpty(t, Fingerprint(msg("user", "")))
	assert.NotEmpty(t, Fingerprint(msg("", "")))
	// 全角空格也是空白，应被折叠。
	assert.Equal(t,
		Fingerprint(msg("user", "你好 世界")),
		Fingerprint(msg("user", "你好　　世界")))
}

// ---------- SelectIncrement：核心场景 ----------

func TestSelectIncrementFirstCaptureIsFull(t *testing.T) {
	messages := []Message{
		msg("user", "q1"), msg("assistant", "a1"),
	}
	inc, wm := SelectIncrement(messages, Watermark{})
	require.Len(t, inc, 2, "首次采集应上传全部")
	assert.Equal(t, 2, wm.UploadedCount)
	// 前缀指纹覆盖**整个已上传前缀**（不是只有末条）——见 B3 回归。
	assert.Equal(t, PrefixFingerprint(messages, 2), wm.PrefixFingerprint)
}

func TestSelectIncrementLongConversationUploadsOnlyNewTurns(t *testing.T) {
	// **用户点名的场景**：长期会话继续对话时，历史轮次内容一模一样，
	// 只应上传新增的那几条，而不是整段会话。
	history := make([]Message, 0, 40)
	for i := range 20 {
		history = append(history,
			msg("user", fmt.Sprintf("问题 %d：请解释这段代码", i)),
			msg("assistant", "这是解释……（很长的回复，token 很多）"),
		)
	}

	// 第一次采集：全量 40 条。
	_, wm := SelectIncrement(history, Watermark{})
	require.Equal(t, 40, wm.UploadedCount)

	// 用户继续两轮 → 会话变成 44 条，但前 40 条**原封不动**。
	grown := append(append([]Message{}, history...),
		msg("user", "再解释一下"), msg("assistant", "好的……"),
		msg("user", "还有呢"), msg("assistant", "还有……"),
	)

	inc, wm2 := SelectIncrement(grown, wm)

	require.Len(t, inc, 4, "只应上传新增的 4 条，而不是 44 条")
	assert.Equal(t, "再解释一下", inc[0].Content)
	assert.Equal(t, 44, wm2.UploadedCount)
}

func TestSelectIncrementReturnsNothingWhenUnchanged(t *testing.T) {
	messages := []Message{msg("user", "q"), msg("assistant", "a")}
	_, wm := SelectIncrement(messages, Watermark{})

	inc, wm2 := SelectIncrement(messages, wm)

	assert.Empty(t, inc, "无新增时不应产生上传")
	assert.Equal(t, wm, wm2, "水位不应被无意义地重建")
}

func TestSelectIncrementReuploadsConservativelyWhenHistoryEdited(t *testing.T) {
	// 历史被编辑/插入/删除时，水位指纹校验会失败 → 保守全量重传。
	// 宁可多传一次，也绝不丢新内容。
	original := []Message{msg("user", "q1"), msg("assistant", "a1")}
	_, wm := SelectIncrement(original, Watermark{})

	// 第 2 条被编辑（assistant 回复被重新生成）。
	edited := []Message{msg("user", "q1"), msg("assistant", "a1-重新生成")}
	inc, wm2 := SelectIncrement(edited, wm)

	assert.Len(t, inc, 2, "历史被改写时必须保守重传，不能只传增量")
	assert.Equal(t, 2, wm2.UploadedCount)
}

func TestSelectIncrementReuploadsWhenMessagesDeleted(t *testing.T) {
	// 用户删了消息 → 消息数少于水位 → 保守全量。
	msgs := []Message{msg("user", "q1"), msg("assistant", "a1"), msg("user", "q2")}
	_, wm := SelectIncrement(msgs, Watermark{})
	require.Equal(t, 3, wm.UploadedCount)

	shrunk := msgs[:2]
	inc, wm2 := SelectIncrement(shrunk, wm)

	assert.Len(t, inc, 2, "消息数少于水位时应保守全量")
	assert.Equal(t, 2, wm2.UploadedCount)
}

func TestSelectIncrementHandlesWatermarkBeyondMessages(t *testing.T) {
	// 水位超出当前消息数（异常状态）→ 保守全量，绝不 panic。
	inc, wm := SelectIncrement([]Message{msg("u", "x")}, Watermark{UploadedCount: 99})
	assert.Len(t, inc, 1)
	assert.Equal(t, 1, wm.UploadedCount)
}

func TestSelectIncrementHandlesNegativeWatermark(t *testing.T) {
	inc, _ := SelectIncrement([]Message{msg("u", "x")}, Watermark{UploadedCount: -3})
	assert.Len(t, inc, 1, "负水位应被当作异常并全量")
}

func TestSelectIncrementHandlesEmptyMessages(t *testing.T) {
	inc, wm := SelectIncrement(nil, Watermark{UploadedCount: 5})
	assert.Empty(t, inc)
	assert.Equal(t, 5, wm.UploadedCount, "空输入不应改动已有水位")

	inc2, wm2 := SelectIncrement([]Message{}, Watermark{})
	assert.Empty(t, inc2)
	assert.Equal(t, 0, wm2.UploadedCount)
}

func TestSelectIncrementKeepsRepeatedContentAsNewTurns(t *testing.T) {
	// 去重是「不重传历史」，**不是**「去重内容」：
	// 用户重复问同一件事是真实对话行为，对训练有价值，必须保留。
	msgs := []Message{msg("user", "同样的问题"), msg("assistant", "答")}
	_, wm := SelectIncrement(msgs, Watermark{})

	again := append(append([]Message{}, msgs...), msg("user", "同样的问题"))
	inc, _ := SelectIncrement(again, wm)

	require.Len(t, inc, 1, "重复内容作为**新轮次**仍应上传")
	assert.Equal(t, "同样的问题", inc[0].Content)
}

func TestSelectIncrementSkipsFingerprintCheckWhenWatermarkHasNoFingerprint(t *testing.T) {
	// 兼容旧数据：水位只有计数没有指纹时，信任计数（不做校验）。
	msgs := []Message{msg("u", "a"), msg("u", "b"), msg("u", "c")}
	inc, wm := SelectIncrement(msgs, Watermark{UploadedCount: 2})

	require.Len(t, inc, 1)
	assert.Equal(t, "c", inc[0].Content)
	assert.NotEmpty(t, wm.PrefixFingerprint, "新水位必须带上指纹")
}

// ---------- GlobalDedup（无会话 id 的退化路径） ----------

func TestGlobalDedupSkipsAlreadySeenContent(t *testing.T) {
	seen := NewBoundedFingerprintSet(100)
	first := GlobalDedup([]Message{msg("u", "a"), msg("u", "b")}, seen)
	assert.Len(t, first, 2)

	// 同样的内容再来一次 → 全部跳过。
	second := GlobalDedup([]Message{msg("u", "a"), msg("u", "b")}, seen)
	assert.Empty(t, second)

	// 新内容 → 放行。
	third := GlobalDedup([]Message{msg("u", "a"), msg("u", "c")}, seen)
	require.Len(t, third, 1)
	assert.Equal(t, "c", third[0].Content)
}

func TestGlobalDedupHandlesNilAndEmpty(t *testing.T) {
	assert.Empty(t, GlobalDedup(nil, NewBoundedFingerprintSet(10)))
	assert.Empty(t, GlobalDedup([]Message{msg("u", "a")}, nil))
}

// ---------- BoundedFingerprintSet ----------

func TestFingerprintSetEvictsOldestBeyondCapacity(t *testing.T) {
	// 无界会让内存随对话量无限增长（每天 10 万条 → 一年 2.3GB，撑爆 900MB 容器）。
	s := NewBoundedFingerprintSet(3)
	s.Add("a")
	s.Add("b")
	s.Add("c")
	assert.Equal(t, 3, s.Len())

	s.Add("d")

	assert.Equal(t, 3, s.Len(), "容量必须被硬守住")
	assert.False(t, s.Has("a"), "最久未使用的应被淘汰")
	assert.True(t, s.Has("b"))
	assert.True(t, s.Has("d"))
}

func TestFingerprintSetHasPromotesRecency(t *testing.T) {
	s := NewBoundedFingerprintSet(3)
	s.Add("a")
	s.Add("b")
	s.Add("c")

	// 访问 "a" → 它变成最近使用，下一个被淘汰的应是 "b"。
	require.True(t, s.Has("a"))
	s.Add("d")

	assert.True(t, s.Has("a"), "被访问过的应免于淘汰")
	assert.False(t, s.Has("b"), "未被访问的最旧项应被淘汰")
}

func TestFingerprintSetCapacityIsBoundedRegardlessOfInput(t *testing.T) {
	// 极端输入下也不得突破容量。
	s := NewBoundedFingerprintSet(50)
	for i := range 5000 {
		s.Add(fmt.Sprintf("fp-%d", i))
	}
	assert.Equal(t, 50, s.Len())
	assert.Equal(t, 50, s.Capacity())
}

func TestFingerprintSetDefaultCapacityWhenNonPositive(t *testing.T) {
	assert.Equal(t, FingerprintSetDefaultCapacity, NewBoundedFingerprintSet(0).Capacity())
	assert.Equal(t, FingerprintSetDefaultCapacity, NewBoundedFingerprintSet(-1).Capacity())
}

func TestFingerprintSetIgnoresEmptyFingerprint(t *testing.T) {
	s := NewBoundedFingerprintSet(5)
	s.Add("")
	assert.Zero(t, s.Len(), "空指纹不应占用容量")
	assert.False(t, s.Has(""))
}

func TestFingerprintSetNilReceiverIsSafe(t *testing.T) {
	var s *BoundedFingerprintSet
	assert.False(t, s.Has("x"))
	assert.NotPanics(t, func() { s.Add("x") })
	assert.Zero(t, s.Len())
}

func TestFingerprintSetHandlesConcurrentUse(t *testing.T) {
	// 单 worker 运行时不需要锁，但接口预置互斥是为了将来横向扩展；
	// 这里用 -race 验证锁真的生效。
	s := NewBoundedFingerprintSet(100)
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 200 {
				fp := fmt.Sprintf("w%d-%d", worker, i)
				s.Add(fp)
				_ = s.Has(fp)
			}
		}(w)
	}
	wg.Wait()
	assert.Equal(t, 100, s.Len(), "并发下容量仍必须被守住")
}

// ---------- 组合：真实对话形态的端到端语义 ----------

func TestIncrementOverRealisticConversationLifecycle(t *testing.T) {
	// 模拟真实生命周期：首采 → 多轮追加 → 中途编辑 → 删除。
	conv := []Message{
		msg("system", "You are a helpful assistant."),
		msg("user", "帮我写一个 Go 的 HTTP server"),
		msg("assistant", "```go\nhttp.ListenAndServe(...)\n```"),
	}

	inc1, wm := SelectIncrement(conv, Watermark{})
	require.Len(t, inc1, 3, "首次全量")

	// 追加两轮（历史不变）→ 只传新增。
	conv = append(conv,
		msg("user", "加个超时"),
		msg("assistant", "```go\nsrv.ReadTimeout = 5*time.Second\n```"),
	)
	inc2, wm2 := SelectIncrement(conv, wm)
	require.Len(t, inc2, 2, "只传新增两条")
	assert.Equal(t, "加个超时", inc2[0].Content)

	// 再追加 → 仍只传新增。
	conv = append(conv, msg("user", "谢谢"))
	inc3, wm3 := SelectIncrement(conv, wm2)
	require.Len(t, inc3, 1)
	assert.Equal(t, "谢谢", inc3[0].Content)

	// 编辑历史中最后一条 → 保守全量（不丢内容）。
	conv[len(conv)-1] = msg("user", "谢谢，再帮我看看日志")
	inc4, _ := SelectIncrement(conv, wm3)
	assert.Len(t, inc4, len(conv), "历史被改写后保守全量")

	// 内容完全没变 → 不上传。
	inc5, _ := SelectIncrement(conv, watermarkAfter(conv))
	assert.Empty(t, inc5)
}

func TestIncrementIsNotDefeatedByWhitespaceOnlyChanges(t *testing.T) {
	// 客户端重发时格式化差异不应触发全量重传。
	conv := []Message{
		msg("user", "写一个函数"),
		msg("assistant", "func f() {\n    return 1\n}"),
	}
	_, wm := SelectIncrement(conv, Watermark{})

	reformatted := []Message{
		msg("user", "写一个函数"),
		// 同一段代码，缩进/换行风格不同。
		msg("assistant", "func f() {\r\n\treturn 1\r\n}"),
	}
	reformatted = append(reformatted, msg("user", "新问题"))
	inc, _ := SelectIncrement(reformatted, wm)

	require.Len(t, inc, 1, "仅排版差异不应被当成历史改写")
	assert.Equal(t, "新问题", inc[0].Content)
}

func TestNormalizeContentDoesNotGrowUnboundedMemory(t *testing.T) {
	// 归一化只做空白折叠；巨大的单条消息本身仍是问题，
	// 但那由上游的请求体上限保证（本函数不引入额外放大）。
	huge := strings.Repeat("a", 100_000)
	out := normalizeContent(huge)
	assert.Equal(t, 100_000, len(out), "非空白内容不得被改动长度")
}

// ---------- B3 回归：中间历史的编辑不得被静默丢弃 ----------

// 回归（B3）：历史中**中间**某条消息被编辑 → 必须被上传。
//
// 原实现只把水位的**末条**指纹存起来校验，于是：
//
//	历史已传: [A, B, C]        水位 count=3, 指纹=C
//	本次请求: [A, B_EDITED, C, D]
//	校验：messages[2] == C → 通过 → 只传 [D]
//	结果：B_EDITED **永久不上传**
//
// 采集不可逆（上传过就不再回看），所以这就是静默的数据丢失。
// 改为**前缀指纹**后，历史中任意位置的改动都会让校验失败 → 保守全量重传。
func TestSelectIncrementReuploadsWhenMiddleMessageEdited(t *testing.T) {
	historical := []Message{
		msg("user", "A"), msg("assistant", "B"), msg("user", "C"),
	}
	_, wm := SelectIncrement(historical, Watermark{})
	require.Equal(t, 3, wm.UploadedCount)

	edited := []Message{
		msg("user", "A"), msg("assistant", "B_EDITED"), msg("user", "C"), msg("user", "D"),
	}
	inc, _ := SelectIncrement(edited, wm)

	var gotEdited bool
	var contents []string
	for _, m := range inc {
		contents = append(contents, m.Content)
		if m.Content == "B_EDITED" {
			gotEdited = true
		}
	}
	assert.True(t, gotEdited,
		"中间被编辑的内容必须被上传，绝不能静默丢弃；实际上传 %v", contents)
	assert.Len(t, inc, 4, "保守全量重传")
}

// 回归（B3 补充）：中间消息被**删除**时同样不得静默通过。
func TestSelectIncrementReuploadsWhenMiddleMessageDeleted(t *testing.T) {
	historical := []Message{
		msg("user", "A"), msg("assistant", "B"), msg("user", "C"), msg("user", "D"),
	}
	_, wm := SelectIncrement(historical, Watermark{})
	require.Equal(t, 4, wm.UploadedCount)

	// 删掉中间那条 C。
	shrunk := []Message{
		msg("user", "A"), msg("assistant", "B"), msg("user", "D"), msg("user", "E"),
	}
	inc, _ := SelectIncrement(shrunk, wm)

	assert.Len(t, inc, len(shrunk), "中间删除也必须被察觉并保守全量重传")
}

// 前缀指纹必须对**任意位置**的改动敏感（这是 B3 修复的核心性质）。
func TestPrefixFingerprintDetectsChangeAtAnyPosition(t *testing.T) {
	base := []Message{
		msg("user", "1"), msg("assistant", "2"), msg("user", "3"),
		msg("assistant", "4"), msg("user", "5"),
	}
	original := PrefixFingerprint(base, len(base))

	for i := range base {
		mutated := append([]Message{}, base...)
		mutated[i] = msg(mutated[i].Role, mutated[i].Content+"-EDIT")
		assert.NotEqual(t, original, PrefixFingerprint(mutated, len(mutated)),
			"位置 %d 的改动必须改变前缀指纹", i)
	}
}

// 前缀指纹对**顺序**变化也敏感（消息重排是真实的编辑行为）。
func TestPrefixFingerprintDetectsReordering(t *testing.T) {
	a := []Message{msg("user", "1"), msg("assistant", "2")}
	b := []Message{msg("assistant", "2"), msg("user", "1")}

	assert.NotEqual(t, PrefixFingerprint(a, 2), PrefixFingerprint(b, 2),
		"消息顺序变化必须改变前缀指纹")
}

// 前缀指纹的边界：n<=0 或空输入返回空串（表示「没有已上传前缀」）。
func TestPrefixFingerprintBoundaries(t *testing.T) {
	msgs := []Message{msg("user", "a")}
	assert.Empty(t, PrefixFingerprint(msgs, 0))
	assert.Empty(t, PrefixFingerprint(msgs, -1))
	assert.Empty(t, PrefixFingerprint(nil, 3))
	// n 超出长度时夹到长度（不 panic）。
	assert.Equal(t, PrefixFingerprint(msgs, 1), PrefixFingerprint(msgs, 99))
}

// 前缀指纹对**前缀**与**整体**的区分：只改后缀不影响前缀指纹
// （这正是「增量上传」成立的前提——否则每次新增都要全量）。
func TestPrefixFingerprintOfPrefixIsStableWhenOnlyTailAppended(t *testing.T) {
	base := []Message{msg("user", "1"), msg("assistant", "2")}
	withTail := append(append([]Message{}, base...), msg("user", "3"))

	assert.Equal(t, PrefixFingerprint(base, 2), PrefixFingerprint(withTail, 2),
		"仅追加尾部时，前 2 条的前缀指纹必须不变（增量上传的前提）")
	assert.NotEqual(t, PrefixFingerprint(base, 2), PrefixFingerprint(withTail, 3))
}
