package training_capture

import (
	"encoding/json"
	"strings"
	"testing"
)

// 对话还原（B9 基础）的测试。
//
// 重点不是「能拼出字符串」，而是三条**诚实性契约**：
//  1. 分片按**序号**拼接（不是字典序）——序错了整个还原就是乱的。
//  2. 坏行**计数并披露**，不能静默当没发生。
//  3. 超限**标记 Truncated**，不能把残缺当完整交出去。

func jsonlLine(t *testing.T, role, content string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"role": role, "content": content})
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}

func TestAssembleConversationConcatenatesInPartOrder(t *testing.T) {
	// 刻意打乱 map 传入顺序，验证按**分片序号**排序而非文件名/插入顺序。
	objects := map[string][]byte{
		"part-0003.jsonl.gz": []byte(jsonlLine(t, "request", "第三段")),
		"part-0001.jsonl.gz": []byte(jsonlLine(t, "request", "第一段")),
		"part-0010.jsonl.gz": []byte(jsonlLine(t, "request", "第十段")),
		"part-0002.jsonl.gz": []byte(jsonlLine(t, "request", "第二段")),
	}

	got := AssembleConversation(objects, 0)
	want := "第一段\n第二段\n第三段\n第十段"
	if got.Text != want {
		t.Fatalf("拼接顺序错误：\n期望 %q\n实际 %q", want, got.Text)
	}
	if got.TotalLines != 4 {
		t.Fatalf("总行数应为 4，实际 %d", got.TotalLines)
	}
	if len(got.Parts) != 4 {
		t.Fatalf("分片数应为 4，实际 %d", len(got.Parts))
	}
	// Parts 必须也是序号序。
	if got.Parts[0].ObjectName != "part-0001.jsonl.gz" {
		t.Fatalf("Parts 未按序号排序，首个为 %s", got.Parts[0].ObjectName)
	}
	if got.Truncated || got.MalformedLines != 0 {
		t.Fatalf("正常数据不该标记异常：truncated=%v malformed=%d",
			got.Truncated, got.MalformedLines)
	}
}

func TestAssembleConversationCountsMalformedLines(t *testing.T) {
	// 数据损伤必须**计数**：静默忽略会让分析端把残缺当完整。
	payload := strings.Join([]string{
		`{"role":"request","content":"好的一行"}`,
		`{这不是 JSON`,
		`{"role":"request"`, // 截断的一行
		`{"role":"request","content":"另一行好的"}`,
		``, // 空行不算坏行
	}, "\n")

	got := AssembleConversation(map[string][]byte{"part-0001.jsonl.gz": []byte(payload)}, 0)

	if got.MalformedLines != 2 {
		t.Fatalf("坏行应为 2，实际 %d", got.MalformedLines)
	}
	if got.TotalLines != 2 {
		t.Fatalf("有效行应为 2，实际 %d", got.TotalLines)
	}
	if !strings.Contains(got.Text, "好的一行") || !strings.Contains(got.Text, "另一行好的") {
		t.Fatalf("有效内容丢失：%q", got.Text)
	}
	// 坏行不得污染文本。
	if strings.Contains(got.Text, "这不是") {
		t.Fatalf("坏行内容混进了文本：%q", got.Text)
	}
}

func TestAssembleConversationMarksTruncation(t *testing.T) {
	// 超限必须**标记**而不是报错，也不能假装完整。
	big := strings.Repeat("x", 5000)
	objects := map[string][]byte{
		"part-0001.jsonl.gz": []byte(jsonlLine(t, "request", big)),
		"part-0002.jsonl.gz": []byte(jsonlLine(t, "request", big)),
	}

	got := AssembleConversation(objects, 3000)
	if !got.Truncated {
		t.Fatal("超限时必须置 Truncated=true（不能把残缺当完整）")
	}
	if len(got.Text) > 3000 {
		t.Fatalf("文本超过上限：%d", len(got.Text))
	}
	if len(got.Text) == 0 {
		t.Fatal("截断不该变成空")
	}
	// 行数仍应如实统计（截断的是文本，不是统计）。
	if got.TotalLines != 2 {
		t.Fatalf("行数统计不应受截断影响，实际 %d", got.TotalLines)
	}
}

