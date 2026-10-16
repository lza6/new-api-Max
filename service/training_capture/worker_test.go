package training_capture

import (
	"context"
	"fmt"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/stretchr/testify/assert"
	"strings"
	"sync"
	"testing"
	"time"
)

// 内存后端：让 worker 的核心逻辑（增量 → 目录 → 分片 → 上传）可测，
// 而不依赖真实云盘。**只用于测试**，生产走 OneDrive（见 onedrive.go）。
//
// 它同时充当「契约测试」的载体：OneDrive 必须满足同一组语义
// （幂等建目录、目录不存在时 List 返回空、超限报错等），
// 因为 worker 对两者一视同仁。
type memSink struct {
	mu sync.Mutex
	// files: 完整路径 → 内容
	files map[string][]byte
	// dirs: 显式创建过的目录集合
	dirs map[string]bool

	// 故障注入
	failWrite bool
	failList  bool
	failDir   bool

	// 调用记录（用于断言「没有多余调用」）
	writeCalls int
	listCalls  int
	dirCalls   int

	// maxWritePart 本 sink 自报的单次写入上限（0 → 用 defaultMaxWritePartBytes）。
	// 可调小以在测试里触发「压缩后超限 → 重切」路径，而不必造几十 MB 数据。
	maxWritePart int
}

// MaxWritePartBytes 实现 PartUploadLimiter：让测试 sink 像真实后端一样
// 声明自己的上限，worker 才会按它切分。
func (m *memSink) MaxWritePartBytes() int {
	if m.maxWritePart > 0 {
		return m.maxWritePart
	}
	return defaultMaxWritePartBytes
}

func newMemSink() *memSink {
	return &memSink{files: map[string][]byte{}, dirs: map[string]bool{}}
}

func (m *memSink) Name() string { return "mem" }

func (m *memSink) EnsureDir(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirCalls++
	if m.failDir {
		return fmt.Errorf("mem sink: injected EnsureDir failure")
	}
	// 幂等：重复调用返回 nil。
	m.dirs[path] = true
	return nil
}

func (m *memSink) WritePart(_ context.Context, path string, data []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeCalls++
	if m.failWrite {
		return "", fmt.Errorf("mem sink: injected WritePart failure")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("mem sink: empty payload")
	}
	// 与真实后端一致：超过**自报上限**必须报错，不能截断。
	// 用 MaxWritePartBytes() 而不是 OneDrive 常量：本 sink 通过
	// memSinkMaxWritePartBytes 声明自己的上限，测试才能覆盖
	// 「worker 是否真的按后端声明的上限切分」。
	if maxWrite := m.MaxWritePartBytes(); maxWrite > 0 && len(data) > maxWrite {
		return "", fmt.Errorf("mem sink: %d bytes exceeds declared write limit %d", len(data), maxWrite)
	}
	m.files[path] = append([]byte(nil), data...)
	return path, nil
}

func (m *memSink) List(_ context.Context, path string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listCalls++
	if m.failList {
		return nil, fmt.Errorf("mem sink: injected List failure")
	}
	// 与 OneDrive 一致：目录不存在 → 空切片而非错误。
	prefix := path + "/"
	var names []string
	for full := range m.files {
		if strings.HasPrefix(full, prefix) {
			rest := strings.TrimPrefix(full, prefix)
			// 只返回直接子项（不含子目录内容）。
			if !strings.Contains(rest, "/") {
				names = append(names, rest)
			}
		}
	}
	return names, nil
}

func (m *memSink) Stat(context.Context) (SinkStat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return SinkStat{
		TotalBytes: 1 << 40, UsedBytes: int64(len(m.files)), RemainingBytes: 1 << 40,
		FolderCount: len(m.dirs), FileCount: len(m.files), Provider: "mem",
	}, nil
}

func (m *memSink) snapshot() map[string][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string][]byte, len(m.files))
	for k, v := range m.files {
		out[k] = append([]byte(nil), v...)
	}
	return out
}

// 汇总所有分片的 JSONL 行数（用于断言「到底传了多少条」）。
//
// 分片现在是 **gzip 压缩**的（见 compression.go），统计行数前必须先解压
// ——直接数压缩字节里的 '\n' 是无意义的。按后缀分流：.gz 解压后统计，
// 其它（历史明文 fixture）直接统计。
//
// 必须持锁：worker goroutine 在并发写 files，不持锁的 range map
// 是数据竞争（-race 下直接报错）。
func (m *memSink) totalLines() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for name, data := range m.files {
		if strings.HasSuffix(name, ".gz") {
			plain, err := gzipDecompress(data, 1<<30)
			if err != nil {
				// 解压失败说明写入的不是合法 gzip —— 这本身就是缺陷，
				// 不能静默跳过（否则测试对坏数据视而不见）。
				panic("memSink: part is not valid gzip: " + err.Error())
			}
			data = plain
		}
		total += strings.Count(string(data), "\n")
	}
	return total
}

// writeCallsCount 持锁读取写入次数。
func (m *memSink) writeCallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeCalls
}

// dirCallsCount 持锁读取建目录次数。
func (m *memSink) dirCallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dirCalls
}

// ---------- 测试装置 ----------

func signal(userId int, username, model, convId string, messages []Message, reply string) CaptureSignal {
	return CaptureSignal{
		UserID: userId, Username: username, Model: model,
		ConversationID: convId, Messages: messages, ResponseText: reply,
		CreatedAt: 1791937800, // 2026-10-14 00:30 UTC（与 DateDirName 测试一致）
	}
}

