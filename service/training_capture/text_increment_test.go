package training_capture

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 文本增量是**实测发现的设计缺口**的修复（见 text_increment.go）：
// 请求路径只能拿到一整块拼接文本，消息级水位恒为 1，
// 导致长期会话每次都全量重传 —— 正是用户点名要避免的。

// ---------- 核心行为：只传追加的那一段 ----------

func TestSelectTextIncrementFirstCaptureIsFull(t *testing.T) {
	text := "第一轮问题\n第一轮回答"
	suffix, wm := SelectTextIncrement(text, TextWatermark{})

	if suffix != text {
		t.Fatalf("首次应全量，实际 %q", suffix)
	}
	if wm.UploadedBytes != len(text) {
		t.Fatalf("水位应为 %d 字节，实际 %d", len(text), wm.UploadedBytes)
	}
	if wm.PrefixHash == "" {
		t.Fatal("必须记录前缀指纹")
	}
}

func TestSelectTextIncrementUploadsOnlyAppendedSuffix(t *testing.T) {
	// **这是用户点名的场景**：长期会话继续对话，历史一模一样，
	// 只应上传追加的那段，而不是整段会话。
	first := "system\n你是一个助手\nuser\n第一轮问题\nassistant\n第一轮回答"
	_, wm := SelectTextIncrement(first, TextWatermark{})

	grown := first + "\nuser\n第二轮问题\nassistant\n第二轮回答"
	suffix, wm2 := SelectTextIncrement(grown, wm)

	const wantSuffix = "\nuser\n第二轮问题\nassistant\n第二轮回答"
	if suffix != wantSuffix {
		t.Fatalf("应只传追加部分 %q，实际 %q", wantSuffix, suffix)
	}
	// 关键：增量里**不含**历史。
	if strings.Contains(suffix, "第一轮") {
		t.Fatalf("增量里含历史内容（说明在整段重传）：%q", suffix)
	}
	if wm2.UploadedBytes != len(grown) {
		t.Fatalf("水位应推进到 %d，实际 %d", len(grown), wm2.UploadedBytes)
	}
}

func TestSelectTextIncrementReturnsEmptyWhenUnchanged(t *testing.T) {
	text := "同样的对话"
	suffix, wm := SelectTextIncrement(text, TextWatermark{})
	if suffix != text {
		t.Fatal("首次应全量")
	}

	suffix2, wm2 := SelectTextIncrement(text, wm)
	if suffix2 != "" {
		t.Fatalf("无新增时不应产生内容，实际 %q", suffix2)
	}
	if wm2 != wm {
		t.Fatalf("无新增时水位不该变动：%+v → %+v", wm, wm2)
	}
}

// ---------- 安全边界：历史被改动绝不裁剪 ----------

func TestSelectTextIncrementReuploadsWhenPrefixEdited(t *testing.T) {
	// 历史中间被编辑 → 前缀哈希不匹配 → **保守全量**。
	// 裁剪会丢被改动的内容（采集不可逆 = 永久丢失）。
	first := "第一轮问题\n第一轮回答"
	_, wm := SelectTextIncrement(first, TextWatermark{})

	// 同长度但内容不同（这是最阴险的一种：只比长度的实现会漏掉）。
	// 用「替换 1 个字符」构造，保证字节长度严格相等。
	edited := "第一轮问题X第一轮回答" // 与 first 同长：把 "\n" 换成 "X"
	if len(edited) != len(first) {
		t.Fatalf("本用例要求长度相同以验证「只比长度不够」，%d vs %d", len(edited), len(first))
	}
	suffix, _ := SelectTextIncrement(edited, wm)

	if suffix != edited {
		t.Fatalf("前缀被改动时必须全量重传，实际只传了 %q", suffix)
	}
}

func TestSelectTextIncrementReuploadsWhenShorter(t *testing.T) {
	// 内容变短（用户删了消息）→ 保守全量。
	first := "第一轮问题\n第一轮回答\n第二轮问题"
	_, wm := SelectTextIncrement(first, TextWatermark{})

	shrunk := "第一轮问题"
	suffix, wm2 := SelectTextIncrement(shrunk, wm)

	if suffix != shrunk {
		t.Fatalf("文本变短时必须全量重传，实际 %q", suffix)
	}
	if wm2.UploadedBytes != len(shrunk) {
		t.Fatalf("水位应跟到新长度 %d，实际 %d", len(shrunk), wm2.UploadedBytes)
	}
}

func TestSelectTextIncrementHandlesReplacedContent(t *testing.T) {
	// 完全换了内容（前缀完全对不上）→ 全量。
	_, wm := SelectTextIncrement("旧会话内容", TextWatermark{})
	suffix, _ := SelectTextIncrement("全新的一轮问答", wm)

	if suffix != "全新的一轮问答" {
		t.Fatalf("内容被替换时必须全量，实际 %q", suffix)
	}
}

