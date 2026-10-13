package training_capture

import (
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