// withMemSink 安装内存后端并返回清理函数。
//
// **同时初始化测试 DB**：processSignal 必然读水位（`GetTrainingDataWatermark`），
// 没有 DB 会 nil deref。把两者绑在一起，避免每加一个用例就要记得配 DB
// ——那类遗漏会让测试在无关的地方崩，掩盖真正要验证的行为。
func withMemSink(t *testing.T) *memSink {
	t.Helper()
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	sink := newMemSink()
	restore := SetSinkForTest(sink)
	t.Cleanup(restore)
	return sink
}

// ---------- 目录结构（用户点名：日期/用户名/模型） ----------

func TestProcessSignalCreatesDateUserModelLayout(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)

	sig := signal(7, "alice", "gpt-4o", "conv-1",
		[]Message{{Role: "user", Content: "你好"}}, "你也好")
	if _, _, err := processSignal(context.Background(), sig, limiter); err != nil {
		t.Fatalf("processSignal: %v", err)
	}

	files := sink.snapshot()
	if len(files) != 1 {
		t.Fatalf("应有且仅有 1 个分片，实际 %d 个：%v", len(files), keysOf(files))
	}
	// 三级目录：日期 / 用户名-用户ID / 模型。日期取 UTC。
	const want = "2026-10-14/alice-7/gpt-4o/part-0001.jsonl.gz"
	if _, ok := files[want]; !ok {
		t.Fatalf("目录结构不对，期望 %q，实际 %v", want, keysOf(files))
	}
}

func TestProcessSignalSanitizesDangerousUsernameInPath(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)

	// 用户名是用户可控输入，必须净化，否则可制造路径穿越。
	sig := signal(42, "../../etc/passwd", "m", "c",
		[]Message{{Role: "user", Content: "x"}}, "")
	if _, _, err := processSignal(context.Background(), sig, limiter); err != nil {
		t.Fatalf("processSignal: %v", err)
	}

	for path := range sink.snapshot() {
		if strings.Contains(path, "..") {
			t.Fatalf("路径出现了上跳片段：%q", path)
		}
		if strings.Count(path, "/") != 3 {
			t.Fatalf("目录层级被用户名破坏：%q", path)
		}
		// 仍必须带用户 ID 后缀以避免不同用户互相覆盖。
		if !strings.Contains(path, "-42/") {
			t.Fatalf("净化后的用户名必须带用户 ID 后缀：%q", path)
		}
	}
}

func TestProcessSignalRejectsInvalidInputWithoutTouchingSink(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)

	cases := []struct {
		name string
		sig  CaptureSignal
	}{
		{"零用户 ID", signal(0, "a", "m", "c", []Message{{Role: "user", Content: "x"}}, "")},
		{"负用户 ID", signal(-1, "a", "m", "c", []Message{{Role: "user", Content: "x"}}, "")},
		{"无消息", signal(1, "a", "m", "c", nil, "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, b, err := processSignal(context.Background(), tc.sig, limiter)
			if err != nil {
				t.Fatalf("无效输入不应报错（应静默跳过）：%v", err)
			}
			if n != 0 || b != 0 {
				t.Fatalf("无效输入不应上传任何内容，实际 n=%d b=%d", n, b)
			}
		})
	}
	// 无效输入不得产生任何云盘调用（否则会在云盘留下空目录垃圾）。
	if sink.dirCallsCount() != 0 || sink.writeCallsCount() != 0 {
		t.Fatalf("无效输入不应触达后端：EnsureDir=%d WritePart=%d", sink.dirCallsCount(), sink.writeCallsCount())
	}
}

// ---------- 增量上传（用户点名的核心场景） ----------

func TestProcessSignalUploadsOnlyIncrementOnRepeatedRequest(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()

	// 长期会话：先有一次完整历史。
	history := []Message{
		{Role: "user", Content: "第一轮问题"},
		{Role: "assistant", Content: "第一轮回答"},
	}
	sig := signal(101, "bob", "gpt-4o", "conv-long", history, "第一轮回答")
	if n, _, err := processSignal(ctx, sig, limiter); err != nil || n != 3 {
		t.Fatalf("首次应上传 3 条（2 历史 + 1 回复），实际 n=%d err=%v", n, err)
	}

	// 第二轮：历史原封不动，只多了一条 user + 一条 assistant。
	sig2 := signal(101, "bob", "gpt-4o", "conv-long",
		append(append([]Message{}, history...),
			Message{Role: "assistant", Content: "第一轮回答"},
			Message{Role: "user", Content: "第二轮问题"},
		), "第二轮回答")
	n2, _, err := processSignal(ctx, sig2, limiter)
	if err != nil {
		t.Fatalf("第二轮：%v", err)
	}
	if n2 != 2 {
		t.Fatalf("第二轮只应上传新增的 2 条，实际 %d 条", n2)
	}

	// 全部分片加起来应恰好 5 行（3 + 2），没有重复上传历史。
	// 这是「长期会话不重复传 token」的可观测证据。
	if got := sink.totalLines(); got != 5 {
		t.Fatalf("累计行数应为 5（无历史重传），实际 %d", got)
	}
}

