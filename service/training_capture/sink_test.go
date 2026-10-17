package training_capture

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sink 注册表与 disabled 后端的契约。
// 关键：**未配置时必须明确报错，不能静默成功**——否则数据丢了管理员却以为在采集。

func TestDisabledSinkReportsErrorsNotSilentSuccess(t *testing.T) {
	// 这是本文件最重要的断言：未配置云盘时，所有写操作必须返回错误。
	// 若 WritePart 返回 nil 错误，上层会以为采集成功，数据静默丢失。
	s := disabledSink{}
	assert.Equal(t, "disabled", s.Name())

	assert.ErrorIs(t, s.EnsureDir(context.Background(), "a/b"), ErrSinkNotConfigured)
	_, err := s.WritePart(context.Background(), "a/b", []byte("x"))
	assert.ErrorIs(t, err, ErrSinkNotConfigured)
	_, err = s.List(context.Background(), "a/b")
	assert.ErrorIs(t, err, ErrSinkNotConfigured)
	_, err = s.Stat(context.Background())
	assert.ErrorIs(t, err, ErrSinkNotConfigured)
}

func TestConfigureSinkUnknownNameIsExplicitError(t *testing.T) {
	// 配错后端名时必须报错，**不静默退回 disabled**——
	// 静默降级会让管理员以为在采集。
	err := ConfigureSink("onedrive-not-registered")
	require.Error(t, err, "未知后端名必须报错")
	assert.Contains(t, err.Error(), "unknown backend")
}

func TestConfigureSinkEmptyOrDisabledSelectsDisabled(t *testing.T) {
	require.NoError(t, ConfigureSink(""))
	assert.False(t, IsSinkConfigured())
	assert.Equal(t, "disabled", ActiveSink().Name())

	require.NoError(t, ConfigureSink("disabled"))
	assert.False(t, IsSinkConfigured())
}

func TestConfigureSinkUsesRegisteredFactory(t *testing.T) {
	restore := SetSinkForTest(nil)
	defer restore()

	RegisterSink("fake-for-test", func() (TrainingDataSink, error) {
		return fakeSink{name: "fake-for-test"}, nil
	})

	require.NoError(t, ConfigureSink("fake-for-test"))
	assert.True(t, IsSinkConfigured())
	assert.Equal(t, "fake-for-test", ActiveSink().Name())
}

func TestConfigureSinkPropagatesFactoryError(t *testing.T) {
	// 工厂报错（如凭据无效）必须冒泡，不能吞掉后启用一个坏后端。
	restore := SetSinkForTest(nil)
	defer restore()

	boom := errors.New("credentials invalid")
	RegisterSink("broken-for-test", func() (TrainingDataSink, error) {
		return nil, boom
	})

	err := ConfigureSink("broken-for-test")
	assert.ErrorIs(t, err, boom)
}

func TestActiveSinkIsNeverNil(t *testing.T) {
	// 调用方不应需要判 nil；未配置时是 disabledSink。
	restore := SetSinkForTest(nil)
	defer restore()
	assert.NotNil(t, ActiveSink())
}

func TestSetSinkForTestRestoresPrevious(t *testing.T) {
	original := ActiveSink()
	restore := SetSinkForTest(fakeSink{name: "temp"})
	assert.Equal(t, "temp", ActiveSink().Name())
	restore()
	assert.Equal(t, original.Name(), ActiveSink().Name())
}

func TestSetSinkForTestNilMeansDisabled(t *testing.T) {
	restore := SetSinkForTest(fakeSink{name: "x"})
	require.True(t, IsSinkConfigured())
	// 传 nil 应等价于「恢复为未配置」。
	restore2 := SetSinkForTest(nil)
	assert.False(t, IsSinkConfigured())
	restore2()
	restore()
}

func TestRegisteredSinkNamesIncludesRegistered(t *testing.T) {
	RegisterSink("listed-for-test", func() (TrainingDataSink, error) {
		return fakeSink{name: "listed-for-test"}, nil
	})
	names := RegisteredSinkNames()
	assert.Contains(t, names, "listed-for-test")
}

