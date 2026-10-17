package training_capture

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// 训练数据「一键导出」的打包实现（增量需求）。
//
// ## 为什么选 tar（在候选里权衡后的结论）
//
// 候选与取舍：
//
//  1. **zip** —— 需要一个「中央目录」写在文件末尾，因此**无法顺序生成后
//     直接流式下载**（必须先把全部成员写完回到开头写目录）；且 zip 不便于
//     逐成员流式消费。淘汰。
//  2. **逐文件下载清单**（不打包，只给一堆链接）—— 训练管线要自己循环，
//     且**断点续传语义会碎成「每个文件各自续传」**，用户体验与实现都更差。
//     淘汰为默认，作为「直链」之外的补充不必要。
//  3. **直接拼接 JSONL**（把各分片的明文 JSONL 首尾相接成一个大 .jsonl）——
//     最贴合训练管线（`jsonlines` reader 直接吃），但**丢失了「哪个用户/模型」
//     的目录结构与元数据**，且单个坏分片无法被隔离（拼接后无法定位来源）。
//  4. **tar** —— 顺序写、可流式生成、成员边界清晰（每个云盘分片是一个
//     tar 成员，保留原始相对路径），解包即还原目录结构；配合一个
//     `manifest.json` 记录导出范围与统计，训练管线既能 `tar -x` 拿目录树，
//     也能逐成员流式读。**选它。**
//
// 归档成员布局：
//
//	manifest.json                    导出范围、统计、生成时间（元数据）
//	<云盘相对路径>.jsonl.gz          原样搬运的压缩分片（未改动字节）
//
// 刻意**原样搬运 gzip 分片**而不在打包时解压再拼接：
//   - 不改变数据字节 → 校验和可对齐云盘原始文件；
//   - 打包过程只是 IO 搬运，CPU 近乎为零（gzip 是 CPU 热点，几十 GB 级
//     重新压缩会显著拖长打包时间且无收益）；
//   - 消费方何时解压由它自己决定（与既有 `.jsonl.gz` 契约一致）。

const (
	// exportManifestName 归档内的元数据成员名。
	exportManifestName = "manifest.json"

	// ExportFormatCurrent 当前记录形态标识。
	// 形态演进时必须改这个值 —— 消费方据此判断能否解析（见 ExportManifest.Format）。
	// 版本号含义（消费方据此判断能否解析）：
	//   v1 = 每行 {role,content}，role ∈ {request, assistant}。
	//   v2 = 在 v1 基础上给 request 行加  字段（full|suffix），
	//        用于区分「完整快照」与「增量后缀」—— 不做这个区分会让消费方
	//        把历史重复拼接 N 遍（见 Message.Kind 的说明）。
	// **v1 消费方读到 v2 数据必须拒绝**：它不知道 kind，会把后缀当全量。
	ExportFormatCurrent = "gateway-conversation-v2"

	// exportPartExt 归档内分片后缀（与云盘一致，显式声明「需解压」）。
	exportPartExt = ".jsonl.gz"

	// exportMaxParts 单个导出包含的分片数硬顶。
	// 防「日期范围写错（如 1970-01-01~2099-01-01）」把整云盘都拉进来：
	// 一次导出几十万个分片既不现实也会打爆云盘 API。
	exportMaxParts = 100000

	// exportListTreeMaxEntries 列举云盘时的条目硬顶。
	exportListTreeMaxEntries = 100000
)

// ExportListTreeMaxEntries 是对外暴露的云盘列举条目硬顶（供 controller 传参）。
const ExportListTreeMaxEntries = exportListTreeMaxEntries

// ErrExportRangeInvalid 导出日期范围非法。
var ErrExportRangeInvalid = errors.New("training export: invalid date range")