func TestProcessSignalNoopWhenNothingNew(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()

	sig := signal(102, "carol", "m", "conv-x",
		[]Message{{Role: "user", Content: "同样的问题"}}, "同样的回答")
	if _, _, err := processSignal(ctx, sig, limiter); err != nil {
		t.Fatalf("首次：%v", err)
	}
	writesAfterFirst := sink.writeCallsCount()

	// 完全相同的请求再来一次 → 不应产生任何上传。
	n, b, err := processSignal(ctx, sig, limiter)
	if err != nil {
		t.Fatalf("重复：%v", err)
	}
	if n != 0 || b != 0 {
		t.Fatalf("无新增时不应上传，实际 n=%d b=%d", n, b)
	}
	if sink.writeCallsCount() != writesAfterFirst {
		t.Fatalf("无新增时不应调用 WritePart（实际多调用了 %d 次）", sink.writeCallsCount()-writesAfterFirst)
	}
	// 也不该白建目录（增量判定在 EnsureDir 之前）。
	// 这里只断言没有新增写入，目录调用是否发生不影响正确性。
}

func TestProcessSignalFallsBackToGlobalDedupWithoutConversationId(t *testing.T) {
	withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()

	// 先清空全局指纹集合，避免与其它用例互相影响。
	globalSeenFingerprints = NewBoundedFingerprintSet(FingerprintSetDefaultCapacity)

	sig := signal(103, "dave", "m", "",
		[]Message{{Role: "user", Content: "无会话 ID 的内容"}}, "")
	if n, _, err := processSignal(ctx, sig, limiter); err != nil || n != 1 {
		t.Fatalf("首次应上传 1 条，实际 n=%d err=%v", n, err)
	}
	// 同内容再次 → 全局去重跳过。
	n, _, err := processSignal(ctx, sig, limiter)
	if err != nil {
		t.Fatalf("第二次：%v", err)
	}
	if n != 0 {
		t.Fatalf("同内容在无会话 ID 时应被全局去重，实际上传 %d 条", n)
	}
}

// ---------- 分片（用户点名：文件不能无限增大） ----------

func TestEncodeJSONLIsOneJSONPerLine(t *testing.T) {
	payload := encodeJSONL([]Message{
		{Role: "user", Content: "第一行"},
		{Role: "assistant", Content: "含\"引号\"与\n换行的内容"},
	})
	lines := strings.Split(strings.TrimRight(string(payload), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("应有 2 行 JSONL，实际 %d 行：%q", len(lines), payload)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
			t.Fatalf("第 %d 行不是完整 JSON 对象：%q", i, line)
		}
	}
	// 换行必须被 JSON 转义（否则会破坏 JSONL 的行边界，整份数据集不可解析）。
	if strings.Count(string(payload), "\n") != 2 {
		t.Fatalf("内容里的换行必须被转义，实际 JSONL 换行数异常：%d", strings.Count(string(payload), "\n"))
	}
}

func TestSplitBySizeNeverCutsLineInHalf(t *testing.T) {
	// 构造每行 100 字节、共 50 行的 payload。
	var b strings.Builder
	for range 50 {
		b.WriteString(strings.Repeat("x", 99))
		b.WriteByte('\n')
	}
	payload := []byte(b.String())

	chunks := splitBySize(payload, 1000)
	if len(chunks) < 2 {
		t.Fatalf("应被切成多块，实际 %d 块", len(chunks))
	}
	for i, chunk := range chunks {
		// 每块必须要么以换行结尾，要么是最后一块。
		if i < len(chunks)-1 && chunk[len(chunk)-1] != '\n' {
			t.Fatalf("第 %d 块没有在行边界结束（会把 JSON 切成两半），尾部字节：%q",
				i, chunk[len(chunk)-4:])
		}
		if len(chunk) > 1000 {
			t.Fatalf("第 %d 块超过上限：%d 字节", i, len(chunk))
		}
	}
	// 重组必须逐字一致（不丢字节、不重复）。
	if got := string(concat(chunks)); got != string(payload) {
		t.Fatalf("重组后与原始 payload 不一致（长度 %d vs %d）", len(got), len(payload))
	}
}

func TestSplitBySizeHandlesOversizedSingleLine(t *testing.T) {
	// 单行就超限：必须能前进（硬切），不能死循环。
	payload := []byte(strings.Repeat("y", 2500) + "\n")
	chunks := splitBySize(payload, 1000)
	if len(chunks) == 0 {
		t.Fatal("超长单行也必须产出分块")
	}
	if got := string(concat(chunks)); got != string(payload) {
		t.Fatal("硬切后重组必须一致")
	}
}

func TestSplitBySizeBoundaries(t *testing.T) {
	if got := splitBySize(nil, 100); got != nil {
		t.Fatalf("空输入应返回 nil，实际 %v", got)
	}
	small := []byte("a\nb\n")
	if got := splitBySize(small, 100); len(got) != 1 || string(got[0]) != string(small) {
		t.Fatalf("未超限时不应切分，实际 %v", got)
	}
	if got := splitBySize(small, 0); len(got) != 1 {
		t.Fatalf("maxBytes<=0 时不应切分，实际 %d 块", len(got))
	}
}