func TestErrSinkNotSupportedIsDistinctFromNotConfigured(t *testing.T) {
	// 两者语义不同：「不支持」→ 降级展示；「未配置」→ 提示管理员去配。
	// 混为一谈会让管理员看到错误的行动指引。
	assert.False(t, errors.Is(ErrSinkNotSupported, ErrSinkNotConfigured))
	assert.False(t, errors.Is(ErrSinkNotConfigured, ErrSinkNotSupported))
}

// fakeSink 满足接口的最小实现（仅用于注册表测试）。
type fakeSink struct{ name string }

func (f fakeSink) Name() string { return f.name }

func (f fakeSink) EnsureDir(context.Context, string) error { return nil }

func (f fakeSink) WritePart(context.Context, string, []byte) (string, error) {
	return "ref", nil
}

func (f fakeSink) List(context.Context, string) ([]string, error) { return nil, nil }

func (f fakeSink) Stat(context.Context) (SinkStat, error) {
	return SinkStat{Provider: f.name}, nil
}

// ---------- 后端自报单次写入上限（PartUploadLimiter） ----------

// limitedSink 声明自己的单次写入上限（模拟 S3/OSS/WebDAV 等不同后端）。
type limitedSink struct {
	fakeSink
	maxWrite int
}

func (l limitedSink) MaxWritePartBytes() int { return l.maxWrite }

func TestEffectiveMaxWritePartBytesUsesBackendDeclaration(t *testing.T) {
	// 后端**声明**了上限 → 必须用它。worker 据此切分，
	// 否则抽象层就是个谎言（OneDrive 4MB 写死，换 S3 会白白切碎）。
	if got := EffectiveMaxWritePartBytes(limitedSink{maxWrite: 5 << 30}); got != 5<<30 {
		t.Fatalf("必须采用后端声明的上限，实际 %d", got)
	}
}

func TestEffectiveMaxWritePartBytesFallsBackWhenUndeclared(t *testing.T) {
	// 未声明上限的后端 → 退保守默认值，行为与历史一致（不会变坏）。
	if got := EffectiveMaxWritePartBytes(fakeSink{name: "x"}); got != defaultMaxWritePartBytes {
		t.Fatalf("未声明时应退默认值 %d，实际 %d", defaultMaxWritePartBytes, got)
	}
	// nil sink 也必须给出正数（调用方可能在 sink 就绪前算限额）。
	if got := EffectiveMaxWritePartBytes(nil); got <= 0 {
		t.Fatalf("nil sink 必须返回正数上限，实际 %d", got)
	}
}

func TestEffectiveMaxWritePartBytesIgnoresNonPositiveDeclaration(t *testing.T) {
	// 后端误报 0/负数不能把上限变成 0 —— 那会让分片被无限切碎。
	if got := EffectiveMaxWritePartBytes(limitedSink{maxWrite: 0}); got != defaultMaxWritePartBytes {
		t.Fatalf("非正声明必须退默认值，实际 %d", got)
	}
	if got := EffectiveMaxWritePartBytes(limitedSink{maxWrite: -1}); got != defaultMaxWritePartBytes {
		t.Fatalf("负声明必须退默认值，实际 %d", got)
	}
}

// ---------- 压缩比差时的重切（F18：不能再切压缩产物） ----------