// ValidateExportRange 校验日期范围（含两端）并归一化为 UTC 日期字符串。
//
// 规则（顺序即契约）：
//  1. 两端都必须能按 YYYY-MM-DD 解析；
//  2. from <= to（倒序范围没有任何意义，明确拒绝而不是静默交换 ——
//     静默交换会让管理员以为「我填反了也能work」，从而不修正）；
//  3. 跨度上限（防误填把整个历史拉进一次导出）。
func ValidateExportRange(from, to string, maxDays int) (string, string, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return "", "", fmt.Errorf("%w: date_from and date_to are required", ErrExportRangeInvalid)
	}
	fromT, err := time.Parse("2006-01-02", from)
	if err != nil {
		return "", "", fmt.Errorf("%w: date_from must be YYYY-MM-DD", ErrExportRangeInvalid)
	}
	toT, err := time.Parse("2006-01-02", to)
	if err != nil {
		return "", "", fmt.Errorf("%w: date_to must be YYYY-MM-DD", ErrExportRangeInvalid)
	}
	if toT.Before(fromT) {
		return "", "", fmt.Errorf("%w: date_to must not be before date_from", ErrExportRangeInvalid)
	}
	if maxDays > 0 {
		days := int(toT.Sub(fromT).Hours()/24) + 1
		if days > maxDays {
			return "", "", fmt.Errorf("%w: range spans %d days, exceeds limit %d", ErrExportRangeInvalid, days, maxDays)
		}
	}
	return fromT.Format("2006-01-02"), toT.Format("2006-01-02"), nil
}

// ExportSelection 一次导出的选择条件。
type ExportSelection struct {
	DateFrom string
	DateTo   string
	// UserId 可选（0 表示不按用户过滤）。匹配云盘路径里的 `-<id>` 后缀段。
	UserId int
	// Model 可选（空表示不按模型过滤）。
	Model string
}

// IsCanonicalDate 判断字符串是否为**规范化**的 `YYYY-MM-DD` 日期。
//
// parse 后 round-trip 必须相等：既排除了非日期字符串，也排除了
// `2026-10-1`（parse 成功但 Format 回 `2026-10-01` 不等）、`2026-1-01` 等。
//
// 导出的理由：日期是本模块的**目录名格式**（`DateDirName` 恒产出补零形式），
// 而调用方（HTTP 层）也要校验用户传来的日期。若各写各的，
// 两处对「`2026-10-1` 算不算合法」的答案可能不一致 ——
// 那会让「导出的范围」与「校验通过的范围」悄悄错开。
func IsCanonicalDate(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return false
	}
	return t.Format("2006-01-02") == s
}

// ExportPart 一个待打包的云盘分片（相对云盘根的路径 + 大小）。
type ExportPart struct {
	Path      string
	SizeBytes int64
}

// SelectExportParts 从云盘枚举出的目录树中筛选出属于该导出范围的**分片文件**。
//
// 云盘布局为 `<日期>/<用户名-id>/<模型>/part-NNNN.jsonl.gz`（见 naming.go）。
// 按路径分段匹配而不是字符串前缀：避免 `2026-10-1` 误匹配 `2026-10-14`。
//
// 过滤语义：
//   - 日期段（第 1 段）必须落在 [DateFrom, DateTo] 闭区间（字符串比较，格式固定）。
//   - UserId>0 时，第 2 段必须**以 `-<id>` 结尾**（SanitizeDirName 的无条件消歧后缀）。
//   - Model 非空时，第 3 段必须等于净化后的模型名。
func SelectExportParts(entries []TreeEntry, sel ExportSelection) []ExportPart {
	modelWant := ""
	if sel.Model != "" {
		modelWant = SanitizeModelName(sel.Model)
	}
	userSuffix := ""
	if sel.UserId > 0 {
		userSuffix = "-" + fmt.Sprint(sel.UserId)
	}

	out := make([]ExportPart, 0, 64)
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		if !strings.HasSuffix(e.Path, exportPartExt) && !strings.HasSuffix(e.Path, ".jsonl") {
			continue
		}
		segments := strings.Split(strings.Trim(e.Path, "/"), "/")
		if len(segments) < 4 {
			// 期望 `<日期>/<用户>/<模型>/<文件>`；不满足的跳过（不是本程序的布局）。
			continue
		}
		date := segments[0]
		// **必须是规范化日期**：云盘目录名由 DateDirName 生成，恒为补零的
		// `YYYY-MM-DD`。但目录树可能有历史/手工产生的非补零目录（如 `2026-10-1`），
		// 它与 `2026-10-01` 的**字符串比较会串味**：`"2026-10-1"` 既 > `"2026-10-01"`
		// 又 < `"2026-10-14"`，会被错误地纳入 `2026-10-01..2026-10-14` 范围。
		// 因此先做格式校验（parse 后 round-trip 相等），不规范的直接跳过。
		if !IsCanonicalDate(date) {
			continue
		}
		if date < sel.DateFrom || date > sel.DateTo {
			continue
		}
		if userSuffix != "" && !strings.HasSuffix(segments[1], userSuffix) {
			continue
		}
		if modelWant != "" && segments[2] != modelWant {
			continue
		}
		out = append(out, ExportPart{Path: e.Path, SizeBytes: e.SizeBytes})
		if len(out) >= exportMaxParts {
			break
		}
	}
	// 稳定排序，让归档成员顺序确定（便于复现与校验）。
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ExportManifest 归档内的元数据成员。
type ExportManifest struct {
	DateFrom      string `json:"date_from"`
	DateTo        string `json:"date_to"`
	UserId        int    `json:"user_id,omitempty"`
	Model         string `json:"model,omitempty"`
	ArchiveFormat string `json:"archive_format"`
	PartCount     int    `json:"part_count"`
	RecordCount   int    `json:"record_count"`
	TotalBytes    int64  `json:"total_bytes"`
	GeneratedAt   int64  `json:"generated_at"`
	// Provider 数据来源云盘后端名（便于追溯是哪个后端采集的）。
	Provider string `json:"provider"`

	// ---- 格式自描述（盲点 B5 的修复）----
	//
	// 数据一旦离开本站（导出给训练管线/第三方），消费方**无从得知**
	// 记录是什么形态、要不要解压、增量语义是什么。此前这些只存在于
	// 代码注释里 —— 对消费方等于不存在，会导致静默误读/误训。
	// 因此把契约写进 manifest，让归档**自描述**。
	//
	// 这些字段是给机器读的（稳定标识），配套的散文说明见 FormatNotes。

	// Format 记录形态标识。当前恒为 "gateway-conversation-v1"。
	// **消费方应先判 Format 再解析**：将来形态演进时此值会变，
	// 不认识就应拒绝而不是猜。
	Format string `json:"format"`
	// PartEncoding 分片编码："gzip-jsonl"（需先 gunzip 再按行解析）。
	PartEncoding string `json:"part_encoding"`
	// ExtractHint 给人/脚本看的一步到位解压命令。
	ExtractHint string `json:"extract_hint"`
	// FormatNotes 诚实说明数据的形态与局限（消费方必读）。
	FormatNotes []string `json:"format_notes"`
}