func TestProcessSignalOpensNewPartWhenExistingPartsPresent(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()

	// 预置已存在的分片，模拟「进程重启」。
	dir := "2026-10-14/eve-104/m"
	if err := sink.EnsureDir(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := sink.WritePart(ctx, dir+"/part-0001.jsonl.gz", []byte("{\"role\":\"user\",\"content\":\"旧\"}\n")); err != nil {
		t.Fatal(err)
	}

	sig := signal(104, "eve", "m", "conv-restart",
		[]Message{{Role: "user", Content: "重启后的新内容"}}, "")
	if _, _, err := processSignal(ctx, sig, limiter); err != nil {
		t.Fatalf("processSignal: %v", err)
	}

	files := sink.snapshot()
	// **重启后绝不能覆盖已上传分片**（不可逆的数据破坏）。
	if len(files[dir+"/part-0001.jsonl.gz"]) == 0 ||
		!strings.Contains(string(files[dir+"/part-0001.jsonl.gz"]), "旧") {
		t.Fatal("已上传的 part-0001 被覆盖了——重启会破坏历史数据")
	}
	if _, ok := files[dir+"/part-0002.jsonl.gz"]; !ok {
		t.Fatalf("应在下一个序号开新分片，实际分片：%v", keysOf(files))
	}
}

// ---------- 失败路径：错就明确错，绝不假装成功 ----------

func TestProcessSignalSurfacesSinkErrors(t *testing.T) {
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()
	sig := signal(105, "frank", "m", "conv-err",
		[]Message{{Role: "user", Content: "x"}}, "")

	stages := []struct {
		name  string
		setup func(*memSink)
	}{
		{"EnsureDir 失败", func(m *memSink) { m.failDir = true }},
		{"WritePart 失败", func(m *memSink) { m.failWrite = true }},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			sink := withMemSink(t)
			stage.setup(sink)
			if _, _, err := processSignal(ctx, sig, limiter); err == nil {
				t.Fatal("后端失败必须向上报错，不能静默成功")
			}
		})
	}
}

func TestProcessSignalDoesNotAdvanceWatermarkWhenUploadFails(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()
	const userId, convId = 106, "conv-atomic"

	sig := signal(userId, "grace", "m", convId,
		[]Message{{Role: "user", Content: "重要内容"}}, "")

	// 先让上传失败。
	sink.failWrite = true
	if _, _, err := processSignal(ctx, sig, limiter); err == nil {
		t.Fatal("上传失败时应报错")
	}

	// 水位**不得**推进——否则这条内容永远不会再被采集（静默丢数据）。
	wm, err := loadWatermarkForTest(userId, convId)
	if err != nil {
		t.Fatalf("读水位：%v", err)
	}
	if wm != nil && wm.UploadedCount != 0 {
		t.Fatalf("上传失败后水位被推进到 %d——这条内容将永久丢失", wm.UploadedCount)
	}

	// 恢复后重试：内容必须能补上。
	sink.failWrite = false
	n, _, err := processSignal(ctx, sig, limiter)
	if err != nil {
		t.Fatalf("重试：%v", err)
	}
	if n != 1 {
		t.Fatalf("失败后重试应补传 1 条，实际 %d 条", n)
	}
}

func TestProcessSignalAdvancesWatermarkAfterSuccess(t *testing.T) {
	withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()
	const userId, convId = 107, "conv-wm"

	sig := signal(userId, "heidi", "m", convId,
		[]Message{{Role: "user", Content: "a"}, {Role: "assistant", Content: "b"}}, "")
	if _, _, err := processSignal(ctx, sig, limiter); err != nil {
		t.Fatalf("processSignal: %v", err)
	}

	wm, err := loadWatermarkForTest(userId, convId)
	if err != nil {
		t.Fatalf("读水位：%v", err)
	}
	if wm == nil {
		t.Fatal("成功上传后必须写入水位，否则下次会整段重传")
	}
	if wm.UploadedCount != 2 {
		t.Fatalf("水位应推进到 2，实际 %d", wm.UploadedCount)
	}
	if wm.PrefixFingerprint == "" {
		t.Fatal("水位必须带前缀指纹，否则中间编辑无法被察觉（B3 回归）")
	}
}

// ---------- 限速（用户点名：不能影响用户请求） ----------

func TestByteLimiterDoesNotWaitWithinBurst(t *testing.T) {
	limiter := NewByteLimiter(1<<20, nil) // 1MB/s，初始桶 1MB
	start := time.Now()
	limiter.wait(context.Background(), 1<<19) // 512KB，在桶内
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("桶内额度不应等待，实际等了 %v", elapsed)
	}
}

func TestByteLimiterWaitsWhenOverBudget(t *testing.T) {
	limiter := NewByteLimiter(100_000, nil) // 100KB/s
	ctx := context.Background()
	limiter.wait(ctx, 100_000) // 用光桶
	start := time.Now()
	limiter.wait(ctx, 50_000) // 需要 0.5s
	elapsed := time.Since(start)
	if elapsed < 300*time.Millisecond {
		t.Fatalf("超额度必须等待，实际只等了 %v", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("等待时间明显超预期：%v", elapsed)
	}
}

func TestByteLimiterRespectsContextCancellation(t *testing.T) {
	limiter := NewByteLimiter(1000, nil) // 极慢；不绑停止信号
	ctx, cancel := context.WithCancel(context.Background())
	limiter.wait(ctx, 1000)
	cancel()
	start := time.Now()
	limiter.wait(ctx, 500_000) // 在正常速率下要等 500s
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("ctx 取消后必须立刻返回（否则停机被卡住），实际等了 %v", elapsed)
	}
}

func TestByteLimiterRespectsStopSignal(t *testing.T) {
	// 停机信号必须能打断限速等待，否则 StopWorker 会卡在一次限速上。
	stop := make(chan struct{})
	limiter := NewByteLimiter(1000, stop)
	limiter.wait(context.Background(), 1000) // 用光桶

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(stop)
	}()

	start := time.Now()
	limiter.wait(context.Background(), 500_000) // 正常要等 500s
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("停机信号必须立刻中断限速等待，实际等了 %v", elapsed)
	}
}

