package training_capture

import (
	"fmt"
	"strings"
	"unicode"
)

// 云盘目录名与分片文件名的生成（纯函数，无 IO）。
//
// ## 为什么目录名必须净化（安全，不是美观）
//
// 云盘路径由「日期 / 用户名 / 模型名」三段拼成，其中**用户名是用户可控输入**。
// 直接拼接有两个真实后果：
//  1. **路径穿越**：用户名 `../../etc` 或 `..%2f` 会让写入逃出预期的目录树，
//     覆盖其它用户的数据（OneDrive 用 `root:/{path}:/content` 形式提交路径，
//     对分隔符敏感）。
//  2. **互相覆盖**：两个不同用户名为 `a/b` 和 `a_b` 时会落到同一目录，
//     把彼此的数据混在一起——训练数据集的用户隔离就此失效。
//
// 因此：**白名单化 + 消歧后缀**，绝不直接使用原始用户名。

const (
	// dirNameMaxLen 目录名单段最大长度。云盘对路径长度有限制，
	// 且过长会让人工排查困难。
	dirNameMaxLen = 64

	// userIDFallbackPrefix 用户名净化后为空时的兜底前缀。
	userIDFallbackPrefix = "user-"

	// partFilePrefix 分片文件名前缀。
	partFilePrefix = "part-"

	// partFileSuffix 分片文件名后缀。逐行 JSON（JSONL）便于流式追加与逐条读取。
	partFileSuffix = ".jsonl"

	// partFileDigits 分片序号补零位数。
	//
	// **必须补零**：不补零时 `part-10` 在字典序里排在 `part-9` 前面，
	// 而恢复分片序号正是靠「列举后取字典序最大」——不补零会让重启后
	// 从头覆盖已上传的分片。4 位可支撑 9999 个分片（按 8MB/分片 ≈ 78GB/天/用户/模型）。
	partFileDigits = 4
)

// SanitizeDirName 把任意用户名净化为**安全的目录名**。
//
// 规则（顺序即契约）：
//  1. 只保留 ASCII 字母、数字与 `-` `_` `.`
//  2. 连续 `_` 折叠为一个（避免 `a___b` 这类噪声名）
//  3. 去掉首尾的 `.` 与 `_`（首部的 `.` 会产生隐藏目录；尾部的 `.` 在部分
//     文件系统上非法）
//  4. 结果为空、或为 `.` / `..` → 用 `user-<id>` 兜底
//  5. 截断到 dirNameMaxLen
//  6. **始终追加 `-<userID>`** 消歧
//
// 第 6 条是**无条件**的，不是「冲突时才加」：判断「是否冲突」需要全局视图
// （要知道所有用户名），而这是一个纯函数、且并发写入时也无从协调。
// 无条件加后缀让「不同用户必然不同目录」成为**结构性保证**，
// 代价只是目录名稍长。
func SanitizeDirName(userID int, username string) string {
	var b strings.Builder
	b.Grow(len(username))

	lastWasUnderscore := false
	for _, r := range username {
		switch {
		case (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '.':
			b.WriteRune(r)
			lastWasUnderscore = false
		case r == '_':
			// 折叠连续的 '_'（'__' → '_'）。
			if !lastWasUnderscore {
				b.WriteRune('_')
				lastWasUnderscore = true
			}
		default:
			// 其它一切（中文、emoji、`/`、`\`、空格、控制字符…）替换为 '_'。
			// 统一替换而不是丢弃：保留「这里有内容」的位置感，
			// 让 `张三`/`李四` 不会都变成空串。
			if !lastWasUnderscore {
				b.WriteRune('_')
				lastWasUnderscore = true
			}
		}
	}

	name := strings.Trim(b.String(), "._")

	// 兜底：净化后无有效字符，或退化成了路径穿越片段。
	if name == "" {
		name = userIDFallbackPrefix + fmt.Sprint(userID)
	} else {
		if len(name) > dirNameMaxLen {
			name = strings.Trim(name[:dirNameMaxLen], "._")
			if name == "" {
				name = userIDFallbackPrefix + fmt.Sprint(userID)
			}
		}
		// 消歧后缀。截断后再截一次，保证总长仍受控。
		suffix := "-" + fmt.Sprint(userID)
		maxBase := dirNameMaxLen - len(suffix)
		if maxBase < 1 {
			// userID 极大导致后缀本身就超长：纯用兜底名。
			return userIDFallbackPrefix + fmt.Sprint(userID)
		}
		if len(name) > maxBase {
			name = strings.Trim(name[:maxBase], "._")
			if name == "" {
				name = userIDFallbackPrefix[:len(userIDFallbackPrefix)-1]
			}
		}
		name += suffix
	}

	// 双重保险：兜底名同样要过一遍「不含路径分隔符」的检查。
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return userIDFallbackPrefix + fmt.Sprint(userID)
	}
	return name
}