func TestAssembleConversationRecordsRolesSeen(t *testing.T) {
	payload := jsonlLine(t, "request", "a") + jsonlLine(t, "assistant", "b") + jsonlLine(t, "request", "c")
	got := AssembleConversation(map[string][]byte{"part-0001.jsonl.gz": []byte(payload)}, 0)

	if len(got.Parts) != 1 {
		t.Fatalf("分片数错误：%d", len(got.Parts))
	}
	roles := got.Parts[0].RolesSeen
	// 去重且保持首次出现顺序：request 在前，assistant 在后。
	if len(roles) != 2 || roles[0] != "request" || roles[1] != "assistant" {
		t.Fatalf("角色统计错误：%v", roles)
	}
	if got.Parts[0].LineCount != 3 {
		t.Fatalf("行数错误：%d", got.Parts[0].LineCount)
	}
}

func TestAssembleConversationHandlesEmptyAndNonPartNames(t *testing.T) {
	// 空输入：不 panic，返回空结果。
	if got := AssembleConversation(nil, 0); got.Text != "" || got.TotalLines != 0 {
		t.Fatalf("空输入结果异常：%+v", got)
	}

	// 非分片命名的文件也要被读（不能丢数据），排在分片之后。
	objects := map[string][]byte{
		"part-0001.jsonl.gz": []byte(jsonlLine(t, "request", "分片内容")),
		"notes.txt":          []byte(jsonlLine(t, "request", "另一个文件")),
	}
	got := AssembleConversation(objects, 0)
	if !strings.Contains(got.Text, "分片内容") || !strings.Contains(got.Text, "另一个文件") {
		t.Fatalf("非分片文件被丢弃：%q", got.Text)
	}
	if !strings.HasPrefix(got.Text, "分片内容") {
		t.Fatalf("分片应排在前面：%q", got.Text)
	}
}

func TestAssembleConversationSkipsEmptyContentWithoutCountingAsMalformed(t *testing.T) {
	payload := `{"role":"request","content":""}` + "\n" +
		`{"role":"request","content":"有内容"}` + "\n"
	got := AssembleConversation(map[string][]byte{"part-0001.jsonl.gz": []byte(payload)}, 0)

	if got.MalformedLines != 0 {
		t.Fatalf("空 content 是合法记录，不该计入坏行：%d", got.MalformedLines)
	}
	if got.TotalLines != 2 {
		t.Fatalf("两行都应计入：%d", got.TotalLines)
	}
	if got.Text != "有内容" {
		t.Fatalf("空 content 不该产生空行：%q", got.Text)
	}
}

// ---------- 启发式切分（明确标注为启发式） ----------

func TestSplitHeuristicTurnsSplitsOnRoleLines(t *testing.T) {
	text := "system\n你是助手\nuser\n第一个问题\nassistant\n第一个回答\nuser\n第二个问题"
	turns := SplitHeuristicTurns(text)

	if len(turns) != 4 {
		t.Fatalf("应切出 4 段，实际 %d：%+v", len(turns), turns)
	}
	if turns[0].Role != "system" || turns[0].Text != "你是助手" {
		t.Fatalf("第 1 段错误：%+v", turns[0])
	}
	if turns[1].Role != "user" || turns[1].Text != "第一个问题" {
		t.Fatalf("第 2 段错误：%+v", turns[1])
	}
	if turns[3].Role != "user" || turns[3].Text != "第二个问题" {
		t.Fatalf("第 4 段错误：%+v", turns[3])
	}
}

func TestSplitHeuristicTurnsMarksUnknownLeadingSegment(t *testing.T) {
	// 分隔符之前的内容没有角色 → unknown（不猜）。
	turns := SplitHeuristicTurns("没有角色标记的开头\nuser\n问题")
	if len(turns) != 2 {
		t.Fatalf("应切出 2 段，实际 %d", len(turns))
	}
	if turns[0].Role != "unknown" {
		t.Fatalf("首段应标 unknown（不猜角色），实际 %q", turns[0].Role)
	}
}

func TestSplitHeuristicTurnsHandlesEmpty(t *testing.T) {
	if got := SplitHeuristicTurns(""); got != nil {
		t.Fatalf("空输入应返回 nil，实际 %+v", got)
	}
	if got := SplitHeuristicTurns("   \n  "); got != nil {
		t.Fatalf("纯空白应返回 nil，实际 %+v", got)
	}
}

func TestSplitHeuristicTurnsIsDocumentedAsLossy(t *testing.T) {
	// 记录**已知的失败模式**：用户消息里单独成行的 "user" 会被误判为分隔符。
	// 这个用例的存在是为了让「启发式不可靠」这件事有据可查，
	// 而不是作为「这里有 bug」——产品语义已明确它是启发式。
	text := "user\n请解释这个词：\nuser\n是什么意思"
	turns := SplitHeuristicTurns(text)

	// 误切：本应是 1 轮 user 消息，被切成 2 轮。
	if len(turns) != 2 {
		t.Fatalf("预期出现已知的误切（2 段），实际 %d 段 —— "+
			"若切分算法改了请同步更新本用例与 hotpath/conversation 的注释", len(turns))
	}
}