func TestByteLimiterDoesNotDependOnGlobalWorkerStop(t *testing.T) {
	// 回归：原实现直接读包级 workerStop。测试里前一个用例关掉它之后，
	// 后续用例的限速等待会**立即返回**（该等 0.5s 却等了 0s）。
	// 这里刻意先关掉全局 workerStop，再验证限速仍然生效。
	originalStop := workerStop
	workerStop = make(chan struct{})
	close(workerStop)
	defer func() { workerStop = originalStop }()

	limiter := NewByteLimiter(100_000, nil) // 不绑任何停止信号
	ctx := context.Background()
	limiter.wait(ctx, 100_000) // 用光桶
	start := time.Now()
	limiter.wait(ctx, 50_000) // 需要 0.5s
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("限速被全局 workerStop 影响而失效，实际只等了 %v", elapsed)
	}
}

func TestByteLimiterNilAndZeroAreSafe(t *testing.T) {
	var nilLimiter *byteLimiter
	nilLimiter.wait(context.Background(), 100) // 不得 panic
	newByteLimiter(0).wait(context.Background(), 100)
	newByteLimiter(-5).wait(context.Background(), 100)
	NewByteLimiter(1000, nil).wait(context.Background(), 0)
}

// ---------- 队列与 MaybeCapture 的契约 ----------

func TestMaybeCaptureReturnsFalseForInvalidSignal(t *testing.T) {
	// 用户 ID 非法 → 直接拒绝，不查授权（省掉一次查询）。
	if MaybeCapture(CaptureSignal{UserID: 0, Messages: []Message{{Role: "user", Content: "x"}}}) {
		t.Fatal("用户 ID 为 0 时不应入队")
	}
	if MaybeCapture(CaptureSignal{UserID: 1}) {
		t.Fatal("无消息时不应入队")
	}
}

// ---------- 统计口径 ----------

func TestRolloverDayResetsTodayCounters(t *testing.T) {
	// 直接把 Day 设为昨天，然后触发跨日。
	yesterday := "2000-01-01"
	captureStats.Day.Store(&yesterday)
	captureStats.Users.Store(999)
	captureStats.Conversations.Store(888)
	captureStats.BytesUploaded.Store(777)
	captureStats.MessagesUploaded.Store(666)

	rolloverDayIfNeeded(time.Unix(1791937800, 0)) // 2026-10-14

	if *captureStats.Day.Load() != "2026-10-14" {
		t.Fatalf("日期未切换：%s", *captureStats.Day.Load())
	}
	snap := GetCaptureStats()
	for name, v := range map[string]int64{
		"Users": snap.Users, "Conversations": snap.Conversations,
		"BytesUploaded": snap.BytesUploaded, "MessagesUploaded": snap.MessagesUploaded,
	} {
		if v != 0 {
			t.Fatalf("跨日后 %s 应清零，实际 %d", name, v)
		}
	}
}

func TestRolloverDayIsNoopWithinSameDay(t *testing.T) {
	today := timeUTCDate(1791937800)
	captureStats.Day.Store(&today)
	captureStats.Users.Store(5)

	rolloverDayIfNeeded(time.Unix(1791937900, 0)) // 同一天稍后

	if got := captureStats.Users.Load(); got != 5 {
		t.Fatalf("同一天内不应清计数，实际 %d", got)
	}
}

func TestMarkUserTodayCountsDistinctUsers(t *testing.T) {
	today := timeUTCDate(1791937800)
	captureStats.Day.Store(&today)
	captureStats.usersTodayMu.Lock()
	captureStats.usersToday = make(map[int]struct{})
	captureStats.usersTodayMu.Unlock()
	captureStats.Users.Store(0)

	markUserToday(1)
	markUserToday(1)
	markUserToday(2)
	markUserToday(3)
	markUserToday(2)

	if got := captureStats.Users.Load(); got != 3 {
		t.Fatalf("应统计 3 个不同用户，实际 %d", got)
	}
}

// ---------- 工具 ----------

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func concat(chunks [][]byte) []byte {
	var out []byte
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}