// SanitizeModelName 把模型名净化为安全的目录名。
//
// 模型名通常来自后端配置（管理员可控），但**也可能来自用户请求**
// （自定义模型名/模型映射），因此同样净化。
//
// 与用户名不同：模型名**不追加 id 消歧**（模型名本身就是标识，
// 且同一模型在所有用户下应落到同名目录，便于跨用户聚合）。
func SanitizeModelName(model string) string {
	var b strings.Builder
	b.Grow(len(model))
	lastWasUnderscore := false
	for _, r := range model {
		switch {
		case (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '.':
			b.WriteRune(r)
			lastWasUnderscore = false
		default:
			if !lastWasUnderscore {
				b.WriteRune('_')
				lastWasUnderscore = true
			}
		}
	}
	name := strings.Trim(b.String(), "._")
	if name == "" {
		return "unknown-model"
	}
	if len(name) > dirNameMaxLen {
		name = strings.Trim(name[:dirNameMaxLen], "._")
		if name == "" {
			return "unknown-model"
		}
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "unknown-model"
	}
	return name
}

// PartFileName 生成分片文件名（零填充，字典序 == 数字序）。
//
// seq 从 1 开始。超过 4 位时自然增长（`part-10000.jsonl`），
// 此时字典序会失真，但 9999 个分片（≈78GB/天/用户/模型）已远超实际；
// 万一到达，`List` 的解析会按数值比较兜底（见 PickNextPartSeq）。
func PartFileName(seq int) string {
	if seq < 1 {
		seq = 1
	}
	return fmt.Sprintf("%s%0*d%s", partFilePrefix, partFileDigits, seq, partFileSuffix)
}

// ParsePartSeq 从分片文件名解析序号。非本程序命名规则的文件返回 0。
func ParsePartSeq(name string) int {
	if !strings.HasPrefix(name, partFilePrefix) || !strings.HasSuffix(name, partFileSuffix) {
		return 0
	}
	digits := strings.TrimSuffix(strings.TrimPrefix(name, partFilePrefix), partFileSuffix)
	if digits == "" {
		return 0
	}
	seq := 0
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0
		}
		seq = seq*10 + int(r-'0')
	}
	return seq
}

// PickNextPartSeq 从已存在的文件名列表算出**下一个应写入的分片序号**。
//
// 这是「重启后不覆盖已上传分片」的实现：按**数值**取最大而非字典序
// （字典序在位数不同时会失真）。
// 空列表返回 1。非本程序命名规则的文件被忽略。
func PickNextPartSeq(names []string) int {
	maxSeq := 0
	for _, name := range names {
		if seq := ParsePartSeq(name); seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq + 1
}

// DateDirName 生成一级目录名（UTC 日期，`YYYY-MM-DD`）。
//
// 用 UTC 而非服务器本地时区：容器时区可配（本部署设 `TZ=Asia/Shanghai`），
// 而**目录名一旦写进云盘就不会随部署变化**。固定 UTC 让「同一天」的定义
// 不依赖部署配置，避免换机器/改 TZ 后新旧数据落到不同目录造成割裂。
// 展示层需要本地日期时由前端换算。
func DateDirName(unixSeconds int64) string {
	return timeUTCDate(unixSeconds)
}

// isEmptyDirSegment 报告净化后的段是否仍是危险值（防御性，供断言与测试）。
func isEmptyDirSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return true
	}
	if strings.ContainsAny(s, `/\`) {
		return true
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