// TestResplitProducesIndependentlyValidGzipMembers 锁定最重要的一条：
// 重切后每个子块**各自压缩**，都是**完整 gzip member**，可独立解压还原。
//
// 旧实现直接 `splitBySize(压缩产物)` —— 碎片不是合法 gzip，云盘上出现
// 二进制垃圾（ADR-003 记录的同类损坏）。这条测试用**不可压缩数据**
// （随机字节 → 压缩比≈1，必然走进重切分支）把它钉死。
func TestResplitProducesIndependentlyValidGzipMembers(t *testing.T) {
	defer ResetBudgetForTest()

	// 随机数据压缩比≈1：8MB 原始 → ~8MB 压缩，超过 4MB 上限 → 必须重切。
	rnd := make([]byte, 8<<20)
	seed := uint64(0x9E3779B97F4A7C15)
	for i := range rnd {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		rnd[i] = byte(seed)
	}
	// 用换行把随机数据分成可切的行（splitBySize 按行切）。
	for i := 1024; i < len(rnd); i += 1024 {
		rnd[i-1] = '\n'
	}

	const limit = 4 << 20
	chunks := resplitCompressedBetter(rnd, limit)
	require.Greater(t, len(chunks), 1, "超限数据必须被切成多块")

	var total int
	for i, c := range chunks {
		blob, err := gzipCompress(c)
		require.NoError(t, err)
		// 不变量 1：每块压缩后必须 ≤ 后端上限（否则 WritePart 会报错）。
		assert.LessOrEqual(t, len(blob), limit,
			"第 %d 块压缩后 %d 字节仍超上限 %d", i, len(blob), limit)
		// 不变量 2：每块必须是**完整 member**，可独立解压回原文。
		back, err := gzipDecompress(blob, int64(len(c))+1024)
		require.NoError(t, err, "第 %d 块不是合法 gzip member（这正是旧实现在做错的事）", i)
		assert.Equal(t, c, back, "第 %d 块解压后与原文不一致", i)
		total += len(c)
	}
	// 不变量 3：不丢字节（拼接后与原文完全一致）。
	assert.Equal(t, len(rnd), total, "重切不得丢字节")
	var joined []byte
	for _, c := range chunks {
		joined = append(joined, c...)
	}
	assert.Equal(t, rnd, joined, "重切后拼接必须逐字节等于原文")
}

// TestResplitLeavesCompressibleDataAlone 压缩良好的数据不应被切 ——
// 否则会把正常采集的分片数无故放大。
func TestResplitLeavesCompressibleDataAlone(t *testing.T) {
	defer ResetBudgetForTest()

	// 高度可压缩：4MB 重复文本 → 压缩后极小。
	body := bytes.Repeat([]byte("{\"role\":\"user\",\"content\":\"hello world\"}\n"), 100_000)
	chunks := resplitCompressedBetter(body, 4<<20)
	require.Len(t, chunks, 1, "能放下就不该切")
	assert.Equal(t, body, chunks[0])
}

// ---------- 目录/文件计数（管理员视图的「N 个目录 / M 个文件」） ----------

// layoutTree 按真实采集布局造一棵树：
// `<date>/<user-id>/<model>/part-0001.jsonl.gz`，两层日期/两个模型。
func layoutTree() func(string) ([]dirEntry, error) {
	tree := map[string][]dirEntry{
		"": {
			{name: "2026-10-16", isFolder: true},
			{name: "2026-10-17", isFolder: true},
		},
		"2026-10-16": {
			{name: "alice-1", isFolder: true},
		},
		"2026-10-16/alice-1": {
			{name: "gpt-4o", isFolder: true},
		},
		"2026-10-16/alice-1/gpt-4o": {
			{name: "part-0001.jsonl.gz"},
			{name: "part-0002.jsonl.gz"},
		},
		"2026-10-17": {
			{name: "bob-2", isFolder: true},
		},
		"2026-10-17/bob-2": {
			{name: "claude", isFolder: true},
		},
		"2026-10-17/bob-2/claude": {
			{name: "part-0001.jsonl.gz"},
		},
	}
	return func(path string) ([]dirEntry, error) { return tree[path], nil }
}

// TestCountTreeWithLayoutDepthMissesFiles 锁定**曾经的真实缺陷**：
// 用布局层数（3）作遍历深度，会把模型目录计入目录数、却从不展开它，
// 于是文件数恒为 0 —— 管理员看到「N 个文件夹 / 0 个文件」这个
// 自信但错误的数字（而 truncated=false 让错误更像真的）。
func TestCountTreeWithLayoutDepthMissesFiles(t *testing.T) {
	folders, files, truncated := countTree(layoutTree(), CaptureLayoutDepth, 0)

	// 目录数：2 个日期 + 2 个用户 + 2 个模型 = 6（模型目录被计数）。
	assert.Equal(t, 6, folders, "目录数应含未展开的模型目录")
	// 这正是缺陷：分片在模型目录**内部**，深度不够就一个也数不到。
	assert.Equal(t, 0, files, "布局层数作深度时文件数恒为 0（这就是缺陷本身）")
	assert.False(t, truncated, "队列排空了，可错误值看起来完全『可信』")
}

