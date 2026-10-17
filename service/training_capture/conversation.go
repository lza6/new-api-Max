package training_capture

import (
	"bufio"
	"bytes"
	"github.com/lza6/new-api-Max/common"

	"sort"
	"strings"
)

// 把采集到的分片还原成可分析的对话文本（B9 的基础，纯逻辑、无 IO）。
//
// ## 数据形态与诚实边界（决定了能还原到什么程度）
//
// 分片是 gzip 压缩的 JSONL，每行 `{"role":"request","content":"..."}`。
// `content` 是上游 `TokenCountMeta.CombineText` —— 把整段对话里
// **所有消息的 role 与文本按顺序拼成的一个字符串**（见 hotpath.go）。
//
// 因此：
//   - 一次请求 = 一行。**首行是该会话当时的全量快照**，
//     之后每行是**相对上一行的新增后缀**（增量上传，见 text_increment.go）。
//   - **按行序拼接**即可还原出「最后一次请求时的完整对话文本」。
//   - 但**逐轮 role 无法可靠还原**：拼接把 role 和文本混在一起，
//     且用户消息里若恰好有单独成行的 "user"/"assistant" 会造成误切。
//
// 所以本文件只做**忠实拼接**，并提供**启发式的角色切分**供展示参考，
// 且明确标注它是启发式 —— 不假装是精确的逐轮结构化还原。
// 分析由 LLM 完成：它读得懂这段文本里的角色标记。

// ConversationPart 一个分片还原出的对话内容。
type ConversationPart struct {
	// ObjectName 分片文件名（如 part-0001.jsonl.gz）。
	ObjectName string
	// Text 该分片还原出的文本（行内容按序拼接）。
	Text string
	// LineCount 该分片包含的记录行数。
	LineCount int
	// RolesSeen 出现过的 role 值（去重、按首次出现排序）。
	RolesSeen []string
}

// AssemblyResult 一次「多分片 → 对话文本」的还原结果。
type AssemblyResult struct {
	// Text 完整对话文本（所有分片按序号拼接），**忠实拼接、不加任何标注**。
	Text string
	// AnnotatedText 与 Text 同源，但每行带**角色标注**（见 renderLineForPrompt）。
	//
	// 专门给「送进模型做分析」的提示词用：分析要求「逐字引用用户原话」，
	// 而不加标注的 Text 里用户侧与模型回复混在一起，模型会把模型自己的
	// 回复当成用户原话引用 —— 报告从根上不可信且管理员看不出来。
	//
	// 保留 Text 不合并的理由：Text 是**数据保真视图**（将来若要让用户
	// 导出/查看自己的数据，可分析的原文不该夹带我们加的中文标签）。
	// 两者由同一次扫描产出，不存在重复解析或状态漂移。
	AnnotatedText string
	// Parts 各分片的信息（按序号升序）。
	Parts []ConversationPart
	// TotalLines 总记录行数。
	TotalLines int
	// Truncated 为 true 表示因超出 maxBytes 而**未还原完整对话**。
	// 调用方必须向管理员披露这一点（不能把残缺当完整展示）。
	Truncated bool
	// MalformedLines 无法解析的 JSON 行数。
	// 非零说明数据有损伤；调用方应提示而不是静默忽略。
	MalformedLines int
}

// lineRecord JSONL 一行的结构。
type lineRecord struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// renderLineForPrompt 把一行记录渲染成**带角色标注**的文本，供分析提示词使用。
//
// ## 为什么必须标注角色
//
// 采集记录里现在有两类行（见 dedup.go 的 `kind` 与 worker 的回复写入）：
//   - `role:"request"`  —— 上游拼接后的请求侧文本（含历史各轮）
//   - `role:"assistant"` —— 模型对该轮的回复
//
// 而分析提示词要求「**逐字引用用户原话**」。若把两类行不加区分地
// 顺序拼接，模型无法判断某段文本是用户说的还是模型说的 ——
// 于是「用户原话摘录」会把**模型自己的回复**当成用户原话引用，
// 分析报告从根上不可信，而管理员看不出这一点。
//
// 标注是**数据结构事实**（role 字段就是这么存的），不是猜测 ——
// 与 `SplitHeuristicTurns` 那种「从拼接文本里猜角色」是两件事，不要混淆：
// 那个是启发式（会误判），这个是读取现成字段（不会误判）。
func renderLineForPrompt(rec lineRecord) string {
	switch rec.Role {
	case "assistant":
		return "【模型回复】" + rec.Content
	case "request":
		// request 行内部混合了多轮 role，逐轮无法可靠还原
		// （见本文件头部说明），故只标注「这是请求侧」。
		return "【请求侧（含历史各轮拼接，逐轮角色不可靠）】" + rec.Content
	default:
		if rec.Role == "" {
			return rec.Content
		}
		return "【" + rec.Role + "】" + rec.Content
	}
}

