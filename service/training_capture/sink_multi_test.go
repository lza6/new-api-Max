package training_capture

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 多账号后端（负载均衡）的**不变量**测试。
//
// 最要命的一条：**同一目录的分片必须永远落在同一个账号**。
// 分片序号按目录递增（part-0001、part-0002…），一旦同一目录被劈到两个账号，
// 两边都从 0001 开始 → 同名不同内容 → 导出/分析出现重复或覆盖（不可逆）。

// recSink 记录写入的假后端（同时实现 SinkReader 与可选能力）。
type recSink struct {
	name string

	mu      sync.Mutex
	files   map[string][]byte // path -> content
	dirs    map[string]bool
	listErr error

	// 可选能力开关
	supportTruncation bool
	truncated         bool
	countFolders      int
	countFiles        int
	maxWrite          int
}

func newRecSink(name string) *recSink {
	return &recSink{
		name:              name,
		files:             map[string][]byte{},
		dirs:              map[string]bool{},
		supportTruncation: true,
		maxWrite:          4 << 20,
	}
}

func (r *recSink) Name() string { return r.name }

func (r *recSink) EnsureDir(_ context.Context, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dirs[path] = true
	return nil
}

func (r *recSink) WritePart(_ context.Context, path string, data []byte) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[path] = append([]byte(nil), data...)
	return r.name + ":" + path, nil
}