// TestCountTreeWithLayoutDepthPlusOneCountsFiles 锁定正确用法：
// 生产调用方必须传 `CaptureLayoutDepth + 1`。
func TestCountTreeWithLayoutDepthPlusOneCountsFiles(t *testing.T) {
	folders, files, truncated := countTree(layoutTree(), CaptureLayoutDepth+1, 0)

	assert.Equal(t, 6, folders)
	// 3 个分片文件全部数到。
	assert.Equal(t, 3, files, "展开到模型目录内部才数得到分片")
	assert.False(t, truncated)
}

// TestCountTreeDepthBoundaries 穷举深度语义：maxDepth=1 只看根的直接子项，
// 依此类推。防止将来有人改判断方向（`<` 改 `<=`）造成静默偏移。
func TestCountTreeDepthBoundaries(t *testing.T) {
	cases := []struct {
		maxDepth int
		folders  int
		files    int
	}{
		// 深度 1：只列根 → 2 个日期目录被计数，均不展开。
		{maxDepth: 1, folders: 2, files: 0},
		// 深度 2：列日期 → 2 个用户目录被计数。
		{maxDepth: 2, folders: 4, files: 0},
		// 深度 3：列用户 → 2 个模型目录被计数（缺陷所在）。
		{maxDepth: 3, folders: 6, files: 0},
		// 深度 4：列模型 → 3 个分片文件被计数。
		{maxDepth: 4, folders: 6, files: 3},
		// 深度更大：没有更深层级，数字不变。
		{maxDepth: 9, folders: 6, files: 3},
	}
	for _, tc := range cases {
		folders, files, truncated := countTree(layoutTree(), tc.maxDepth, 0)
		assert.Equalf(t, tc.folders, folders, "maxDepth=%d 的目录数", tc.maxDepth)
		assert.Equalf(t, tc.files, files, "maxDepth=%d 的文件数", tc.maxDepth)
		assert.Falsef(t, truncated, "maxDepth=%d 不应截断（队列可排空）", tc.maxDepth)
	}
}

// TestCountTreeReportsTruncationWhenVisitLimitHit 达到访问上限必须报
// truncated —— 否则管理员把**下界**当总量做扩容决策。
func TestCountTreeReportsTruncationWhenVisitLimitHit(t *testing.T) {
	// 只允许访问 1 个目录（根）→ 队列里还剩 2 个日期目录。
	folders, files, truncated := countTree(layoutTree(), CaptureLayoutDepth+1, 1)

	assert.True(t, truncated, "队列未排空必须报截断")
	assert.Equal(t, 2, folders, "只展开了根")
	assert.Equal(t, 0, files)
}

// TestCountTreeToleratesUnreadableDirectory 单个目录读失败不得中断整体统计
// （部分结果比没有结果有用），但**必须报告结果不完整**。
//
// [修复防御] 本条原先断言 `truncated == false`，等于把缺陷固化成了契约：
// 读失败时管理员看到「N 个目录 / M 个文件」且没有任何"不完整"标志，
// 会当成权威规模去做扩缩容决策 —— 而实际有一整棵子树没读到。
// 这与「文件数恒为 0」是同一类错误：数字自信、无任何提示。
func TestCountTreeToleratesUnreadableDirectory(t *testing.T) {
	full := layoutTree()
	flaky := func(path string) ([]dirEntry, error) {
		if path == "2026-10-16" {
			return nil, assert.AnError
		}
		return full(path)
	}
	folders, files, truncated := countTree(flaky, CaptureLayoutDepth+1, 0)

	// 另一支（2026-10-17）仍应被数到。
	assert.Equal(t, 1, files, "可读的那一支必须仍被统计")
	assert.True(t, truncated,
		"读失败意味着数字是**下界**，必须置 truncated，否则管理员会把它当权威值")
	assert.Greater(t, folders, 0)
}

// TestCountTreeEmptyTreeIsZeroNotTruncated 空云盘（采集尚未开始）是正常状态。
func TestCountTreeEmptyTreeIsZeroNotTruncated(t *testing.T) {
	empty := func(string) ([]dirEntry, error) { return nil, nil }
	folders, files, truncated := countTree(empty, CaptureLayoutDepth+1, 0)

	assert.Equal(t, 0, folders)
	assert.Equal(t, 0, files)
	assert.False(t, truncated, "空树不是截断——前端才不会误显示『数字是下界』")
}