// 回归：**大 payload 切分后，每个分片必须是可独立解压的完整 gzip**。
//
// 这条测试的缺失曾经让一个数据损坏级 bug 逃过整套测试：
// 原实现先把整个 payload 压成**一个** gzip member，再把这个 member 的
// 字节按大小切开 —— 切出来的是 gzip **碎片**，解压报
// `unexpected EOF` / `invalid header`。而且不是报错而是**静默乱码**：
// 预览按 magic 判断失败后当明文显示二进制，分析端把碎片当明文喂给模型。
//
// 触发条件：压缩后超过单次上传上限。单条文本上限在内存大的机器上
// 会自适应到 8MB，不可压缩内容（代码/随机串）压缩后仍接近 8MB → 必然触发。
func TestProcessSignalEveryPartIsIndependentlyDecompressible(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()

	// 把单分片原始上限压到 1MB，让下面的体量稳定切出多片。
	// （不变量保证它 ≥ 单条文本上限 × 8，所以单条记录永远不会被切开。）
	//
	// **必须先复位预算**：别的用例可能调用过 `initResourceBudget`，
	// 把包级预算设成了大机器的值（maxPart=16MB），那样本用例的
	// 3.7MB 内容会塞进 1 个分片、**测不到多分片路径**。
	// 这是真实的测试隔离问题（我第一版就是这么假过的）。
	ResetBudgetForTest()
	t.Setenv(EnvCaptureMaxPartMB, "1")
	t.Cleanup(ResetBudgetForTest)

	// **多轮对话**形态：很多条记录合计超过单分片上限。
	// 这正是真实「长期会话」的样子 —— 也正因如此，多分片是常态而非边缘。
	//
	// 内容刻意用**不可压缩**的十六进制：压缩率接近 1，
	// 保证能跨过分片阈值（若用重复文本，压缩后可能只剩几十 KB）。
	var msgs []Message
	perMsg := 4096
	for i := range 900 {
		var b strings.Builder
		for j := 0; b.Len() < perMsg; j++ {
			fmt.Fprintf(&b, "%08x%08x%08x\n", i*131+j, i*7919+j, i*2654435761+j)
		}
		msgs = append(msgs, Message{Role: "user", Content: b.String()})
	}

	sig := signal(901, "biguser", "m", "conv-big", msgs, "")
	n, _, err := processSignal(ctx, sig, limiter)
	if err != nil {
		t.Fatalf("processSignal：%v", err)
	}
	if n != len(msgs) {
		t.Fatalf("应上传 %d 条消息，实际 %d", len(msgs), n)
	}

	files := sink.snapshot()
	if len(files) < 2 {
		t.Fatalf("这么大的内容应被切成多个分片，实际 %d 个 —— "+
			"若只有 1 个，说明本用例没覆盖到多分片路径，需调整体量", len(files))
	}

	// **核心断言 1**：每个分片都必须能**独立解压**。
	// 「先整体压缩再按字节切」会让这里报 unexpected EOF / invalid header。
	parts := make(map[string]string, len(files))
	for name, data := range files {
		if !strings.HasSuffix(name, ".jsonl.gz") {
			t.Fatalf("分片名不符合契约：%s", name)
		}
		plain, derr := gzipDecompress(data, 64<<20)
		if derr != nil {
			t.Fatalf("分片 %s 不是合法的完整 gzip（%v）—— "+
				"这正是「先压缩再按字节切」会造成的碎片化损坏", name, derr)
		}
		parts[name] = string(plain)
	}

	// **核心断言 2**：每个分片里的每一行都必须是**完整 JSON**，
	// 且解码出的内容要能对上原文（逐字无损）。
	// 单条记录被切成两半时这里会失败（JSONL 数据集就此不可解析）。
	//
	// 注意：必须**解码后**比对内容，不能拿原文去 strings.Contains 编码后的
	// payload —— JSON 会把换行转义成 `\n` 两个字面字符。
	decodedCount := map[string]int{}
	var totalRecords int
	for name, plain := range parts {
		for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
			if line == "" {
				continue
			}
			var rec struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			if err := common.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("分片 %s 里有被切碎的记录（不是完整 JSON）：%v\n行首：%q",
					name, err, firstN(line, 60))
			}
			if rec.Role == "" || rec.Content == "" {
				t.Fatalf("分片 %s 里的记录缺字段：role=%q content 长度=%d",
					name, rec.Role, len(rec.Content))
			}
			decodedCount[rec.Content]++
			totalRecords++
		}
	}
	if totalRecords != len(msgs) {
		t.Fatalf("还原出的记录数 %d != 上传的 %d —— 有记录在切分/压缩中丢失",
			totalRecords, len(msgs))
	}
	// 逐条比对：上传的每条内容都必须**恰好出现一次**。
	for i, m := range msgs {
		if decodedCount[m.Content] != 1 {
			t.Fatalf("第 %d 条记录还原后出现 %d 次（应为 1）—— 压缩/切分链路有损或有重复",
				i, decodedCount[m.Content])
		}
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// 回归（盲点 F10）：worker 启停必须可以**任意次往返**。
//
// 原实现用 `workerOnce.Do` + 关闭 stop channel：
// 「管理员关开关 → 再打开」后第二次 `StartWorker` 是 no-op，
// 而 stop channel 已关闭 → **没有消费者，采集永久停摆**，
// 而管理端仍显示 enabled（典型的「看起来在工作其实没工作」）。
//
// 这个缺陷长期未被发现，是因为测试都**手工重置内部状态**
// （`workerOnce = sync.Once{}` + `go runWorker()`），绕过了真实启停路径。
func TestWorkerCanRestartAfterStop(t *testing.T) {
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	originalQueue := captureQueue
	captureQueue = NewCaptureQueue(64)
	t.Cleanup(func() { captureQueue = originalQueue })
	t.Cleanup(waitWorkerStopped)

	// 第一轮：启 → 停。
	if !StartWorker() {
		t.Fatal("首次 StartWorker 应返回 true")
	}
	if !WorkerRunning() {
		t.Fatal("启动后 WorkerRunning 应为 true")
	}
	if StartWorker() {
		t.Fatal("重复 StartWorker 应是 no-op（返回 false）")
	}
	if !StopWorker() {
		t.Fatal("StopWorker 应返回 true（确实停了一个）")
	}
	waitWorkerStopped()
	if WorkerRunning() {
		t.Fatal("停止后 WorkerRunning 应为 false")
	}

	// **第二轮：再启** —— 这是原实现失效的地方。
	if !StartWorker() {
		t.Fatal("停止后再次 StartWorker 必须能真正启动（原实现在这里是 no-op）")
	}
	if !WorkerRunning() {
		t.Fatal("重启后 WorkerRunning 应为 true —— 否则采集静默停摆")
	}

	// 更进一步：重启后的 worker 必须**真的在消费队列**。
	// 只断言 WorkerRunning 不足以证明「有消费者」。
	sink := withMemSink(t)
	restoreGateway := SetGatewayEnabledForTest(true)
	t.Cleanup(restoreGateway)
	const uid = 91001
	grantForTest(t, uid)

	sig := BuildSignal(CaptureInput{
		UserId: uid, Username: "restart", Model: "m", ConversationID: "conv-restart",
		Meta:      &types.TokenCountMeta{CombineText: "重启之后必须还能采集"},
		CreatedAt: 1791937800,
	})
	if sig == nil {
		t.Fatal("BuildSignal 返回 nil")
	}
	if !MaybeCapture(*sig) {
		t.Fatal("重启后 MaybeCapture 应能入队")
	}
	if !waitFor(t, 10*time.Second, func() bool { return sink.writeCallsCount() > 0 }) {
		t.Fatal("重启后的 worker **没有消费队列** —— 这正是原缺陷的表现")
	}
}

// 回归：一条坏数据 panic 后，worker 必须**继续处理后续信号**，
// 而不是整个停摆。
func TestWorkerSurvivesPanicInOneSignal(t *testing.T) {
	if err := ensureTestDB(t); err != nil {
		t.Fatalf("初始化测试 DB 失败：%v", err)
	}
	// 让第 1 条信号 panic、之后的正常。
	sink := &panicOnceSink{}
	restoreSink := SetSinkForTest(sink)
	t.Cleanup(restoreSink)

	originalQueue := captureQueue
	captureQueue = NewCaptureQueue(64)
	t.Cleanup(func() { captureQueue = originalQueue })

	restoreGateway := SetGatewayEnabledForTest(true)
	t.Cleanup(restoreGateway)
	if !StartWorker() {
		t.Fatal("StartWorker 应启动")
	}
	t.Cleanup(waitWorkerStopped)

	const uid = 91002
	grantForTest(t, uid)
	meta := &types.TokenCountMeta{CombineText: "一条会触发 panic 的数据"}

	for i := range 2 {
		sig := BuildSignal(CaptureInput{
			UserId: uid, Username: "panic", Model: "m",
			ConversationID: "conv-panic-" + itoaSmall(i),
			Meta:           meta, CreatedAt: 1791937800,
		})
		if sig == nil || !MaybeCapture(*sig) {
			t.Fatalf("第 %d 条应能入队", i)
		}
	}

	// 第 2 条（panic 之后的那条）必须被成功处理。
	if !waitFor(t, 10*time.Second, func() bool { return sink.successCount() > 0 }) {
		t.Fatal("panic 之后 worker 未继续处理后续信号 —— 采集因一条坏数据永久停摆")
	}
	if !WorkerRunning() {
		t.Fatal("单条 panic 不应让 worker 退出")
	}
}

// panicOnceSink 第一次写入时 panic，之后正常。
type panicOnceSink struct {
	mu       sync.Mutex
	written  int
	panicked bool
}

func (p *panicOnceSink) Name() string { return "panic-once" }
func (p *panicOnceSink) EnsureDir(context.Context, string) error {
	return nil
}

func (p *panicOnceSink) WritePart(context.Context, string, []byte) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.panicked {
		p.panicked = true
		panic("injected panic from sink")
	}
	p.written++
	return "ref", nil
}