func (r *recSink) List(_ context.Context, path string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	prefix := strings.Trim(path, "/") + "/"
	var names []string
	for p := range r.files {
		if strings.HasPrefix(p, prefix) {
			rest := strings.TrimPrefix(p, prefix)
			if !strings.Contains(rest, "/") {
				names = append(names, rest)
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

func (r *recSink) Stat(context.Context) (SinkStat, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return SinkStat{
		Provider: r.name, TotalBytes: 100, UsedBytes: int64(len(r.files)),
		RemainingBytes: 100 - int64(len(r.files)), FileCount: len(r.files),
	}, nil
}

func (r *recSink) ReadFile(_ context.Context, path string, _ int64) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.files[path]
	if !ok {
		return nil, ErrExportNotFound
	}
	return b, nil
}

func (r *recSink) DeleteFile(_ context.Context, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.files[path]; !ok {
		return ErrExportNotFound
	}
	delete(r.files, path)
	return nil
}

func (r *recSink) ListTree(ctx context.Context, root string, maxEntries int) ([]TreeEntry, error) {
	entries, _, err := r.ListTreeWithTruncation(ctx, root, maxEntries)
	return entries, err
}

func (r *recSink) ListTreeWithTruncation(_ context.Context, root string, _ int) ([]TreeEntry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefix := strings.Trim(root, "/")
	if prefix != "" {
		prefix += "/"
	}
	var out []TreeEntry
	for p, b := range r.files {
		if prefix == "" || strings.HasPrefix(p, prefix) {
			out = append(out, TreeEntry{Path: p, SizeBytes: int64(len(b))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, r.truncated, nil
}

func (r *recSink) CountItems(context.Context, int) (int, int, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.countFolders, r.countFiles, r.truncated, nil
}

func (r *recSink) MaxWritePartBytes() int { return r.maxWrite }

func newMultiForTest(t *testing.T, n int) (*MultiAccountSink, []*recSink) {
	t.Helper()
	sinks := make([]*recSink, 0, n)
	asIface := make([]TrainingDataSink, 0, n)
	for i := range n {
		s := newRecSink(string(rune('A' + i)))
		sinks = append(sinks, s)
		asIface = append(asIface, s)
	}
	m, err := NewMultiAccountSink(asIface)
	require.NoError(t, err)
	return m, sinks
}

func (r *recSink) has(path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.files[path]
	return ok
}

func (r *recSink) fileCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.files)
}

// TestMultiSinkSameDirectoryAlwaysSameAccount 核心不变量：
// 同一目录的多次写入必须落在**同一个**账号上。
func TestMultiSinkSameDirectoryAlwaysSameAccount(t *testing.T) {
	m, sinks := newMultiForTest(t, 2)
	ctx := context.Background()
	dir := "2026-10-18/alice-7/gpt-4o"

	require.NoError(t, m.EnsureDir(ctx, dir))
	for i := range 10 {
		path := dir + "/part-000" + string(rune('1'+i)) + ".jsonl.gz"
		_, err := m.WritePart(ctx, path, []byte("x"))
		require.NoError(t, err)
	}

	// 只能有一个账号收到文件。
	nonEmpty := 0
	for _, s := range sinks {
		if s.fileCount() > 0 {
			nonEmpty++
		}
	}
	assert.Equal(t, 1, nonEmpty,
		"同一目录被劈到两个账号 → 分片序号会撞车（一边 part-0001、另一边也 part-0001）")
}

// TestMultiSinkExistingDirectoryDoesNotDrift 加入新账号后，
// **已有数据的老目录不能漂移**到新账号 —— 否则老分片在 A、新分片在 B，
// 同一目录被劈开，序号再次撞车。
func TestMultiSinkExistingDirectoryDoesNotDrift(t *testing.T) {
	// 先用单账号写入一个老目录。
	oldSink := newRecSink("A")
	ctx := context.Background()
	dir := "2026-10-18/bob-9/claude"
	require.NoError(t, oldSink.EnsureDir(ctx, dir))
	_, err := oldSink.WritePart(ctx, dir+"/part-0001.jsonl.gz", []byte("old"))
	require.NoError(t, err)

	// 之后才把 B 加进来。
	newSink := newRecSink("B")
	m, err := NewMultiAccountSink([]TrainingDataSink{oldSink, newSink})
	require.NoError(t, err)

	// 该目录继续写入 → 必须仍然落在 A。
	_, err = m.WritePart(ctx, dir+"/part-0002.jsonl.gz", []byte("new"))
	require.NoError(t, err)

	assert.True(t, oldSink.has(dir+"/part-0002.jsonl.gz"),
		"老目录的新分片必须跟着老分片留在同一账号")
	assert.False(t, newSink.has(dir+"/part-0002.jsonl.gz"),
		"老目录不得漂移到新账号")
}

// TestMultiSinkDistributesNewDirectories 全新目录必须**真的分到两个账号**，
// 否则「账号 B 也要用上」就是空话。
func TestMultiSinkDistributesNewDirectories(t *testing.T) {
	m, sinks := newMultiForTest(t, 2)
	ctx := context.Background()

	for i := range 40 {
		dir := "2026-10-18/user-" + string(rune('a'+i%26)) + "-" + itoaSmall(i) + "/gpt-4o"
		require.NoError(t, m.EnsureDir(ctx, dir))
		_, err := m.WritePart(ctx, dir+"/part-0001.jsonl.gz", []byte("x"))
		require.NoError(t, err)
	}

	a, b := sinks[0].fileCount(), sinks[1].fileCount()
	assert.Greater(t, a, 0, "账号 A 未被使用")
	assert.Greater(t, b, 0, "账号 B 未被使用 —— 负载均衡没生效")
	assert.Equal(t, 40, a+b, "所有分片都应落盘，实际 %d/%d", a, b)
}

// TestMultiSinkReadFallsBackToOtherAccount 读侧兜底：
// 数据若因历史/手工迁移落在非归属账号，仍要能读到，而不是报「文件不存在」。
func TestMultiSinkReadFallsBackToOtherAccount(t *testing.T) {
	m, sinks := newMultiForTest(t, 2)
	ctx := context.Background()

	// 直接往 B 塞一个文件，但不经过 m（模拟历史数据/手工迁移）。
	dir := "2026-10-18/carol-3/gpt-4o"
	path := dir + "/part-0001.jsonl.gz"
	sinks[1].files[path] = []byte("legacy-on-B")

	got, err := m.ReadFile(ctx, path, 1<<20)
	require.NoError(t, err, "归属账号没有时应兜底问其它账号")
	assert.Equal(t, []byte("legacy-on-B"), got)

	// 兜底成功后应记住真实归属：再次 List 该目录能看到它。
	names, err := m.List(ctx, dir)
	require.NoError(t, err)
	assert.Contains(t, names, "part-0001.jsonl.gz")
}

// TestMultiSinkListTreeMergesAndDedupes 浏览/导出要合并各账号的树，
// 且不能因为重复路径出现两行。
func TestMultiSinkListTreeMergesAndDedupes(t *testing.T) {
	m, sinks := newMultiForTest(t, 2)
	ctx := context.Background()

	pA := "2026-10-18/a-1/m/part-0001.jsonl.gz"
	pB := "2026-10-18/b-2/m/part-0001.jsonl.gz"
	sinks[0].files[pA] = []byte("a")
	sinks[1].files[pB] = []byte("b")
	// 人为制造重复路径（模拟手工迁移留下的副本）。
	sinks[1].files[pA] = []byte("dup")

	entries, truncated, err := m.ListTreeWithTruncation(ctx, "", 0)
	require.NoError(t, err)
	assert.False(t, truncated)

	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	assert.Contains(t, paths, pA)
	assert.Contains(t, paths, pB)
	// 去重：pA 只出现一次。
	count := 0
	for _, p := range paths {
		if p == pA {
			count++
		}
	}
	assert.Equal(t, 1, count, "重复路径必须去重，否则管理端出现两行同名文件")
	// 输出必须稳定排序（各账号返回顺序不同）。
	assert.True(t, sort.StringsAreSorted(paths), "合并后必须排序，实际 %v", paths)
}

// TestMultiSinkListTreeMarksTruncatedWhenOneAccountFails 某个账号读失败时，
// 结果必须标记为**不完整** —— 否则导出会产出"看似完整"的半包。
func TestMultiSinkListTreeMarksTruncatedWhenOneAccountFails(t *testing.T) {
	m, sinks := newMultiForTest(t, 2)
	ctx := context.Background()
	sinks[0].files["2026-10-18/a-1/m/p.jsonl.gz"] = []byte("a")
	sinks[1].truncated = true // 模拟该账号列举被截断

	_, truncated, err := m.ListTreeWithTruncation(ctx, "", 0)
	require.NoError(t, err)
	assert.True(t, truncated, "任一账号不完整，整体就不能声称完整")
}

// TestMultiSinkStatSumsAccounts 容量要跨账号累加（否则管理员看到的剩余容量偏小）。
func TestMultiSinkStatSumsAccounts(t *testing.T) {
	m, _ := newMultiForTest(t, 2)
	st, err := m.Stat(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(200), st.TotalBytes, "两个账号各 100 应累加为 200")
	assert.Contains(t, st.Provider, "x2", "供应商名要能看出用了几个账号")
}

// TestMultiSinkMaxWritePartBytesTakesMinimum worker 用这个值切分；
// 取最大值会让上限较小的账号收到超限分片而报错。
func TestMultiSinkMaxWritePartBytesTakesMinimum(t *testing.T) {
	m, sinks := newMultiForTest(t, 2)
	sinks[0].maxWrite = 8 << 20
	sinks[1].maxWrite = 2 << 20
	assert.Equal(t, 2<<20, m.MaxWritePartBytes(), "必须取最小值")
}

// TestMultiSinkCountItemsSumsAccounts 总览的目录/文件数要跨账号累加。
func TestMultiSinkCountItemsSumsAccounts(t *testing.T) {
	m, sinks := newMultiForTest(t, 2)
	sinks[0].countFolders, sinks[0].countFiles = 3, 10
	sinks[1].countFolders, sinks[1].countFiles = 4, 20

	folders, files, truncated, err := m.CountItems(context.Background(), 4)
	require.NoError(t, err)
	assert.Equal(t, 7, folders)
	assert.Equal(t, 30, files)
	assert.False(t, truncated)
}

// TestMultiSinkRejectsNonReaderBackend 只写不读的后端在这里没有意义：
// 管理端浏览/预览/删除/分析/导出全都要读。构造时就要拒绝。
func TestMultiSinkRejectsNonReaderBackend(t *testing.T) {
	writeOnly := &writeOnlySink{}
	_, err := NewMultiAccountSink([]TrainingDataSink{writeOnly})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SinkReader")
}

// TestMultiSinkToleratesListErrorsWhenResolving 解析归属时某账号 List 失败，
// 不能把"有数据的老目录"误判成全新目录（那会漂移到别的账号）。
func TestMultiSinkToleratesListErrorsWhenResolving(t *testing.T) {
	ctx := context.Background()
	a := newRecSink("A")
	b := newRecSink("B")
	dir := "2026-10-18/d-4/m"
	// 老数据在 B 上；A 的 List 报错。
	b.files[dir+"/part-0001.jsonl.gz"] = []byte("old")
	a.listErr = errors.New("transient")

	m, err := NewMultiAccountSink([]TrainingDataSink{a, b})
	require.NoError(t, err)

	_, err = m.WritePart(ctx, dir+"/part-0002.jsonl.gz", []byte("new"))
	require.NoError(t, err)
	assert.True(t, b.has(dir+"/part-0002.jsonl.gz"),
		"A 读失败时仍应发现数据在 B，从而继续用 B（不能漂移）")
}

// writeOnlySink 只实现写入接口，用于验证构造期校验。
type writeOnlySink struct{}

func (writeOnlySink) Name() string                                              { return "write-only" }
func (writeOnlySink) EnsureDir(context.Context, string) error                   { return nil }
func (writeOnlySink) WritePart(context.Context, string, []byte) (string, error) { return "", nil }
func (writeOnlySink) List(context.Context, string) ([]string, error)            { return nil, nil }
func (writeOnlySink) Stat(context.Context) (SinkStat, error)                    { return SinkStat{}, nil }

// TestLoadOneDriveConfigsPartialSecondAccountFails 「只配了一半的第二个账号」
// 必须**明确报错**，不能静默跳过。
//
// 静默跳过的后果：管理员以为两个账号在负载均衡，实际只有一个在写 ——
// 与「账号 B 也要用上」的诉求正好相反，且没有任何提示。
func TestLoadOneDriveConfigsPartialSecondAccountFails(t *testing.T) {
	// 主账号完整
	t.Setenv(EnvOneDriveTenant, "t")
	t.Setenv(EnvOneDriveClientID, "c")
	t.Setenv(EnvOneDriveClientSecret, "s")
	t.Setenv(EnvOneDriveDriveID, "d")
	// 第二账号只配了一部分（缺 secret 和 drive）
	t.Setenv("TRAINING_ONEDRIVE_2_TENANT", "t2")
	t.Setenv("TRAINING_ONEDRIVE_2_CLIENT_ID", "c2")

	_, err := LoadOneDriveConfigsFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "partially configured")
	assert.Contains(t, err.Error(), "account 2")
}

// TestLoadOneDriveConfigsSkipsAbsentSecondAccount 完全没配第二账号时正常返回一个。
func TestLoadOneDriveConfigsSkipsAbsentSecondAccount(t *testing.T) {
	t.Setenv(EnvOneDriveTenant, "t")
	t.Setenv(EnvOneDriveClientID, "c")
	t.Setenv(EnvOneDriveClientSecret, "s")
	t.Setenv(EnvOneDriveDriveID, "d")

	cfgs, err := LoadOneDriveConfigsFromEnv()
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
}

// TestLoadOneDriveConfigsInheritsRootPath 第二个账号未配 RootPath 时
// **沿用主账号**：多账号必须共用同一套目录布局，否则管理端会看到两套树。
func TestLoadOneDriveConfigsInheritsRootPath(t *testing.T) {
	t.Setenv(EnvOneDriveTenant, "t")
	t.Setenv(EnvOneDriveClientID, "c")
	t.Setenv(EnvOneDriveClientSecret, "s")
	t.Setenv(EnvOneDriveDriveID, "d")
	t.Setenv(EnvOneDriveRootPath, "training-data")

	t.Setenv("TRAINING_ONEDRIVE_2_TENANT", "t2")
	t.Setenv("TRAINING_ONEDRIVE_2_CLIENT_ID", "c2")
	t.Setenv("TRAINING_ONEDRIVE_2_CLIENT_SECRET", "s2")
	t.Setenv("TRAINING_ONEDRIVE_2_DRIVE_ID", "d2")
	// 注意：不设 TRAINING_ONEDRIVE_2_ROOT_PATH

	cfgs, err := LoadOneDriveConfigsFromEnv()
	require.NoError(t, err)
	require.Len(t, cfgs, 2)
	assert.Equal(t, "training-data", cfgs[1].RootPath, "第二账号应继承主账号的 RootPath")
}