// AssembleConversation 把多分片还原成对话文本。
//
// `objects` 是「文件名 → 已解压的明文 JSONL 字节」，调用方负责解压
// （解压是 IO + 有 zip-bomb 风险，故放在 sink/controller 层做）。
//
// 按**文件名中的分片序号**排序拼接（不是字典序，见 PickNextPartSeq）。
//
// maxBytes <= 0 表示不限制；超限时**截断并置 Truncated**，
// 而不是返回错误 —— 管理端分析「前 1MB」比「什么都看不到」有用，
// 前提是把截断事实披露出去。
func AssembleConversation(objects map[string][]byte, maxBytes int) AssemblyResult {
	names := make([]string, 0, len(objects))
	for name := range objects {
		names = append(names, name)
	}
	// 按分片序号排序；非分片命名的文件排到最后（仍会被读，不丢数据）。
	sort.SliceStable(names, func(i, j int) bool {
		si, sj := ParsePartSeq(names[i]), ParsePartSeq(names[j])
		if si == 0 && sj == 0 {
			return names[i] < names[j]
		}
		if si == 0 {
			return false
		}
		if sj == 0 {
			return true
		}
		return si < sj
	})

	var result AssemblyResult
	var sb strings.Builder
	// 第二个 builder：与 sb 逐行同源，但带角色标注（供分析提示词用）。
	var annotated strings.Builder
	if maxBytes > 0 {
		sb.Grow(min(maxBytes, 1<<20))
		annotated.Grow(min(maxBytes, 1<<20))
	}

	for _, name := range names {
		part := ConversationPart{ObjectName: name}
		seen := map[string]bool{}

		scanner := bufio.NewScanner(bytes.NewReader(objects[name]))
		// 单行上限放大到 8MB：一条记录可能包含很长的对话拼接文本
		// （budget 允许单条到 8MB）。默认 64KB 会把正常数据判为「行太长」。
		scanner.Buffer(make([]byte, 64<<10), 8<<20)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var rec lineRecord
			if err := common.Unmarshal([]byte(line), &rec); err != nil {
				result.MalformedLines++
				continue
			}
			part.LineCount++
			if rec.Role != "" && !seen[rec.Role] {
				seen[rec.Role] = true
				part.RolesSeen = append(part.RolesSeen, rec.Role)
			}
			if rec.Content == "" {
				continue
			}
			// 超限：记录事实后停止追加（不报错）。
			// 两个视图各自按自己的长度判定 —— 标注会加前缀，两者长度不同，
			// 用其中一个的值去卡另一个会让实际下发的正文与上限不符。
			overBudget := maxBytes > 0 && (sb.Len() >= maxBytes || annotated.Len() >= maxBytes)
			if overBudget {
				result.Truncated = true
				continue
			}
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(rec.Content)

			if annotated.Len() > 0 {
				annotated.WriteByte('\n')
			}
			annotated.WriteString(renderLineForPrompt(rec))
		}
		result.Parts = append(result.Parts, part)
		result.TotalLines += part.LineCount

		// [修复防御] **必须检查 scanner.Err()**。
		// bufio.Scanner 遇到超过 Buffer 上限的单行时，Scan() 返回 false，
		// 该分片**剩余的所有行被静默跳过** —— 而 MalformedLines/Truncated
		// 都还是 0/false，调用方（分析接口）会把这半截数据当完整正文
		// 喂给模型，并且不告诉模型「有内容被丢」。
		// 这正是本文件反复声明的「不假装完整」的反面。
		if err := scanner.Err(); err != nil {
			result.MalformedLines++
			result.Truncated = true
		}
	}

	result.Text = sb.String()
	// 二次确认：即使没走到上面的提前判定，也要如实反映「内容被切过」。
	if maxBytes > 0 && len(result.Text) > maxBytes {
		result.Text = truncateText(result.Text, maxBytes)
		result.Truncated = true
	}

	result.AnnotatedText = annotated.String()
	// 同一道二次确认用在标注视图上（它更长，可能先于正文触顶）。
	if maxBytes > 0 && len(result.AnnotatedText) > maxBytes {
		result.AnnotatedText = truncateText(result.AnnotatedText, maxBytes)
		result.Truncated = true
	}
	return result
}

// HeuristicTurn 启发式切分出的一个「疑似轮次」。
type HeuristicTurn struct {
	// Role 猜测的角色（"user" / "assistant" / "unknown"）。
	Role string
	// Text 该轮文本。
	Text string
}

// SplitHeuristicTurns 用启发式把拼接文本切成疑似轮次。
//
// ## 为什么只能叫「启发式」
//
// 文本形如：
//
//	system
//	<系统提示>
//	user
//	<用户第1轮>
//	assistant
//	<助手第1轮>
//
// 切分规则：**单独成行**且等于已知 role 词的行作为分隔符。
// 失败模式（必须向调用方披露）：
//   - 用户消息里若自己有一行写着 "user" / "assistant"，会被误判为分隔符；
//   - 用户消息以 "system" 等词单独成行时同理；
//   - 第一段（分隔符之前）没有角色，标为 unknown。
//
// 因此本函数的输出**仅供展示参考**，不可作为「精确对话结构」使用。
// 真正的分析交给 LLM —— 它读整段文本比这里的规则切分更可靠。
func SplitHeuristicTurns(text string) []HeuristicTurn {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	roleWords := map[string]bool{"system": true, "user": true, "assistant": true, "tool": true}

	var turns []HeuristicTurn
	var cur strings.Builder
	currentRole := "unknown"

	flush := func() {
		body := strings.TrimSpace(cur.String())
		if body != "" {
			turns = append(turns, HeuristicTurn{Role: currentRole, Text: body})
		}
		cur.Reset()
	}

	for _, raw := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(raw)
		if roleWords[strings.ToLower(trimmed)] {
			flush()
			currentRole = strings.ToLower(trimmed)
			continue
		}
		if cur.Len() > 0 {
			cur.WriteByte('\n')
		}
		cur.WriteString(raw)
	}
	flush()
	return turns
}