// ExportBuilder 把选中的云盘分片流式写入一个 tar 归档。
//
// ## 为什么是「build 到本地文件」而不是「边读边推给客户端」
//
// 断点续传要求产物**可随机寻址**（Range 请求要能 seek 到任意偏移）。
// 边读边推是单向流，客户端断线后无法从中间继续（除非云盘侧支持对
// 单个文件做 Range —— 但归档是多个文件的聚合，不成立）。
// 因此：先打包成**本地产物**（可 seek 的普通文件），下载走
// `http.ServeContent`（原生支持 Range/If-Range/206/416）。
//
// ## 内存边界
//
// 逐分片读取（每个分片由 sink 层保证 ≤64MB，且云盘上的压缩分片远小于此），
// 读一个 → 写一个 → 释放。**绝不**把整个数据集读进内存。
type ExportBuilder struct {
	// Reader 云盘读取能力（SinkReader）。
	Reader SinkReader
	// Provider 后端名（写入 manifest）。
	Provider string
	// PartReadMaxBytes 单分片读取上限（传给 ReadFile）。
	PartReadMaxBytes int64
	// Now 时间注入（测试可覆盖）。
	Now func() int64
}

// Build 执行打包：从云盘读选中分片 → 写 tar → 落盘到 dstPath。
//
// 返回实际写入的字节数与统计。任何一步失败都返回错误（**不吞**），
// 并把部分产物删除（半个归档比没有更危险 —— 会被误当成完整数据集）。
func (b *ExportBuilder) Build(ctx context.Context, sel ExportSelection, parts []ExportPart, dstPath string) (int64, ExportManifest, error) {
	var manifest ExportManifest
	if b.Reader == nil {
		return 0, manifest, errors.New("training export: no reader available (cloud drive not configured)")
	}
	now := time.Now().Unix
	if b.Now != nil {
		now = b.Now
	}
	maxBytes := b.PartReadMaxBytes
	if maxBytes <= 0 {
		maxBytes = onedriveReadMaxBytes
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
		return 0, manifest, err
	}
	// 先写临时文件，成功后原子重命名：避免半截产物被下载端读到。
	tmpPath := dstPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return 0, manifest, err
	}
	// 统一清理：任何错误路径都删临时文件。
	cleanupTmp := func() { _ = f.Close(); _ = os.Remove(tmpPath) }

	// 用计数 writer 精确统计归档字节（tar writer 本身不报总大小）。
	counter := &countingWriter{w: f}
	tw := tar.NewWriter(counter)

	// manifest 成员：先写（消费方一开头就能读到范围与统计的声明；
	// 但统计值在末尾才准 —— 因此这里先写**占位**，最后不重写，
	// 改为在 manifest 里如实标注为「打包前枚举的统计」）。
	// 为简单与诚实的取舍：manifest 记录的是**打包时已知的枚举统计**
	// （分片数=选中数、总字节=云盘报的压缩后大小），而非逐个重算。
	var totalBytes int64
	for _, p := range parts {
		totalBytes += p.SizeBytes
	}
	manifest = ExportManifest{
		DateFrom:      sel.DateFrom,
		DateTo:        sel.DateTo,
		UserId:        sel.UserId,
		Model:         sel.Model,
		ArchiveFormat: "tar",
		PartCount:     len(parts),
		TotalBytes:    totalBytes,
		GeneratedAt:   now(),
		Provider:      b.Provider,
		// 格式自描述（见 ExportManifest 的说明）。
		Format:       ExportFormatCurrent,
		PartEncoding: "gzip-jsonl",
		ExtractHint:  "for f in $(find . -name 'part-*.jsonl.gz'); do gzip -dc \"$f\" >> all.jsonl; done",
		FormatNotes:  exportFormatNotes(),
	}
	manifestBytes, err := common.Marshal(manifest)
	if err != nil {
		cleanupTmp()
		return 0, manifest, err
	}
	// 缩进便于人工查看（manifest 很小，开销可忽略）。
	if pretty, perr := common.IndentJson(manifestBytes); perr == nil {
		manifestBytes = pretty
	}
	if err := writeTarFile(tw, exportManifestName, manifestBytes, now()); err != nil {
		cleanupTmp()
		return 0, manifest, err
	}

	// 逐分片搬运。原样字节写入（不解压重压）。
	recordCount := 0
	for _, p := range parts {
		if err := ctx.Err(); err != nil {
			cleanupTmp()
			return 0, manifest, err
		}
		data, err := b.Reader.ReadFile(ctx, p.Path, maxBytes)
		if err != nil {
			cleanupTmp()
			return 0, manifest, fmt.Errorf("read part %s: %w", p.Path, err)
		}
		if err := writeTarFile(tw, p.Path, data, now()); err != nil {
			cleanupTmp()
			return 0, manifest, err
		}
		// 尽力统计记录数：仅对明文 .jsonl 直接数行；gzip 分片不解压
		// （解压会引入 CPU 热点且 manifest 只是元数据，非权威）。
		if strings.HasSuffix(p.Path, ".jsonl") && !strings.HasSuffix(p.Path, exportPartExt) {
			recordCount += countJSONLines(data)
		}
	}
	if err := tw.Close(); err != nil {
		cleanupTmp()
		return 0, manifest, err
	}
	if err := f.Sync(); err != nil {
		cleanupTmp()
		return 0, manifest, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, manifest, err
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, manifest, err
	}
	manifest.RecordCount = recordCount
	return counter.n, manifest, nil
}

