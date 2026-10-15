package training_capture

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 目录名净化是**安全边界**：算错就是路径穿越或用户间数据互相覆盖。
// 因此边界要穷举，不留「大概没问题」。

func TestSanitizeDirNamePreventsPathTraversal(t *testing.T) {
	// 这是本函数存在的首要理由：用户名是用户可控输入，
	// 直接拼进云盘路径能逃出目录树并覆盖他人数据。
	attacks := []string{
		"../../etc/passwd",
		"..",
		"../..",
		`..\..\windows`,
		"a/../../b",
		"./../",
	}

	for _, attack := range attacks {
		got := SanitizeDirName(42, attack)
		assert.NotContains(t, got, "/", "输入 %q 不得留下路径分隔符", attack)
		assert.NotContains(t, got, `\`, "输入 %q 不得留下反斜杠", attack)
		assert.NotEqual(t, "..", got)
		assert.NotEqual(t, ".", got)
		assert.False(t, strings.HasPrefix(got, "."), "不得产生隐藏目录: %q", got)
		assert.False(t, isEmptyDirSegment(got), "结果必须是安全段: %q", got)
	}
}

func TestSanitizeDirNameAlwaysAppendsUserIDSuffix(t *testing.T) {
	// 无条件追加 id：让「不同用户必然不同目录」成为结构性保证，
	// 而不是依赖「是否恰好重名」的判断（纯函数无从得知全局用户名）。
	got := SanitizeDirName(7, "alice")
	assert.Equal(t, "alice-7", got)
}

func TestSanitizeDirNameDisambiguatesCollidingNames(t *testing.T) {
	// 两个不同用户名净化后若同名，必须因 id 后缀而分开——
	// 否则训练数据集的用户隔离失效，数据混在一起。
	a := SanitizeDirName(1, "a/b")
	b := SanitizeDirName(2, "a_b")
	assert.NotEqual(t, a, b, "不同用户绝不允许落到同一目录")
	assert.True(t, strings.HasSuffix(a, "-1"))
	assert.True(t, strings.HasSuffix(b, "-2"))
}

func TestSanitizeDirNameHandlesNonASCII(t *testing.T) {
	// 中文用户名很常见（生产实测有「凌清秋」）。
	got := SanitizeDirName(3, "凌清秋")
	assert.True(t, strings.HasSuffix(got, "-3"))
	assert.NotEmpty(t, strings.TrimSuffix(got, "-3"), "非 ASCII 不应被整体丢弃")

	// 全 emoji 也不能变成空目录名。
	emoji := SanitizeDirName(4, "🎉🎉")
	assert.True(t, strings.HasSuffix(emoji, "-4"))
	assert.NotEqual(t, "-4", emoji, "尾部必须有非空基名")
}

func TestSanitizeDirNameFallsBackWhenEmpty(t *testing.T) {
	for _, name := range []string{"", "   ", "...", "___", "///", `\\\`} {
		got := SanitizeDirName(99, name)
		assert.NotEmpty(t, got, "输入 %q 不得产出空目录名", name)
		assert.NotEqual(t, ".", got)
		assert.NotEqual(t, "..", got)
		assert.True(t, strings.HasSuffix(got, "99"), "兜底名应带 id: %q", got)
	}
}

func TestSanitizeDirNameCollapsesUnderscores(t *testing.T) {
	got := SanitizeDirName(5, "a___b")
	assert.Equal(t, "a_b-5", got, "连续下划线应折叠")
}

func TestSanitizeDirNameTrimsLeadingAndTrailingDots(t *testing.T) {
	// 首部 '.' 会产生隐藏目录；尾部 '.' 在部分文件系统上非法。
	got := SanitizeDirName(6, ".hidden.")
	assert.False(t, strings.HasPrefix(got, "."), "不得是隐藏目录")
	assert.Equal(t, "hidden-6", got)
}

func TestSanitizeDirNameBoundsLength(t *testing.T) {
	// 云盘对路径长度有限制，且过长不利人工排查。
	long := strings.Repeat("a", 500)
	got := SanitizeDirName(8, long)
	assert.LessOrEqual(t, len(got), dirNameMaxLen, "总长必须受控")
	assert.True(t, strings.HasSuffix(got, "-8"), "截断后仍需保留消歧后缀")
}

func TestSanitizeDirNameKeepsSafeCharactersIntact(t *testing.T) {
	assert.Equal(t, "user.name_01-abc-10", SanitizeDirName(10, "user.name_01-abc"),
		"合法字符不应被改动")
}

func TestSanitizeDirNameIsDeterministic(t *testing.T) {
	// 同一输入必须永远同一输出：否则同一用户的数据会散到不同目录。
	for _, name := range []string{"alice", "凌清秋", "a/b", "🎉", ""} {
		first := SanitizeDirName(11, name)
		for range 5 {
			require.Equal(t, first, SanitizeDirName(11, name), "输入 %q", name)
		}
	}
}

func TestSanitizeModelNameBasics(t *testing.T) {
	assert.Equal(t, "deepseek-v4.1-flash", SanitizeModelName("deepseek-v4.1-flash"))
	assert.Equal(t, "gpt-image-1", SanitizeModelName("gpt-image-1"))
	// 与用户名不同：**不追加 id**——同一模型应跨用户落到同名目录，便于聚合。
	assert.NotContains(t, SanitizeModelName("gpt-4o"), "-1")
}

func TestSanitizeModelNamePreventsTraversalAndEmpty(t *testing.T) {
	for _, attack := range []string{"../../x", "..", "", "   ", "///"} {
		got := SanitizeModelName(attack)
		assert.NotContains(t, got, "/", "输入 %q", attack)
		assert.NotEmpty(t, got, "输入 %q 不得产出空目录名", attack)
		assert.NotEqual(t, "..", got)
	}
	assert.Equal(t, "unknown-model", SanitizeModelName(""))
	assert.Equal(t, "unknown-model", SanitizeModelName("🎉"))
}

func TestSanitizeModelNameBoundsLength(t *testing.T) {
	got := SanitizeModelName(strings.Repeat("m", 500))
	assert.LessOrEqual(t, len(got), dirNameMaxLen)
}

func TestPartFileNameZeroPads(t *testing.T) {
	// 补零是**功能需求**而非美观：恢复分片序号靠「列举后取最大」，
	// 不补零时 `part-10` 字典序小于 `part-9`，重启会覆盖已上传分片。
	assert.Equal(t, "part-0001.jsonl.gz", PartFileName(1))
	assert.Equal(t, "part-0009.jsonl.gz", PartFileName(9))
	assert.Equal(t, "part-0010.jsonl.gz", PartFileName(10))
	assert.Equal(t, "part-9999.jsonl.gz", PartFileName(9999))
	// 超过 4 位自然增长（解析侧按数值比较兜底）。
	assert.Equal(t, "part-10000.jsonl.gz", PartFileName(10000))
}

func TestPartFileNameClampsNonPositive(t *testing.T) {
	// seq 0/负数无意义；夹到 1 而不是产出 `part-0000.jsonl`。
	assert.Equal(t, "part-0001.jsonl.gz", PartFileName(0))
	assert.Equal(t, "part-0001.jsonl.gz", PartFileName(-5))
}

func TestParsePartSeqRoundTrips(t *testing.T) {
	for _, seq := range []int{1, 2, 9, 10, 9999, 10000} {
		assert.Equal(t, seq, ParsePartSeq(PartFileName(seq)), "seq=%d", seq)
	}
}

func TestParsePartSeqRejectsForeignNames(t *testing.T) {
	// 云盘目录里可能有别的东西；非本程序命名的一律不认（返回 0）。
	for _, name := range []string{
		"", "part-.jsonl", "part-abc.jsonl", "part-0001.txt",
		"other-0001.jsonl", "part-0001.jsonl.gz.gz", "0001.jsonl",
	} {
		assert.Zero(t, ParsePartSeq(name), "不应解析 %q", name)
	}
}

func TestPickNextPartSeqUsesNumericMaxNotLexical(t *testing.T) {
	// 关键回归：位数不同时字典序失真（"part-9" > "part-10" 字面上成立），
	// 必须按**数值**取最大，否则重启后从 10 退回 10……或更糟，覆盖 part-10。
	names := []string{"part-0009.jsonl.gz", "part-0010.jsonl.gz", "part-0002.jsonl.gz"}
	assert.Equal(t, 11, PickNextPartSeq(names))
}

func TestPickNextPartSeqStartsAtOneWhenEmpty(t *testing.T) {
	assert.Equal(t, 1, PickNextPartSeq(nil))
	assert.Equal(t, 1, PickNextPartSeq([]string{}))
	// 只有无关文件时同样从 1 开始。
	assert.Equal(t, 1, PickNextPartSeq([]string{"readme.txt", "part-abc.jsonl"}))
}

func TestPickNextPartSeqIgnoresForeignAndHandlesFiveDigits(t *testing.T) {
	assert.Equal(t, 10001, PickNextPartSeq([]string{"part-10000.jsonl.gz", "notes.md"}))
}

func TestDateDirNameIsUTCAndStable(t *testing.T) {
	// 固定 UTC：目录名一旦写入云盘就不随部署时区变化，
	// 避免换机器/改 TZ 后新旧数据落到不同目录造成割裂。
	// 2026-10-14 00:30:00 UTC
	assert.Equal(t, "2026-10-14", DateDirName(1791937800))
	// 同一天的稍晚时刻仍是同一天。
	assert.Equal(t, "2026-10-14", DateDirName(1791937800+3600))
	// 跨 UTC 午夜则变。
	assert.Equal(t, "2026-10-15", DateDirName(1791937800+86400))
}