func (p *panicOnceSink) List(context.Context, string) ([]string, error) { return nil, nil }
func (p *panicOnceSink) Stat(context.Context) (SinkStat, error) {
	return SinkStat{Provider: "panic-once"}, nil
}

func (p *panicOnceSink) successCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.written
}

// 回归（审查 #6）：**生产唯一形态**下的助手回复追加分支。
//
// 生产路径是「1 条 role=request + ResponseText」（见 hotpath.go），
// 走 `useTextIncrement && reply != ""` 那条分支。此前**没有任何用例覆盖它**：
//   - worker_test 的 reply 用例传的是 Role:"user"（2 条）→ 走**消息级**路径；
//   - e2e_test 走 BuildSignal 但 ResponseText 恒空。
//
// 也就是说：这条分支若坏了，全套测试仍然全绿。
func TestProcessSignalAppendsReplyInProductionShape(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()

	// 用真实生产形态：单条 request + 回复。
	build := func(text, reply string) CaptureSignal {
		sig := BuildSignal(CaptureInput{
			UserId: 92001, Username: "prod", Model: "m",
			ConversationID: "conv-prod-shape",
			Meta:           &types.TokenCountMeta{CombineText: text},
			ResponseText:   reply,
			CreatedAt:      1791937800,
		})
		if sig == nil {
			t.Fatal("BuildSignal 返回 nil")
		}
		return *sig
	}

	// 第一次：全量请求 + 回复。
	sig1 := build("第一轮提问", "第一轮回答")
	n1, _, err := processSignal(ctx, sig1, limiter)
	if err != nil {
		t.Fatalf("首次：%v", err)
	}
	if n1 != 2 {
		t.Fatalf("首次应上传 2 条（请求 + 回复），实际 %d —— "+
			"若为 1 说明回复没被追加（生产形态的回复会丢）", n1)
	}
	if got := sink.totalLines(); got != 2 {
		t.Fatalf("云盘上应有 2 行，实际 %d", got)
	}

	// 第二次：请求文本增长 → 只传新增请求文本 + 该轮回复。
	sig2 := build("第一轮提问\n第一轮回答\n第二轮提问", "第二轮回答")
	n2, _, err := processSignal(ctx, sig2, limiter)
	if err != nil {
		t.Fatalf("第二次：%v", err)
	}
	if n2 != 2 {
		t.Fatalf("第二次应为「新增请求 + 回复」= 2 条，实际 %d", n2)
	}
	if got := sink.totalLines(); got != 4 {
		t.Fatalf("累计应有 4 行（2 + 2），实际 %d —— 回复被重复或漏传", got)
	}

	// 第三次：请求**完全没变** → 不传任何东西（含回复）。
	// 这防止「同一条回复每轮被反复上传」。
	before := sink.totalLines()
	n3, _, err := processSignal(ctx, sig2, limiter)
	if err != nil {
		t.Fatalf("第三次：%v", err)
	}
	if n3 != 0 {
		t.Fatalf("请求未推进时不应上传任何内容，实际 %d 条", n3)
	}
	if after := sink.totalLines(); after != before {
		t.Fatalf("请求未推进时云盘行数不应变化：%d → %d", before, after)
	}
}

