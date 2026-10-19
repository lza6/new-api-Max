package common

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountOverloadSampleRequiresConsecutiveSamples(t *testing.T) {
	samples := 0
	assert.False(t, countOverloadSample(&samples, true))
	assert.False(t, countOverloadSample(&samples, true))
	assert.True(t, countOverloadSample(&samples, true), "第三次连续超阈值才判定过载")

	// 任一采样回落立即清零，重新累计。
	assert.False(t, countOverloadSample(&samples, false))
	assert.False(t, countOverloadSample(&samples, true))
	assert.False(t, countOverloadSample(&samples, true))
	assert.True(t, countOverloadSample(&samples, true))
}

func TestCheckSystemOverload(t *testing.T) {
	now := time.Now()
	cfg := PerformanceMonitorConfig{Enabled: true, CPUThreshold: 90, MemoryThreshold: 90, DiskThreshold: 90}

	over := SystemStatus{
		SampledAt:     now,
		CPUUsage:      95,
		CPUOverloaded: true,
	}
	got := CheckSystemOverload(over, cfg, now)
	require.NotNil(t, got)
	assert.Equal(t, "system_cpu_overloaded", got.Code)

	// 采样过期（监控停滞）不得熔断。
	stale := over
	stale.SampledAt = now.Add(-31 * time.Second)
	assert.Nil(t, CheckSystemOverload(stale, cfg, now))

	// 监控关闭不得熔断。
	disabled := cfg
	disabled.Enabled = false
	assert.Nil(t, CheckSystemOverload(over, disabled, now))

	// 阈值被调高后立即放行（不等待下一次采样）。
	raised := cfg
	raised.CPUThreshold = 99
	assert.Nil(t, CheckSystemOverload(over, raised, now))

	// 标记为过载但当前值已回落 → 放行。
	recovered := over
	recovered.CPUUsage = 80
	assert.Nil(t, CheckSystemOverload(recovered, cfg, now))
}

func TestContainerMemoryUsageExcludesInactiveFileCache(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "memory.max"), []byte("1000\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "memory.current"), []byte("400\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "memory.stat"), []byte("inactive_file 100\n"), 0o644))

	u := &containerUsage{cgroupRoot: dir}
	pct, ok := u.memoryUsage()
	require.True(t, ok)
	// (400 - 100) / 1000 = 30%
	assert.InDelta(t, 30, pct, 0.01)

	// 未限制（v2 的 "max"）→ 不可用，调用方回退宿主机口径。
	require.NoError(t, os.WriteFile(filepath.Join(dir, "memory.max"), []byte("max\n"), 0o644))
	_, ok = u.memoryUsage()
	assert.False(t, ok)
}

func TestContainerCPUSaturation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cpu.max"), []byte("50000 100000\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cpu.stat"), []byte("usage_usec 1000000\n"), 0o644))

	u := &containerUsage{cgroupRoot: dir, schedulableCPUs: 4}
	t0 := time.Now()
	_, ok := u.cpuSaturation(t0)
	assert.False(t, ok, "第一次采样只建立基线")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "cpu.stat"), []byte("usage_usec 1500000\n"), 0o644))
	pct, ok := u.cpuSaturation(t0.Add(time.Second))
	require.True(t, ok)
	// 1 秒内用了 0.5 秒 CPU，配额 0.5 核 → 100%
	assert.InDelta(t, 100, pct, 0.5)
}