// countingWriter 统计写入字节数。
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// writeTarFile 往 tar 写一个常规文件成员。
//
// mode 固定 0o644：归档是数据产物，不需要执行位，也不需要保留原始权限
// （云盘文件本就没有 POSIX 权限概念）。
func writeTarFile(tw *tar.Writer, name string, data []byte, modUnix int64) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Unix(modUnix, 0).UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// countJSONLines 统计非空行数（明文 JSONL 用）。
func countJSONLines(data []byte) int {
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// ErrExportNotFound 导出或产物不存在。
var ErrExportNotFound = errors.New("training export: not found")

// exportFormatNotes 返回给消费方看的诚实说明（单一真源，构造与测试共用）。
func exportFormatNotes() []string {
	return []string{
		"Each part file is a complete, independently decompressible gzip member containing JSONL (one JSON object per line).",
		"Each record is {\"role\":\"request\"|\"assistant\",\"content\":\"...\"}. Request records carry conversation text; assistant records carry the model's reply to the immediately preceding request record.",
		"Request records are NOT all the same shape. The `kind` field has three values: `full` = a **complete conversation snapshot** (RESET your accumulated text); `suffix` = only the **appended delta** since the previous snapshot (APPEND it); `message` = **one or more complete messages** produced by the structured-message path (APPEND with message boundaries, do NOT treat it as a snapshot and do NOT concatenate it without separators). Accumulating every request row blindly will duplicate the history N times.",
		"`kind` cannot be inferred from position: after the per-conversation watermark is cleaned up (inactive conversations), ANY part file may start with `kind=full`. Always read the field.",
		"Parts written **before** `kind` existed (older deployments) have request rows with no `kind`. For those legacy parts only, apply the pre-v2 rule: the first request row is a full snapshot and every later one is a suffix. Never mix this fallback with rows that do carry `kind`.",
		"Assistant records have no `kind` field and are always standalone complete replies.",
		"Request text is the gateway's concatenation of the whole conversation (roles and text interleaved) and is NOT reversible into per-turn roles; per-turn role recovery is heuristic only.",
		"Request text longer than the capture limit is stored head+tail with an inline marker `…[training-capture: N bytes elided]…`; detect and drop such samples before training.",
		"Assistant replies are captured from the response stream and are truncated at 256KB per response.",
		"Consumers MUST check the `format` field before parsing; unknown values should be rejected rather than guessed.",
	}
}

// DailyCaptureStat 某一天的采集规模（由云盘目录树聚合）。
type DailyCaptureStat struct {
	// Date 规范化日期 `YYYY-MM-DD`（与目录名一致）。
	Date string `json:"date"`
	// Users 该日期下有采集数据的用户数（二级目录数）。
	Users int `json:"users"`
	// Models 该日期下的「用户-模型」组合数（三级目录数之和）。
	Models int `json:"models"`
	// Parts 分片文件数。
	Parts int `json:"parts"`
	// Bytes 分片文件总字节数（**压缩后**，即云盘实际占用）。
	Bytes int64 `json:"bytes"`
}

// AggregateDailyStats 按日期聚合云盘目录树，得到「每天采了多少」。
//
// ## 为什么从云盘目录树算，而不是查库或读进程内计数
//
// 用户要的是「几月几日采集了多少」。进程内计数**重启即归零**，只反映今天；
// 而云盘目录树本身就是按日期分层的（一级目录就是日期），
// 是**唯一持久且真实**的记录 —— 它反映的是「确实落盘了什么」，
// 而不是「我们以为采了多少」。这也让管理员能把数字与云盘实际内容交叉核对。
//
// 口径：只统计符合本程序布局 `<日期>/<用户-ID>/<模型>/<分片>` 的**分片文件**；
// 非本程序布局的目录/文件一律忽略（同 `SelectExportParts` 的判据）。
//
// 返回按日期升序，便于前端直接展示趋势。
func AggregateDailyStats(entries []TreeEntry) []DailyCaptureStat {
	type acc struct {
		users map[string]bool
		// models 记 `<用户>/<模型>` 组合，避免两个用户同名模型被算成 1。
		models map[string]bool
		parts  int
		bytes  int64
	}
	byDate := map[string]*acc{}

	for _, e := range entries {
		if e.IsDir {
			continue
		}
		if !strings.HasSuffix(e.Path, exportPartExt) && !strings.HasSuffix(e.Path, ".jsonl") {
			continue
		}
		segments := strings.Split(strings.Trim(e.Path, "/"), "/")
		if len(segments) < 4 {
			// 期望 `<日期>/<用户>/<模型>/<文件>`；不满足的不是本程序布局。
			continue
		}
		date := segments[0]
		// 必须规范化（补零）：目录树里的历史/手工目录可能写成 `2026-10-1`，
		// 直接当日期会与 `2026-10-01` 分成两天。
		if !IsCanonicalDate(date) {
			continue
		}
		user := segments[1]
		model := segments[2]
		a := byDate[date]
		if a == nil {
			a = &acc{users: map[string]bool{}, models: map[string]bool{}}
			byDate[date] = a
		}
		a.users[user] = true
		a.models[user+"/"+model] = true
		a.parts++
		a.bytes += e.SizeBytes
	}

	out := make([]DailyCaptureStat, 0, len(byDate))
	for date, a := range byDate {
		out = append(out, DailyCaptureStat{
			Date:   date,
			Users:  len(a.users),
			Models: len(a.models),
			Parts:  a.parts,
			Bytes:  a.bytes,
		})
	}
	// 日期字符串是补零的 `YYYY-MM-DD`，字典序 == 时间序。
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}