// 回复必须携带 role=assistant，且**不带** kind 字段
// （kind 只对请求行有意义；给回复加 kind 会让消费方误判）。
func TestReplyRecordShapeIsCorrect(t *testing.T) {
	sink := withMemSink(t)
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)

	sig := BuildSignal(CaptureInput{
		UserId: 92002, Username: "shape", Model: "m", ConversationID: "conv-shape",
		Meta:         &types.TokenCountMeta{CombineText: "提问"},
		ResponseText: "回答",
		CreatedAt:    1791937800,
	})
	if _, _, err := processSignal(context.Background(), *sig, limiter); err != nil {
		t.Fatalf("processSignal：%v", err)
	}

	var sawRequest, sawAssistant bool
	for name, data := range sink.snapshot() {
		// 分片是 gzip 压缩的，解析前必须先解压（否则会读到二进制垃圾）。
		if strings.HasSuffix(name, ".gz") {
			plain, err := gzipDecompress(data, 1<<30)
			if err != nil {
				t.Fatalf("分片 %s 不是合法 gzip：%v", name, err)
			}
			data = plain
		}
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if line == "" {
				continue
			}
			var rec map[string]any
			if err := common.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("不是合法 JSON：%v", err)
			}
			role, _ := rec["role"].(string)
			switch role {
			case "request":
				sawRequest = true
				if rec["kind"] != KindRequestFull {
					t.Fatalf("首次采集的请求行 kind 应为 %q，实际 %v", KindRequestFull, rec["kind"])
				}
			case "assistant":
				sawAssistant = true
				if _, has := rec["kind"]; has {
					t.Fatal("回复行**不应**带 kind 字段（kind 只对请求行有意义）")
				}
			}
		}
	}
	if !sawRequest || !sawAssistant {
		t.Fatalf("应同时产出 request 与 assistant 行：request=%v assistant=%v", sawRequest, sawAssistant)
	}
}

// TestProcessSignalResplitBranchProducesValidGzip 覆盖**兜底重切**分支。
//
// 场景：内容**压缩后仍超过后端自报的写入上限**（已压缩内容/随机数据）。
// 旧实现在这里直接切**压缩产物** —— 碎片不是合法 gzip，云盘上出现
// 二进制垃圾。主路径修了、这个分支没修，等于把同一个 bug 留在
// 最难触发（只有高压缩比内容才进来）也最难排查的地方。
//
// 做法：用 memSink **自报一个小上限**（128KB），让 1MB 分片必然走进重切，
// 而不必造几十 MB 数据。这也顺带证明 worker 真的读了后端声明的上限。
func TestProcessSignalResplitBranchProducesValidGzip(t *testing.T) {
	sink := withMemSink(t)
	sink.maxWritePart = 128 << 10 // 自报 128KB 上限，强制触发重切
	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()

	ResetBudgetForTest()
	t.Setenv(EnvCaptureMaxPartMB, "1")
	t.Cleanup(ResetBudgetForTest)

	// 不可压缩内容：压缩比≈1 → 1MB 原始压出来仍 ~1MB ≫ 128KB → 必走重切。
	var msgs []Message
	for i := range 200 {
		var b strings.Builder
		for j := 0; b.Len() < 4096; j++ {
			fmt.Fprintf(&b, "%08x%08x%08x\n", i*131+j, i*7919+j, i*2654435761+j)
		}
		msgs = append(msgs, Message{Role: "user", Content: b.String()})
	}

	n, _, err := processSignal(ctx, signal(902, "resplit", "m", "conv-resplit", msgs, ""), limiter)
	if err != nil {
		t.Fatalf("processSignal：%v", err)
	}
	if n != len(msgs) {
		t.Fatalf("应上传 %d 条，实际 %d", len(msgs), n)
	}

	files := sink.snapshot()
	if len(files) < 2 {
		t.Fatalf("128KB 上限下必然多分片，实际 %d 个", len(files))
	}
	for name, data := range files {
		assert.LessOrEqual(t, len(data), sink.maxWritePart,
			"分片 %s 超过后端自报上限 —— worker 没按后端声明的上限切分", name)
		plain, derr := gzipDecompress(data, 64<<20)
		if derr != nil {
			t.Fatalf("重切产生的分片 %s 不是合法 gzip（%v）—— "+
				"这正是「切压缩产物」会造成的碎片化损坏", name, derr)
		}
		for _, line := range strings.Split(strings.TrimRight(string(plain), "\n"), "\n") {
			if line == "" {
				continue
			}
			var rec struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			if err := common.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("分片 %s 里有被切碎的记录：%v", name, err)
			}
		}
	}
}
