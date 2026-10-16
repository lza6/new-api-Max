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