// TestAnnotatedTextDistinguishesRequestFromAssistant 锁定「送进分析提示词的正文
// 必须能区分用户侧与模型回复」。
//
// 这是回复采集上线后的**下游集成不变量**：
// 分析提示词要求「逐字引用用户原话」，若正文不加标注，模型会把**它自己的回复**
// 当成用户原话引用 —— 报告从根上不可信，而管理员完全看不出来。
// 旧实现只写 rec.Content（丢掉 role），正是这个缺陷。
func TestAnnotatedTextDistinguishesRequestFromAssistant(t *testing.T) {
	payload := jsonlLine(t, "request", "用户问的问题") +
		jsonlLine(t, "assistant", "模型给的答案")
	got := AssembleConversation(map[string][]byte{"part-0001.jsonl.gz": []byte(payload)}, 0)

	// 保真视图保持原样（不夹带我们加的标签）。
	if got.Text != "用户问的问题\n模型给的答案" {
		t.Fatalf("保真视图被污染：%q", got.Text)
	}

	// 标注视图必须让两类发言可区分。
	if !strings.Contains(got.AnnotatedText, "用户问的问题") ||
		!strings.Contains(got.AnnotatedText, "模型给的答案") {
		t.Fatalf("标注视图丢了内容：%q", got.AnnotatedText)
	}
	lines := strings.Split(got.AnnotatedText, "\n")
	if len(lines) != 2 {
		t.Fatalf("标注视图行数应为 2，实际 %d：%q", len(lines), got.AnnotatedText)
	}
	reqLine, asstLine := lines[0], lines[1]
	if !strings.Contains(reqLine, "用户问的问题") || !strings.Contains(asstLine, "模型给的答案") {
		t.Fatalf("行序被破坏：\n第 1 行 %q\n第 2 行 %q", reqLine, asstLine)
	}
	// 关键：请求侧那一行的标注里必须**没有**「模型回复」字样，
	// 否则模型会把用户的话当成自己说过的话（或反之）。
	if strings.Contains(reqLine, "模型回复") {
		t.Fatalf("请求侧被标成模型回复：%q", reqLine)
	}
	if !strings.Contains(asstLine, "模型回复") {
		t.Fatalf("助手行未被标注为模型回复：%q", asstLine)
	}
}

// TestAnnotatedTextPreservesUnknownRoles 未知/空角色不得被误标成两类之一。
func TestAnnotatedTextPreservesUnknownRoles(t *testing.T) {
	payload := jsonlLine(t, "system", "系统提示") + jsonlLine(t, "tool", "工具结果")
	got := AssembleConversation(map[string][]byte{"part-0001.jsonl.gz": []byte(payload)}, 0)

	// system/tool 不得被冒充成 request 或 assistant。
	if strings.Contains(got.AnnotatedText, "模型回复") {
		t.Fatalf("system/tool 被误标为模型回复：%q", got.AnnotatedText)
	}
	if strings.Contains(got.AnnotatedText, "请求侧") {
		t.Fatalf("system/tool 被误标为请求侧：%q", got.AnnotatedText)
	}
	for _, want := range []string{"系统提示", "工具结果"} {
		if !strings.Contains(got.AnnotatedText, want) {
			t.Fatalf("内容丢失 %q：%q", want, got.AnnotatedText)
		}
	}
}

// TestAnnotatedTextTruncationIsReported 标注视图更长，可能先触顶；
// 触顶必须如实置 Truncated（不能悄悄少内容）。
func TestAnnotatedTextTruncationIsReported(t *testing.T) {
	var sb strings.Builder
	for range 50 {
		sb.WriteString(jsonlLine(t, "assistant", strings.Repeat("回复内容", 40)))
	}
	got := AssembleConversation(map[string][]byte{"part-0001.jsonl.gz": []byte(sb.String())}, 2000)

	if !got.Truncated {
		t.Fatal("被截断必须置 Truncated（两个视图任一触顶都算）")
	}
	if len(got.AnnotatedText) > 2000 {
		t.Fatalf("标注视图超过上限：%d", len(got.AnnotatedText))
	}
	if len(got.Text) > 2000 {
		t.Fatalf("保真视图超过上限：%d", len(got.Text))
	}
}