func TestSelectTextIncrementHandlesCorruptWatermark(t *testing.T) {
	// 水位超出文本长度（异常状态）→ 保守全量，绝不 panic。
	text := "短文本"
	suffix, wm := SelectTextIncrement(text, TextWatermark{
		UploadedBytes: 9999, PrefixHash: "whatever",
	})
	if suffix != text {
		t.Fatalf("异常水位应保守全量，实际 %q", suffix)
	}
	if wm.UploadedBytes != len(text) {
		t.Fatalf("水位应被修正到 %d，实际 %d", len(text), wm.UploadedBytes)
	}
}

func TestSelectTextIncrementHandlesEmptyInput(t *testing.T) {
	suffix, wm := SelectTextIncrement("", TextWatermark{UploadedBytes: 5, PrefixHash: "x"})
	if suffix != "" {
		t.Fatalf("空输入不应产生内容，实际 %q", suffix)
	}
	if wm.UploadedBytes != 5 {
		t.Fatalf("空输入不该改动已有水位，实际 %d", wm.UploadedBytes)
	}
}

func TestSelectTextIncrementSkipsHashCheckWhenWatermarkHasNoHash(t *testing.T) {
	// 兼容旧数据：水位只有长度没有哈希时，信任长度（不回退为全量，
	// 否则一次升级会让所有活跃会话都重传一遍）。
	text := "abcdef"
	suffix, _ := SelectTextIncrement(text, TextWatermark{UploadedBytes: 3})
	if suffix != "def" {
		t.Fatalf("无哈希时应按长度取增量，实际 %q", suffix)
	}
}

// ---------- 长期会话的累积语义（端到端式） ----------

func TestSelectTextIncrementAcrossManyTurns(t *testing.T) {
	// 模拟 20 轮对话逐轮增长：总上传量应≈最终文本长度，
	// 而不是每轮都传一遍（那会是 O(n²)）。
	var history string
	var wm TextWatermark
	totalUploaded := 0

	for i := range 20 {
		if history != "" {
			history += "\n"
		}
		history += "user\n问题\nassistant\n回答 " + string(rune('A'+i))

		suffix, next := SelectTextIncrement(history, wm)
		wm = next
		totalUploaded += len(suffix)
	}

	// 增量累加应恰好等于最终文本长度（不多传、不丢）。
	if totalUploaded != len(history) {
		t.Fatalf("累计上传 %d 字节，最终文本 %d 字节 —— 不等说明重传或丢失",
			totalUploaded, len(history))
	}
}

// ---------- 分片：不切断 UTF-8 ----------

func TestSplitTextSuffixKeepsChunksWithinLimit(t *testing.T) {
	text := strings.Repeat("这是很长的一行文本内容\n", 500)
	chunks := splitTextSuffix(text, 1000)

	if len(chunks) < 2 {
		t.Fatalf("应被切成多块，实际 %d 块", len(chunks))
	}
	for i, chunk := range chunks {
		if len(chunk) > 1000 {
			t.Fatalf("第 %d 块超过上限：%d 字节", i, len(chunk))
		}
		// 除最后一块外，都应在换行处结束（不在行中间切断）。
		if i < len(chunks)-1 && !strings.HasSuffix(chunk, "\n") {
			t.Fatalf("第 %d 块没有在行边界结束", i)
		}
	}
	if got := strings.Join(chunks, ""); got != text {
		t.Fatal("重组后与原文不一致（丢字节或重复）")
	}
}

func TestSplitTextSuffixHandlesOversizedSingleLine(t *testing.T) {
	// 单行超长（无换行可切）→ 必须在合法 UTF-8 边界硬切，且能前进。
	text := strings.Repeat("中", 2000) // 6000 字节，无换行
	chunks := splitTextSuffix(text, 1000)

	if len(chunks) == 0 {
		t.Fatal("必须产出分块")
	}
	for i, chunk := range chunks {
		if len(chunk) > 1000 {
			t.Fatalf("第 %d 块超过上限：%d", i, len(chunk))
		}
		if !isValidUTF8(chunk) {
			t.Fatalf("第 %d 块被切出了非法 UTF-8", i)
		}
	}
	if got := strings.Join(chunks, ""); got != text {
		t.Fatal("硬切后重组必须一致")
	}
}

func TestSplitTextSuffixBoundaries(t *testing.T) {
	if got := splitTextSuffix("", 100); got != nil {
		t.Fatalf("空输入应返回 nil，实际 %v", got)
	}
	small := "abc"
	if got := splitTextSuffix(small, 100); len(got) != 1 || got[0] != small {
		t.Fatalf("未超限不应切分，实际 %v", got)
	}
	if got := splitTextSuffix(small, 0); len(got) != 1 {
		t.Fatalf("maxBytes<=0 不应切分，实际 %d 块", len(got))
	}
}

// isValidUTF8 用标准库校验（不自己写判定逻辑）。
func isValidUTF8(s string) bool {
	return utf8.ValidString(s)
}
